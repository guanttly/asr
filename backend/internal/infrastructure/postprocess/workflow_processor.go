package postprocess

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"path"
	"strings"
	"unicode/utf8"

	appwf "github.com/lgt/asr/internal/application/workflow"
	asrdomain "github.com/lgt/asr/internal/domain/asr"
	meetingdomain "github.com/lgt/asr/internal/domain/meeting"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
)

// WorkflowExecutor runs a workflow pipeline on text input and returns the execution details.
type WorkflowExecutor interface {
	ExecuteForTask(ctx context.Context, task *asrdomain.TranscriptionTask, inputText string) (*appwf.ExecutionResponse, error)
	ResumeForTaskFromFailure(ctx context.Context, task *asrdomain.TranscriptionTask) (*appwf.ExecutionResponse, error)
}

// WorkflowAwareProcessor chooses between workflow engine and legacy corrector/summarizer
// based on whether the task has a WorkflowID set.
type WorkflowAwareProcessor struct {
	legacy           *BatchMeetingProcessor
	workflowExecutor WorkflowExecutor
	meetingRepo      meetingdomain.MeetingRepository
	transcriptRepo   meetingdomain.TranscriptRepository
	summaryRepo      meetingdomain.SummaryRepository
}

// NewWorkflowAwareProcessor creates a post processor that delegates to workflows when configured.
func NewWorkflowAwareProcessor(
	legacy *BatchMeetingProcessor,
	workflowExecutor WorkflowExecutor,
	meetingRepo meetingdomain.MeetingRepository,
	transcriptRepo meetingdomain.TranscriptRepository,
	summaryRepo meetingdomain.SummaryRepository,
) *WorkflowAwareProcessor {
	return &WorkflowAwareProcessor{
		legacy:           legacy,
		workflowExecutor: workflowExecutor,
		meetingRepo:      meetingRepo,
		transcriptRepo:   transcriptRepo,
		summaryRepo:      summaryRepo,
	}
}

// ProcessCompletedTask fulfils the CompletedTaskProcessor interface.
func (p *WorkflowAwareProcessor) ProcessCompletedTask(ctx context.Context, task *asrdomain.TranscriptionTask) error {
	if task.WorkflowID == nil || p.workflowExecutor == nil {
		return p.legacy.ProcessCompletedTask(ctx, task)
	}
	return p.processWithWorkflow(ctx, task, false)
}

// ResumeCompletedTaskFromFailure continues a failed workflow from the failed node.
func (p *WorkflowAwareProcessor) ResumeCompletedTaskFromFailure(ctx context.Context, task *asrdomain.TranscriptionTask) error {
	if task.WorkflowID == nil || p.workflowExecutor == nil {
		return p.legacy.ResumeCompletedTaskFromFailure(ctx, task)
	}
	return p.processWithWorkflow(ctx, task, true)
}

func (p *WorkflowAwareProcessor) processWithWorkflow(ctx context.Context, task *asrdomain.TranscriptionTask, resume bool) error {
	if task.MeetingID != nil {
		return nil
	}

	text := strings.TrimSpace(task.ResultText)
	if text == "" {
		return fmt.Errorf("empty transcription result")
	}

	var (
		exec *appwf.ExecutionResponse
		err  error
	)
	if resume {
		exec, err = p.workflowExecutor.ResumeForTaskFromFailure(ctx, task)
	} else {
		exec, err = p.workflowExecutor.ExecuteForTask(ctx, task, text)
	}
	if exec != nil && strings.TrimSpace(exec.FinalText) != "" {
		text = exec.FinalText
	}
	task.ResultText = text
	if err != nil {
		return fmt.Errorf("workflow execution failed: %w", err)
	}

	content, modelVersion, ok := extractWorkflowSummary(exec, task.WorkflowID)
	if !ok {
		return nil
	}

	existing, err := p.meetingRepo.GetBySourceTaskID(ctx, task.ID)
	if err != nil {
		return err
	}
	if existing != nil {
		task.MeetingID = &existing.ID
		return nil
	}

	meeting := &meetingdomain.Meeting{
		SourceTaskID:  &task.ID,
		WorkflowID:    task.WorkflowID,
		UserID:        task.UserID,
		Title:         buildWorkflowMeetingTitle(task),
		AudioURL:      task.AudioURL,
		LocalFilePath: task.LocalFilePath,
		Duration:      task.Duration,
		Status:        meetingdomain.MeetingStatusCompleted,
	}
	if err := p.meetingRepo.Create(ctx, meeting); err != nil {
		return err
	}

	transcripts := buildWorkflowMeetingTranscripts(exec, meeting.ID, text, task.Duration)
	if err := p.transcriptRepo.BatchCreate(ctx, transcripts); err != nil {
		return err
	}

	if err := p.summaryRepo.Create(ctx, &meetingdomain.Summary{
		MeetingID:    meeting.ID,
		Content:      content,
		ModelVersion: modelVersion,
	}); err != nil {
		return err
	}

	task.MeetingID = &meeting.ID
	return nil
}

type diarizationSegment struct {
	Speaker   string  `json:"speaker"`
	StartTime float64 `json:"start_time"`
	EndTime   float64 `json:"end_time"`
	Start     float64 `json:"start"`
	End       float64 `json:"end"`
}

func buildWorkflowMeetingTranscripts(exec *appwf.ExecutionResponse, meetingID uint64, fallbackText string, duration float64) []meetingdomain.Transcript {
	transcriptText := workflowTranscriptSourceText(exec, fallbackText)
	segments := extractWorkflowSpeakerSegments(exec)
	if len(segments) == 0 {
		return []meetingdomain.Transcript{{
			MeetingID:    meetingID,
			SpeakerLabel: "ASR",
			Text:         transcriptText,
			StartTime:    0,
			EndTime:      duration,
		}}
	}

	parts := splitTranscriptForSegments(transcriptText, segments)
	transcripts := make([]meetingdomain.Transcript, 0, len(segments))
	for index, seg := range segments {
		startTime := seg.StartTime
		if startTime == 0 && seg.Start > 0 {
			startTime = seg.Start
		}
		endTime := seg.EndTime
		if endTime == 0 && seg.End > 0 {
			endTime = seg.End
		}
		label := strings.TrimSpace(seg.Speaker)
		if label == "" {
			label = fmt.Sprintf("Speaker %d", index+1)
		}
		text := ""
		if index < len(parts) {
			text = strings.TrimSpace(parts[index])
		}
		transcripts = append(transcripts, meetingdomain.Transcript{
			MeetingID:    meetingID,
			SpeakerLabel: label,
			Text:         text,
			StartTime:    startTime,
			EndTime:      endTime,
		})
	}
	return transcripts
}

func workflowTranscriptSourceText(exec *appwf.ExecutionResponse, fallbackText string) string {
	if exec != nil {
		for index := len(exec.NodeResults) - 1; index >= 0; index-- {
			result := exec.NodeResults[index]
			if result.NodeType != wfdomain.NodeMeetingSummary || result.Status != wfdomain.NodeResultSuccess {
				continue
			}
			if text := strings.TrimSpace(result.InputText); text != "" {
				return text
			}
		}
		if text := strings.TrimSpace(exec.FinalText); text != "" {
			return text
		}
	}
	return strings.TrimSpace(fallbackText)
}

func extractWorkflowSpeakerSegments(exec *appwf.ExecutionResponse) []diarizationSegment {
	if exec == nil {
		return nil
	}
	for index := len(exec.NodeResults) - 1; index >= 0; index-- {
		result := exec.NodeResults[index]
		if result.NodeType != wfdomain.NodeSpeakerDiarize || result.Status != wfdomain.NodeResultSuccess || len(result.Detail) == 0 {
			continue
		}
		var payload struct {
			Segments []diarizationSegment `json:"segments"`
		}
		if err := json.Unmarshal(result.Detail, &payload); err != nil {
			continue
		}
		if len(payload.Segments) > 0 {
			return payload.Segments
		}
	}
	return nil
}

func splitTranscriptForSegments(text string, segments []diarizationSegment) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" || len(segments) == 0 {
		return nil
	}

	units := transcriptTextUnits(trimmed)
	if len(units) == 0 {
		return []string{trimmed}
	}
	if len(units) == 1 {
		return splitSingleUnitByDuration(units[0], segments)
	}

	totalRunes := 0
	unitRunes := make([]int, len(units))
	for index, unit := range units {
		count := utf8.RuneCountInString(unit)
		unitRunes[index] = count
		totalRunes += count
	}

	totalDuration := 0.0
	for _, seg := range segments {
		length := segmentDuration(seg)
		if length > 0 {
			totalDuration += length
		}
	}
	if totalDuration <= 0 {
		totalDuration = float64(len(segments))
	}

	parts := make([]string, 0, len(segments))
	unitIndex := 0
	remainingRunes := totalRunes
	remainingDuration := totalDuration

	for segIndex, seg := range segments {
		remainingSegments := len(segments) - segIndex
		if segIndex == len(segments)-1 {
			parts = append(parts, strings.Join(units[unitIndex:], " "))
			break
		}

		segDuration := segmentDuration(seg)
		if segDuration <= 0 {
			segDuration = remainingDuration / float64(remainingSegments)
		}
		targetRunes := int(math.Round(float64(remainingRunes) * (segDuration / remainingDuration)))
		if targetRunes <= 0 {
			targetRunes = 1
		}

		collectedRunes := 0
		startIndex := unitIndex
		for unitIndex < len(units) {
			if unitIndex > startIndex && collectedRunes >= targetRunes && len(units)-unitIndex >= remainingSegments-1 {
				break
			}
			collectedRunes += unitRunes[unitIndex]
			unitIndex += 1
			if len(units)-unitIndex < remainingSegments-1 {
				break
			}
		}
		if unitIndex <= startIndex {
			unitIndex = startIndex + 1
			collectedRunes = unitRunes[startIndex]
		}
		parts = append(parts, strings.Join(units[startIndex:unitIndex], " "))
		remainingRunes -= collectedRunes
		remainingDuration -= segDuration
		if remainingDuration <= 0 {
			remainingDuration = float64(len(segments) - segIndex - 1)
		}
	}

	for len(parts) < len(segments) {
		parts = append(parts, "")
	}
	return parts
}

func transcriptTextUnits(text string) []string {
	normalized := strings.ReplaceAll(strings.TrimSpace(text), "\r\n", "\n")
	if normalized == "" {
		return nil
	}
	lines := strings.Split(normalized, "\n")
	units := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		units = append(units, splitTextBySentence(trimmed)...)
	}
	return units
}

func splitTextBySentence(text string) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 {
		return nil
	}
	parts := make([]string, 0)
	start := 0
	for index, char := range runes {
		if !strings.ContainsRune("。！？!?；;", char) {
			continue
		}
		segment := strings.TrimSpace(string(runes[start : index+1]))
		if segment != "" {
			parts = append(parts, segment)
		}
		start = index + 1
	}
	if start < len(runes) {
		segment := strings.TrimSpace(string(runes[start:]))
		if segment != "" {
			parts = append(parts, segment)
		}
	}
	if len(parts) == 0 {
		return []string{strings.TrimSpace(text)}
	}
	return parts
}

func splitSingleUnitByDuration(text string, segments []diarizationSegment) []string {
	runes := []rune(strings.TrimSpace(text))
	if len(runes) == 0 || len(segments) == 0 {
		return nil
	}
	totalDuration := 0.0
	for _, seg := range segments {
		length := segmentDuration(seg)
		if length > 0 {
			totalDuration += length
		}
	}
	if totalDuration <= 0 {
		totalDuration = float64(len(segments))
	}

	parts := make([]string, 0, len(segments))
	start := 0
	remainingDuration := totalDuration
	remainingRunes := len(runes)
	for index, seg := range segments {
		if index == len(segments)-1 {
			parts = append(parts, strings.TrimSpace(string(runes[start:])))
			break
		}
		segDuration := segmentDuration(seg)
		if segDuration <= 0 {
			segDuration = remainingDuration / float64(len(segments)-index)
		}
		take := int(math.Round(float64(remainingRunes) * (segDuration / remainingDuration)))
		if take <= 0 {
			take = 1
		}
		end := start + take
		minRemaining := len(segments) - index - 1
		maxEnd := len(runes) - minRemaining
		if end > maxEnd {
			end = maxEnd
		}
		if end <= start {
			end = start + 1
		}
		parts = append(parts, strings.TrimSpace(string(runes[start:end])))
		remainingRunes -= end - start
		remainingDuration -= segDuration
		if remainingDuration <= 0 {
			remainingDuration = float64(len(segments) - index - 1)
		}
		start = end
	}
	for len(parts) < len(segments) {
		parts = append(parts, "")
	}
	return parts
}

func segmentDuration(seg diarizationSegment) float64 {
	start := seg.StartTime
	if start == 0 && seg.Start > 0 {
		start = seg.Start
	}
	end := seg.EndTime
	if end == 0 && seg.End > 0 {
		end = seg.End
	}
	if end <= start {
		return 0
	}
	return end - start
}

func extractWorkflowSummary(exec *appwf.ExecutionResponse, workflowID *uint64) (string, string, bool) {
	if exec == nil {
		return "", "", false
	}
	for index := len(exec.NodeResults) - 1; index >= 0; index-- {
		result := exec.NodeResults[index]
		if result.NodeType != wfdomain.NodeMeetingSummary || result.Status != wfdomain.NodeResultSuccess {
			continue
		}
		content := strings.TrimSpace(result.OutputText)
		if content == "" {
			return "", "", false
		}
		return content, workflowSummaryModelVersion(result.Detail, workflowID), true
	}
	return "", "", false
}

func workflowSummaryModelVersion(detail json.RawMessage, workflowID *uint64) string {
	var payload struct {
		ModelVersion string `json:"model_version"`
		Model        string `json:"model"`
		Source       string `json:"source"`
	}
	if len(detail) > 0 {
		_ = json.Unmarshal(detail, &payload)
	}
	if payload.ModelVersion != "" {
		return payload.ModelVersion
	}
	if payload.Model != "" {
		return payload.Model
	}
	if payload.Source != "" {
		return payload.Source
	}
	if workflowID != nil {
		return fmt.Sprintf("workflow:%d", *workflowID)
	}
	return "workflow"
}

func buildWorkflowMeetingTitle(task *asrdomain.TranscriptionTask) string {
	if task.AudioURL != "" {
		base := path.Base(task.AudioURL)
		ext := path.Ext(base)
		if name := strings.TrimSuffix(base, ext); name != "" {
			return name
		}
	}
	return fmt.Sprintf("转写任务 #%d", task.ID)
}

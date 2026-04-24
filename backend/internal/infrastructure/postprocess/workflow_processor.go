package postprocess

import (
	"context"
	"encoding/json"
	"fmt"
	"path"
	"strings"

	appwf "github.com/lgt/asr/internal/application/workflow"
	asrdomain "github.com/lgt/asr/internal/domain/asr"
	meetingdomain "github.com/lgt/asr/internal/domain/meeting"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
	infraDiarization "github.com/lgt/asr/internal/infrastructure/diarization"
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
	zeroBasedAnonymousLabels := workflowSegmentsUseZeroBasedAnonymousLabels(segments)
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
		label = infraDiarization.NormalizeAnonymousSpeakerLabel(label, zeroBasedAnonymousLabels)
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
	if strings.TrimSpace(text) == "" || len(segments) == 0 {
		return nil
	}
	durations := make([]float64, 0, len(segments))
	for _, seg := range segments {
		durations = append(durations, segmentDuration(seg))
	}
	return infraDiarization.SplitTranscriptByDurations(text, durations)
}

func workflowSegmentsUseZeroBasedAnonymousLabels(segments []diarizationSegment) bool {
	labels := make([]string, 0, len(segments))
	for _, seg := range segments {
		if label := strings.TrimSpace(seg.Speaker); label != "" {
			labels = append(labels, label)
		}
	}
	return infraDiarization.AnonymousSpeakerLabelsUseZeroBased(labels)
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

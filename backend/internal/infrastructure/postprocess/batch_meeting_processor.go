package postprocess

import (
	"context"
	"fmt"
	"path"
	"strings"

	asrdomain "github.com/lgt/asr/internal/domain/asr"
	meetingdomain "github.com/lgt/asr/internal/domain/meeting"
)

// Corrector applies terminology correction before transcript persistence.
type Corrector interface {
	Correct(ctx context.Context, dictID *uint64, text string) (string, map[string][]string, error)
}

// Summarizer generates a summary for corrected transcript text.
type Summarizer interface {
	Summarize(ctx context.Context, text string) (string, string, error)
}

// BatchMeetingProcessor materializes completed batch tasks into meetings.
type BatchMeetingProcessor struct {
	meetingRepo    meetingdomain.MeetingRepository
	transcriptRepo meetingdomain.TranscriptRepository
	summaryRepo    meetingdomain.SummaryRepository
	corrector      Corrector
	summarizer     Summarizer
}

// NewBatchMeetingProcessor creates a completed-task post processor.
func NewBatchMeetingProcessor(
	meetingRepo meetingdomain.MeetingRepository,
	transcriptRepo meetingdomain.TranscriptRepository,
	summaryRepo meetingdomain.SummaryRepository,
	corrector Corrector,
	summarizer Summarizer,
) *BatchMeetingProcessor {
	return &BatchMeetingProcessor{
		meetingRepo:    meetingRepo,
		transcriptRepo: transcriptRepo,
		summaryRepo:    summaryRepo,
		corrector:      corrector,
		summarizer:     summarizer,
	}
}

// ProcessCompletedTask creates or reuses a meeting for a finished batch task.
func (p *BatchMeetingProcessor) ProcessCompletedTask(ctx context.Context, task *asrdomain.TranscriptionTask) error {
	if task.MeetingID != nil {
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

	text := strings.TrimSpace(task.ResultText)
	if text == "" {
		return fmt.Errorf("empty transcription result")
	}

	correctedText := text
	if p.corrector != nil {
		value, _, err := p.corrector.Correct(ctx, task.DictID, text)
		if err != nil {
			return err
		}
		correctedText = value
	}

	meeting := &meetingdomain.Meeting{
		SourceTaskID: &task.ID,
		UserID:       task.UserID,
		Title:        buildMeetingTitle(task),
		AudioURL:     task.AudioURL,
		Duration:     task.Duration,
		Status:       meetingdomain.MeetingStatusCompleted,
	}
	if err := p.meetingRepo.Create(ctx, meeting); err != nil {
		return err
	}

	if err := p.transcriptRepo.BatchCreate(ctx, []meetingdomain.Transcript{{
		MeetingID:    meeting.ID,
		SpeakerLabel: "ASR",
		Text:         correctedText,
		StartTime:    0,
		EndTime:      task.Duration,
	}}); err != nil {
		return err
	}

	if p.summarizer != nil {
		content, modelVersion, err := p.summarizer.Summarize(ctx, correctedText)
		if err != nil {
			return err
		}
		if strings.TrimSpace(content) != "" {
			if err := p.summaryRepo.Create(ctx, &meetingdomain.Summary{
				MeetingID:    meeting.ID,
				Content:      content,
				ModelVersion: modelVersion,
			}); err != nil {
				return err
			}
		}
	}

	task.MeetingID = &meeting.ID
	task.ResultText = correctedText
	return nil
}

// ResumeCompletedTaskFromFailure falls back to the legacy full post-process path.
func (p *BatchMeetingProcessor) ResumeCompletedTaskFromFailure(ctx context.Context, task *asrdomain.TranscriptionTask) error {
	return p.ProcessCompletedTask(ctx, task)
}

func buildMeetingTitle(task *asrdomain.TranscriptionTask) string {
	fileName := path.Base(strings.TrimSpace(task.AudioURL))
	if fileName == "." || fileName == "/" || fileName == "" {
		return fmt.Sprintf("批量转写任务 #%d", task.ID)
	}
	return fmt.Sprintf("批量转写 %s", fileName)
}

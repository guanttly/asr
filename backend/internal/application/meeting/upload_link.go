package meeting

import (
	"context"
	"fmt"
	"strings"
	"time"

	appasr "github.com/lgt/asr/internal/application/asr"
	domain "github.com/lgt/asr/internal/domain/meeting"
)

// UploadingMeetingParams describes a meeting promoted from an in-progress,
// resumable upload session that has reached the minimum recording duration.
type UploadingMeetingParams struct {
	UserID          uint64
	UploadSessionID uint64
	Title           string
	WorkflowID      *uint64
	Language        string
	Duration        float64
}

// FinalizeUploadedParams attaches assembled audio to a meeting and starts the
// transcription pipeline.
type FinalizeUploadedParams struct {
	MeetingID     uint64
	AudioURL      string
	LocalFilePath string
	Duration      float64
	Language      string
}

// UploadedMeetingParams creates a fully-uploaded meeting directly (audio already
// assembled) and dispatches the transcription pipeline.
type UploadedMeetingParams struct {
	UserID          uint64
	UploadSessionID uint64
	Title           string
	WorkflowID      *uint64
	Language        string
	AudioURL        string
	LocalFilePath   string
	Duration        float64
}

// CreateUploadingMeeting creates a meeting in the "uploading" state for a
// recording session that has reached the minimum duration. No audio is attached
// yet and the transcription pipeline is NOT dispatched until the upload
// completes. This makes a long recording immediately visible (and durable) so a
// mid-recording disconnect leaves a resumable meeting instead of losing data.
func (s *Service) CreateUploadingMeeting(ctx context.Context, p UploadingMeetingParams) (uint64, error) {
	language, err := appasr.NormalizeLanguage(p.Language)
	if err != nil {
		return 0, err
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = defaultMeetingTitle(time.Now())
	}
	uploadSessionID := p.UploadSessionID
	m := &domain.Meeting{
		UserID:          p.UserID,
		UploadSessionID: &uploadSessionID,
		Title:           title,
		AudioURL:        "",
		Duration:        p.Duration,
		Language:        language,
		WorkflowID:      p.WorkflowID,
		Status:          domain.MeetingStatusUploading,
	}
	if err := s.meetingRepo.Create(ctx, m); err != nil {
		return 0, err
	}
	s.publishMeetingUpdated(m)
	return m.ID, nil
}

// FinalizeUploadedMeeting transitions a previously-promoted meeting from
// "uploading"/"interrupted" to "uploaded", attaches the assembled audio, and
// dispatches the transcription pipeline.
func (s *Service) FinalizeUploadedMeeting(ctx context.Context, p FinalizeUploadedParams) error {
	meeting, err := s.meetingRepo.GetByID(ctx, p.MeetingID)
	if err != nil {
		return err
	}
	audioURL := strings.TrimSpace(p.AudioURL)
	if audioURL == "" {
		return fmt.Errorf("audio_url is required")
	}
	meeting.AudioURL = audioURL
	if localPath := strings.TrimSpace(p.LocalFilePath); localPath != "" {
		meeting.LocalFilePath = localPath
	}
	if p.Duration > 0 {
		meeting.Duration = p.Duration
	}
	if lang, err := appasr.NormalizeLanguage(p.Language); err == nil && lang != "" {
		meeting.Language = lang
	}
	meeting.Status = domain.MeetingStatusUploaded
	meeting.SyncFailCount = 0
	meeting.LastSyncError = ""
	if err := s.meetingRepo.Update(ctx, meeting); err != nil {
		return err
	}
	s.publishMeetingUpdated(meeting)
	if s.batchSubmitter != nil {
		s.dispatchMeetingTask(meeting.ID)
	}
	return nil
}

// CreateUploadedMeeting creates a fully-uploaded meeting in a single step (audio
// already assembled) and dispatches the transcription pipeline. Used when an
// upload completes for a session that was never promoted to a meeting.
func (s *Service) CreateUploadedMeeting(ctx context.Context, p UploadedMeetingParams) (uint64, error) {
	language, err := appasr.NormalizeLanguage(p.Language)
	if err != nil {
		return 0, err
	}
	audioURL := strings.TrimSpace(p.AudioURL)
	if audioURL == "" {
		return 0, fmt.Errorf("audio_url is required")
	}
	title := strings.TrimSpace(p.Title)
	if title == "" {
		title = defaultMeetingTitle(time.Now())
	}
	uploadSessionID := p.UploadSessionID
	m := &domain.Meeting{
		UserID:          p.UserID,
		UploadSessionID: &uploadSessionID,
		Title:           title,
		AudioURL:        audioURL,
		LocalFilePath:   strings.TrimSpace(p.LocalFilePath),
		Duration:        p.Duration,
		Language:        language,
		WorkflowID:      p.WorkflowID,
		Status:          domain.MeetingStatusUploaded,
	}
	if err := s.meetingRepo.Create(ctx, m); err != nil {
		return 0, err
	}
	s.publishMeetingUpdated(m)
	if s.batchSubmitter != nil {
		s.dispatchMeetingTask(m.ID)
	}
	return m.ID, nil
}

// MarkMeetingInterrupted flags an uploading meeting whose client lost its
// heartbeat. The meeting is preserved (pending resume / server recovery) and is
// excluded from the sync loop until finalized.
func (s *Service) MarkMeetingInterrupted(ctx context.Context, meetingID uint64) error {
	meeting, err := s.meetingRepo.GetByID(ctx, meetingID)
	if err != nil {
		return err
	}
	// Only an actively-uploading meeting can become interrupted; ignore meetings
	// that already advanced (uploaded/processing/completed/failed).
	if meeting.Status != domain.MeetingStatusUploading {
		return nil
	}
	meeting.Status = domain.MeetingStatusInterrupted
	return s.meetingRepo.Update(ctx, meeting)
}

// DiscardMeeting removes a meeting and its derived data without owner/status
// checks. Used by the upload pipeline when a promoted recording is explicitly
// aborted by the client.
func (s *Service) DiscardMeeting(ctx context.Context, meetingID uint64) error {
	if meetingID == 0 {
		return nil
	}
	if s.summaryRepo != nil {
		if err := s.summaryRepo.DeleteByMeeting(ctx, meetingID); err != nil {
			return err
		}
	}
	if s.transcriptRepo != nil {
		if err := s.transcriptRepo.DeleteByMeeting(ctx, meetingID); err != nil {
			return err
		}
	}
	return s.meetingRepo.Delete(ctx, meetingID)
}

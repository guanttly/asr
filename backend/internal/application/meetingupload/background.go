package meetingupload

import (
	"context"
	"time"

	domain "github.com/lgt/asr/internal/domain/meetingupload"
)

// RecoverSummary reports one recovery pass.
type RecoverSummary struct {
	Interrupted int
	Finalized   int
	Aborted     int
	Failed      int
}

// CleanupSummary reports one cleanup pass.
type CleanupSummary struct {
	Reclaimed int
	Failed    int
}

// RecoverInterrupted is the server-side safety net that guarantees a recording
// is never lost even if the client never returns. It (1) marks silent recording
// sessions as interrupted and (2) finalizes interrupted sessions that hold
// enough audio, pushing them into the transcription pipeline.
func (s *Service) RecoverInterrupted(ctx context.Context) (RecoverSummary, error) {
	var summary RecoverSummary
	now := time.Now()
	staleBefore := now.Add(-s.cfg.InactiveTimeout)

	stale, err := s.repo.ListStaleRecording(ctx, staleBefore, s.cfg.RecoverBatchSize)
	if err != nil {
		return summary, err
	}
	for _, session := range stale {
		session.Status = domain.SessionStatusInterrupted
		if err := s.repo.UpdateSession(ctx, session); err != nil {
			s.logger.Errorf("meetingupload: mark interrupted %s failed: %v", session.UploadID, err)
			continue
		}
		summary.Interrupted++
		if session.MeetingID != nil {
			if err := s.finalizer.MarkMeetingInterrupted(ctx, *session.MeetingID); err != nil {
				s.logger.Warnf("meetingupload: mark meeting %d interrupted failed: %v", *session.MeetingID, err)
			}
		}
		s.logger.Infof("meetingupload: session %s lost heartbeat, marked interrupted (duration %.1fs)", session.UploadID, session.DurationSec)
	}

	interrupted, err := s.repo.ListRecoverable(ctx, []domain.SessionStatus{domain.SessionStatusInterrupted}, s.cfg.RecoverBatchSize)
	if err != nil {
		return summary, err
	}
	for _, session := range interrupted {
		// Give a client the full inactivity window to resume before the server
		// takes over.
		if session.LastSeenAt.After(staleBefore) {
			continue
		}
		if err := s.recoverOne(ctx, session, &summary); err != nil {
			summary.Failed++
			s.logger.Errorf("meetingupload: recover session %s failed: %v", session.UploadID, err)
		}
	}
	return summary, nil
}

func (s *Service) recoverOne(ctx context.Context, session *domain.UploadSession, summary *RecoverSummary) error {
	lock := s.lockFor(session.UploadID)
	lock.Lock()
	defer lock.Unlock()

	// Re-read under the lock to avoid racing a resuming client.
	fresh, err := s.repo.GetSessionByID(ctx, session.ID)
	if err != nil {
		return err
	}
	if fresh == nil || fresh.Status != domain.SessionStatusInterrupted {
		return nil
	}

	segments, err := s.repo.ListSegments(ctx, fresh.ID)
	if err != nil {
		return err
	}
	if len(segments) == 0 || fresh.DurationSec < s.cfg.MinRecordingDuration.Seconds() {
		if _, err := s.abortLocked(ctx, fresh); err != nil {
			return err
		}
		summary.Aborted++
		s.logger.Infof("meetingupload: interrupted session %s too short, aborted", fresh.UploadID)
		return nil
	}

	fresh.Status = domain.SessionStatusCompleting
	if err := s.repo.UpdateSession(ctx, fresh); err != nil {
		return err
	}
	if _, err := s.assembleAndFinalize(ctx, fresh, segments); err != nil {
		return err
	}
	summary.Finalized++
	s.logger.Infof("meetingupload: recovered interrupted session %s into meeting (duration %.1fs)", fresh.UploadID, fresh.DurationSec)
	return nil
}

// Cleanup reclaims temp segments and stale session rows. It NEVER touches an
// actively-recording or completing session, nor a recording whose heartbeat is
// still fresh — only terminal/aborted/failed sessions and interrupted ones that
// have exceeded the resume retention window.
func (s *Service) Cleanup(ctx context.Context) (CleanupSummary, error) {
	var summary CleanupSummary
	now := time.Now()
	q := domain.CleanupQuery{
		AbortedBefore:     now.Add(-s.cfg.AbortedRetention),
		CompletedBefore:   now.Add(-s.cfg.CompletedTempRetention),
		FailedBefore:      now.Add(-s.cfg.ResumeRetention),
		InterruptedBefore: now.Add(-s.cfg.ResumeRetention),
	}
	candidates, err := s.repo.ListCleanupCandidates(ctx, q, s.cfg.CleanupBatchSize)
	if err != nil {
		return summary, err
	}
	for _, session := range candidates {
		if err := s.reclaimSession(ctx, session); err != nil {
			summary.Failed++
			s.logger.Errorf("meetingupload: cleanup session %s failed: %v", session.UploadID, err)
			continue
		}
		summary.Reclaimed++
	}
	if summary.Reclaimed > 0 || summary.Failed > 0 {
		s.logger.Infof("meetingupload: cleanup reclaimed %d session(s), %d failed", summary.Reclaimed, summary.Failed)
	}
	return summary, nil
}

func (s *Service) reclaimSession(ctx context.Context, session *domain.UploadSession) error {
	lock := s.lockFor(session.UploadID)
	lock.Lock()
	defer lock.Unlock()

	fresh, err := s.repo.GetSessionByID(ctx, session.ID)
	if err != nil {
		return err
	}
	if fresh == nil {
		return nil
	}
	// Never reclaim a session that became active again (resumed/recording).
	if fresh.Status == domain.SessionStatusRecording || fresh.Status == domain.SessionStatusCompleting {
		return nil
	}

	// An interrupted session that survived to its retention cutoff without
	// recovery: drop the orphaned (audio-less) meeting too.
	if fresh.Status == domain.SessionStatusInterrupted && fresh.MeetingID != nil {
		if err := s.finalizer.DiscardMeeting(ctx, *fresh.MeetingID); err != nil {
			s.logger.Warnf("meetingupload: discard orphaned meeting %d failed: %v", *fresh.MeetingID, err)
		}
	}

	s.reclaimSegments(ctx, fresh)
	return s.repo.DeleteSession(ctx, fresh.ID)
}

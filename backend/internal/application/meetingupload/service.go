package meetingupload

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	appasr "github.com/lgt/asr/internal/application/asr"
	appmeeting "github.com/lgt/asr/internal/application/meeting"
	domain "github.com/lgt/asr/internal/domain/meetingupload"
)

// InitParams starts a new resumable upload session.
type InitParams struct {
	UserID        uint64
	Filename      string
	Title         string
	WorkflowID    *uint64
	Language      string
	PublicBaseURL string
}

// InitResult is returned to the client after init.
type InitResult struct {
	UploadID     string
	NextIndex    int
	MaxChunkSize int64
}

// AppendParams uploads one raw-PCM chunk.
type AppendParams struct {
	UserID   uint64
	UploadID string
	Index    int
	Checksum string
	Body     io.Reader
}

// AppendResult reports the post-append session cursor.
type AppendResult struct {
	NextIndex   int
	Received    int64
	DurationSec float64
	Status      string
	MeetingID   *uint64
	Duplicate   bool
}

// StateResult is the public view of a session used by heartbeat/get.
type StateResult struct {
	UploadID    string
	Status      string
	NextIndex   int
	DurationSec float64
	TotalBytes  int64
	MeetingID   *uint64
}

// CompleteResult is returned after assembling the final audio.
type CompleteResult struct {
	MeetingID   *uint64
	Status      string
	DurationSec float64
}

func (s *Service) segmentsDir(uploadID string) string {
	return filepath.Join(s.cfg.UploadDir, s.cfg.SegmentsSubdir, uploadID)
}

func (s *Service) segmentPath(uploadID string, index int) string {
	return filepath.Join(s.segmentsDir(uploadID), fmt.Sprintf("%08d.pcm", index))
}

func (s *Service) audioRelPath(uploadID string) string {
	return path.Join(s.cfg.AudioSubdir, uploadID+".wav")
}

func (s *Service) audioAbsPath(uploadID string) string {
	return filepath.Join(s.cfg.UploadDir, s.cfg.AudioSubdir, uploadID+".wav")
}

// Init creates a new recording session and its on-disk segment directory.
func (s *Service) Init(ctx context.Context, p InitParams) (*InitResult, error) {
	language, err := appasr.NormalizeLanguage(p.Language)
	if err != nil {
		return nil, newError(http.StatusBadRequest, err.Error())
	}
	uploadID := newUploadID()
	if err := os.MkdirAll(s.segmentsDir(uploadID), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create upload directory: %w", err)
	}

	now := time.Now()
	session := &domain.UploadSession{
		UploadID:      uploadID,
		UserID:        p.UserID,
		Status:        domain.SessionStatusRecording,
		Format:        domain.FormatPCMS16LE16kMono,
		Filename:      strings.TrimSpace(p.Filename),
		Title:         strings.TrimSpace(p.Title),
		WorkflowID:    p.WorkflowID,
		Language:      language,
		PublicBaseURL: strings.TrimRight(strings.TrimSpace(p.PublicBaseURL), "/"),
		StartedAt:     now,
		LastSeenAt:    now,
	}
	if err := s.repo.CreateSession(ctx, session); err != nil {
		_ = os.RemoveAll(s.segmentsDir(uploadID))
		return nil, err
	}
	return &InitResult{UploadID: uploadID, NextIndex: 0, MaxChunkSize: s.cfg.MaxChunkBytes}, nil
}

func (s *Service) loadOwnedSession(ctx context.Context, uploadID string, userID uint64) (*domain.UploadSession, error) {
	uploadID = strings.TrimSpace(uploadID)
	if uploadID == "" {
		return nil, newError(http.StatusBadRequest, "missing upload id")
	}
	session, err := s.repo.GetSessionByUploadID(ctx, uploadID)
	if err != nil {
		return nil, err
	}
	if session == nil || session.UserID != userID {
		return nil, newError(http.StatusNotFound, "upload session not found")
	}
	return session, nil
}

// AppendChunk durably stores one chunk and advances the session cursor. It is
// idempotent: re-sending an already-stored index returns success (unless the
// checksum differs, which is a conflict), and gaps are rejected so the assembled
// audio is always correct.
func (s *Service) AppendChunk(ctx context.Context, p AppendParams) (*AppendResult, error) {
	if p.Index < 0 {
		return nil, newError(http.StatusBadRequest, "invalid chunk index")
	}
	lock := s.lockFor(p.UploadID)
	lock.Lock()
	defer lock.Unlock()

	session, err := s.loadOwnedSession(ctx, p.UploadID, p.UserID)
	if err != nil {
		return nil, err
	}
	if session.IsTerminal() {
		return nil, newError(http.StatusConflict, fmt.Sprintf("upload session already %s", session.Status))
	}

	// A heartbeat-lost session that receives a chunk is being resumed.
	if session.Status == domain.SessionStatusInterrupted {
		session.Status = domain.SessionStatusRecording
	}

	// Duplicate retransmit of an already-stored chunk.
	if p.Index < session.NextIndex {
		existing, err := s.repo.GetSegment(ctx, session.ID, p.Index)
		if err != nil {
			return nil, err
		}
		if existing != nil && p.Checksum != "" && existing.Checksum != "" && !strings.EqualFold(existing.Checksum, p.Checksum) {
			return nil, newError(http.StatusConflict, fmt.Sprintf("chunk %d checksum mismatch", p.Index))
		}
		session.LastSeenAt = time.Now()
		if err := s.repo.UpdateSession(ctx, session); err != nil {
			return nil, err
		}
		return &AppendResult{
			NextIndex:   session.NextIndex,
			Received:    session.TotalBytes,
			DurationSec: session.DurationSec,
			Status:      string(session.Status),
			MeetingID:   session.MeetingID,
			Duplicate:   true,
		}, nil
	}

	// Out-of-order chunk: client must resync via GET before continuing.
	if p.Index > session.NextIndex {
		return nil, newError(http.StatusConflict, fmt.Sprintf("unexpected chunk index %d, expected %d", p.Index, session.NextIndex))
	}

	// Recover from a half-applied previous attempt where the segment row was
	// written but the session cursor never advanced.
	if existing, err := s.repo.GetSegment(ctx, session.ID, p.Index); err != nil {
		return nil, err
	} else if existing != nil {
		if p.Checksum != "" && existing.Checksum != "" && !strings.EqualFold(existing.Checksum, p.Checksum) {
			return nil, newError(http.StatusConflict, fmt.Sprintf("chunk %d checksum mismatch", p.Index))
		}
		return s.advanceWithSegment(ctx, session, existing)
	}

	bytesWritten, checksum, err := s.writeSegmentFile(s.segmentPath(p.UploadID, p.Index), p.Body)
	if err != nil {
		return nil, err
	}
	if bytesWritten == 0 {
		_ = os.Remove(s.segmentPath(p.UploadID, p.Index))
		return nil, newError(http.StatusBadRequest, "empty chunk")
	}
	if p.Checksum != "" && !strings.EqualFold(checksum, p.Checksum) {
		_ = os.Remove(s.segmentPath(p.UploadID, p.Index))
		return nil, newError(http.StatusBadRequest, fmt.Sprintf("chunk %d checksum mismatch", p.Index))
	}
	if session.TotalBytes+bytesWritten > s.cfg.MaxSessionBytes {
		_ = os.Remove(s.segmentPath(p.UploadID, p.Index))
		return nil, newError(http.StatusRequestEntityTooLarge, fmt.Sprintf("录音总大小不能超过 %d MB", s.cfg.MaxSessionBytes/1024/1024))
	}

	segment := &domain.UploadSegment{
		UploadSessionID: session.ID,
		SegmentIndex:    p.Index,
		Path:            s.segmentPath(p.UploadID, p.Index),
		Bytes:           bytesWritten,
		DurationSec:     durationFromBytes(bytesWritten),
		Checksum:        checksum,
		Status:          domain.SegmentStatusStored,
	}
	if err := s.repo.CreateSegment(ctx, segment); err != nil {
		_ = os.Remove(s.segmentPath(p.UploadID, p.Index))
		return nil, err
	}
	return s.advanceWithSegment(ctx, session, segment)
}

// advanceWithSegment moves the session cursor past segment, promotes the session
// to a real meeting once it crosses the minimum duration, and persists state.
func (s *Service) advanceWithSegment(ctx context.Context, session *domain.UploadSession, segment *domain.UploadSegment) (*AppendResult, error) {
	session.TotalBytes += segment.Bytes
	session.NextIndex = segment.SegmentIndex + 1
	session.DurationSec = durationFromBytes(session.TotalBytes)
	session.LastSeenAt = time.Now()
	session.Status = domain.SessionStatusRecording

	if session.MeetingID == nil && session.DurationSec >= s.cfg.MinRecordingDuration.Seconds() {
		meetingID, err := s.finalizer.CreateUploadingMeeting(ctx, appmeeting.UploadingMeetingParams{
			UserID:          session.UserID,
			UploadSessionID: session.ID,
			Title:           session.Title,
			WorkflowID:      session.WorkflowID,
			Language:        session.Language,
			Duration:        session.DurationSec,
		})
		if err != nil {
			s.logger.Errorf("meetingupload: promote session %s failed: %v", session.UploadID, err)
		} else {
			session.MeetingID = &meetingID
		}
	}

	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}
	return &AppendResult{
		NextIndex:   session.NextIndex,
		Received:    session.TotalBytes,
		DurationSec: session.DurationSec,
		Status:      string(session.Status),
		MeetingID:   session.MeetingID,
	}, nil
}

// writeSegmentFile streams body to dstPath (truncating any partial leftover),
// enforcing the per-chunk size cap and returning the byte count and SHA-256.
func (s *Service) writeSegmentFile(dstPath string, body io.Reader) (int64, string, error) {
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return 0, "", fmt.Errorf("failed to prepare segment directory: %w", err)
	}
	file, err := os.Create(dstPath)
	if err != nil {
		return 0, "", fmt.Errorf("failed to create segment file: %w", err)
	}
	defer file.Close()

	hasher := sha256New()
	limited := io.LimitReader(body, s.cfg.MaxChunkBytes+1)
	written, err := io.Copy(io.MultiWriter(file, hasher), limited)
	if err != nil {
		return 0, "", fmt.Errorf("failed to write chunk: %w", err)
	}
	if written > s.cfg.MaxChunkBytes {
		return 0, "", newError(http.StatusRequestEntityTooLarge, fmt.Sprintf("单个分片不能超过 %d MB", s.cfg.MaxChunkBytes/1024/1024))
	}
	if err := file.Sync(); err != nil {
		return 0, "", fmt.Errorf("failed to flush chunk: %w", err)
	}
	return written, hashHex(hasher), nil
}

// Heartbeat keeps a recording session alive (and resumes an interrupted one).
func (s *Service) Heartbeat(ctx context.Context, userID uint64, uploadID string) (*StateResult, error) {
	lock := s.lockFor(uploadID)
	lock.Lock()
	defer lock.Unlock()

	session, err := s.loadOwnedSession(ctx, uploadID, userID)
	if err != nil {
		return nil, err
	}
	if session.IsTerminal() {
		return sessionState(session), nil
	}
	if session.Status == domain.SessionStatusInterrupted {
		session.Status = domain.SessionStatusRecording
	}
	session.LastSeenAt = time.Now()
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}
	return sessionState(session), nil
}

// Get returns the durable state of a session so a client can resync after a
// restart (knowing exactly which chunk index to resume from).
func (s *Service) Get(ctx context.Context, userID uint64, uploadID string) (*StateResult, error) {
	session, err := s.loadOwnedSession(ctx, uploadID, userID)
	if err != nil {
		return nil, err
	}
	return sessionState(session), nil
}

// Complete assembles the uploaded chunks into a single WAV and pushes the
// meeting into the transcription pipeline.
func (s *Service) Complete(ctx context.Context, userID uint64, uploadID string) (*CompleteResult, error) {
	lock := s.lockFor(uploadID)
	lock.Lock()
	defer lock.Unlock()

	session, err := s.loadOwnedSession(ctx, uploadID, userID)
	if err != nil {
		return nil, err
	}
	switch session.Status {
	case domain.SessionStatusCompleted:
		return &CompleteResult{MeetingID: session.MeetingID, Status: string(session.Status), DurationSec: session.DurationSec}, nil
	case domain.SessionStatusAborted, domain.SessionStatusExpired, domain.SessionStatusFailed:
		return nil, newError(http.StatusConflict, fmt.Sprintf("upload session already %s", session.Status))
	}

	segments, err := s.repo.ListSegments(ctx, session.ID)
	if err != nil {
		return nil, err
	}
	if missing := missingIndices(segments, session.NextIndex); len(missing) > 0 {
		return nil, &MissingChunksError{Missing: missing}
	}
	if len(segments) == 0 {
		// Nothing was recorded; treat as a discard.
		return s.abortLocked(ctx, session)
	}

	session.Status = domain.SessionStatusCompleting
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}

	return s.assembleAndFinalize(ctx, session, segments)
}

// assembleAndFinalize writes the WAV file, attaches it to the meeting (creating
// one if the session was never promoted), and marks the session completed.
func (s *Service) assembleAndFinalize(ctx context.Context, session *domain.UploadSession, segments []*domain.UploadSegment) (*CompleteResult, error) {
	dataBytes, err := s.assembleWAV(session.UploadID, segments)
	if err != nil {
		s.markFailed(ctx, session)
		return nil, err
	}
	duration := durationFromBytes(dataBytes)

	if duration < s.cfg.MinRecordingDuration.Seconds() {
		// Below the threshold after assembly: discard rather than create a meeting.
		_ = os.Remove(s.audioAbsPath(session.UploadID))
		return s.abortLocked(ctx, session)
	}

	audioURL, err := buildAudioURL(session.PublicBaseURL, s.audioRelPath(session.UploadID))
	if err != nil {
		s.markFailed(ctx, session)
		return nil, err
	}

	var meetingID uint64
	if session.MeetingID != nil {
		meetingID = *session.MeetingID
		if err := s.finalizer.FinalizeUploadedMeeting(ctx, appmeeting.FinalizeUploadedParams{
			MeetingID:     meetingID,
			AudioURL:      audioURL,
			LocalFilePath: s.audioAbsPath(session.UploadID),
			Duration:      duration,
			Language:      session.Language,
		}); err != nil {
			s.markFailed(ctx, session)
			return nil, err
		}
	} else {
		created, err := s.finalizer.CreateUploadedMeeting(ctx, appmeeting.UploadedMeetingParams{
			UserID:          session.UserID,
			UploadSessionID: session.ID,
			Title:           session.Title,
			WorkflowID:      session.WorkflowID,
			Language:        session.Language,
			AudioURL:        audioURL,
			LocalFilePath:   s.audioAbsPath(session.UploadID),
			Duration:        duration,
		})
		if err != nil {
			s.markFailed(ctx, session)
			return nil, err
		}
		meetingID = created
		session.MeetingID = &meetingID
	}

	now := time.Now()
	session.Status = domain.SessionStatusCompleted
	session.DurationSec = duration
	session.TotalBytes = dataBytes
	session.CompletedAt = &now
	session.StoppedAt = &now
	session.LastSeenAt = now
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		s.logger.Errorf("meetingupload: persist completed session %s failed: %v", session.UploadID, err)
	}
	return &CompleteResult{MeetingID: &meetingID, Status: string(domain.SessionStatusCompleted), DurationSec: duration}, nil
}

// assembleWAV writes a 16k/mono/s16le WAV (header + concatenated segments) and
// returns the PCM byte count.
func (s *Service) assembleWAV(uploadID string, segments []*domain.UploadSegment) (int64, error) {
	var dataBytes int64
	for _, seg := range segments {
		dataBytes += seg.Bytes
	}

	dstPath := s.audioAbsPath(uploadID)
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return 0, fmt.Errorf("failed to prepare audio directory: %w", err)
	}
	out, err := os.Create(dstPath)
	if err != nil {
		return 0, fmt.Errorf("failed to create audio file: %w", err)
	}
	defer out.Close()

	if err := writeWAVHeader(out, dataBytes); err != nil {
		return 0, err
	}
	for _, seg := range segments {
		if err := appendFile(out, seg.Path); err != nil {
			return 0, fmt.Errorf("failed to assemble segment %d: %w", seg.SegmentIndex, err)
		}
	}
	if err := out.Sync(); err != nil {
		return 0, fmt.Errorf("failed to flush audio: %w", err)
	}
	return dataBytes, nil
}

// Abort discards an in-progress upload. If the recording was already promoted to
// a meeting, that meeting is removed (explicit user discard).
func (s *Service) Abort(ctx context.Context, userID uint64, uploadID string) error {
	lock := s.lockFor(uploadID)
	lock.Lock()
	defer lock.Unlock()

	session, err := s.loadOwnedSession(ctx, uploadID, userID)
	if err != nil {
		return err
	}
	if session.IsTerminal() {
		return nil
	}
	_, err = s.abortLocked(ctx, session)
	return err
}

func (s *Service) abortLocked(ctx context.Context, session *domain.UploadSession) (*CompleteResult, error) {
	if session.MeetingID != nil {
		if err := s.finalizer.DiscardMeeting(ctx, *session.MeetingID); err != nil {
			s.logger.Warnf("meetingupload: discard meeting %d for aborted session %s failed: %v", *session.MeetingID, session.UploadID, err)
		}
		session.MeetingID = nil
	}
	now := time.Now()
	session.Status = domain.SessionStatusAborted
	session.AbortedAt = &now
	session.StoppedAt = &now
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		return nil, err
	}
	s.reclaimSegments(ctx, session)
	return &CompleteResult{MeetingID: nil, Status: string(domain.SessionStatusAborted), DurationSec: session.DurationSec}, nil
}

func (s *Service) markFailed(ctx context.Context, session *domain.UploadSession) {
	session.Status = domain.SessionStatusFailed
	if err := s.repo.UpdateSession(ctx, session); err != nil {
		s.logger.Errorf("meetingupload: mark session %s failed: %v", session.UploadID, err)
	}
}

// reclaimSegments removes the on-disk temp segments and their rows.
func (s *Service) reclaimSegments(ctx context.Context, session *domain.UploadSession) {
	if err := os.RemoveAll(s.segmentsDir(session.UploadID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.logger.Warnf("meetingupload: remove segments dir for %s failed: %v", session.UploadID, err)
	}
	if err := s.repo.DeleteSegments(ctx, session.ID); err != nil {
		s.logger.Warnf("meetingupload: delete segment rows for %s failed: %v", session.UploadID, err)
	}
}

func sessionState(session *domain.UploadSession) *StateResult {
	return &StateResult{
		UploadID:    session.UploadID,
		Status:      string(session.Status),
		NextIndex:   session.NextIndex,
		DurationSec: session.DurationSec,
		TotalBytes:  session.TotalBytes,
		MeetingID:   session.MeetingID,
	}
}

func missingIndices(segments []*domain.UploadSegment, nextIndex int) []int {
	present := make(map[int]struct{}, len(segments))
	for _, seg := range segments {
		present[seg.SegmentIndex] = struct{}{}
	}
	var missing []int
	for i := 0; i < nextIndex; i++ {
		if _, ok := present[i]; !ok {
			missing = append(missing, i)
		}
	}
	return missing
}

func writeWAVHeader(w io.Writer, dataBytes int64) error {
	var header [44]byte
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], uint32(36+dataBytes))
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1) // PCM
	binary.LittleEndian.PutUint16(header[22:24], audioChannels)
	binary.LittleEndian.PutUint32(header[24:28], audioSampleRate)
	binary.LittleEndian.PutUint32(header[28:32], audioByteRate)
	binary.LittleEndian.PutUint16(header[32:34], audioBytesPerSamp*audioChannels)
	binary.LittleEndian.PutUint16(header[34:36], 16) // bits per sample
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], uint32(dataBytes))
	_, err := w.Write(header[:])
	return err
}

func appendFile(dst io.Writer, srcPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()
	_, err = io.Copy(dst, src)
	return err
}

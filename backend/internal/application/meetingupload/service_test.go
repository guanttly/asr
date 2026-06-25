package meetingupload

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	appmeeting "github.com/lgt/asr/internal/application/meeting"
	domain "github.com/lgt/asr/internal/domain/meetingupload"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

// fakeRepo is an in-memory implementation of domain.Repository that mirrors the
// query semantics of the real GORM repository closely enough to exercise the
// recovery and cleanup logic.
type fakeRepo struct {
	mu            sync.Mutex
	nextSessionID uint64
	nextSegmentID uint64
	sessions      map[uint64]*domain.UploadSession
	byUpload      map[string]uint64
	segments      map[uint64]map[int]*domain.UploadSegment
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		sessions: map[uint64]*domain.UploadSession{},
		byUpload: map[string]uint64{},
		segments: map[uint64]map[int]*domain.UploadSegment{},
	}
}

func cloneSession(s *domain.UploadSession) *domain.UploadSession {
	cp := *s
	return &cp
}

func cloneSegment(s *domain.UploadSegment) *domain.UploadSegment {
	cp := *s
	return &cp
}

func (r *fakeRepo) CreateSession(_ context.Context, session *domain.UploadSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSessionID++
	session.ID = r.nextSessionID
	now := time.Now()
	session.CreatedAt = now
	session.UpdatedAt = now
	r.sessions[session.ID] = cloneSession(session)
	r.byUpload[session.UploadID] = session.ID
	r.segments[session.ID] = map[int]*domain.UploadSegment{}
	return nil
}

func (r *fakeRepo) GetSessionByUploadID(_ context.Context, uploadID string) (*domain.UploadSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byUpload[uploadID]
	if !ok {
		return nil, nil
	}
	return cloneSession(r.sessions[id]), nil
}

func (r *fakeRepo) GetSessionByID(_ context.Context, id uint64) (*domain.UploadSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s, ok := r.sessions[id]
	if !ok {
		return nil, nil
	}
	return cloneSession(s), nil
}

func (r *fakeRepo) UpdateSession(_ context.Context, session *domain.UploadSession) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	session.UpdatedAt = time.Now()
	r.sessions[session.ID] = cloneSession(session)
	return nil
}

func (r *fakeRepo) DeleteSession(_ context.Context, id uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if s, ok := r.sessions[id]; ok {
		delete(r.byUpload, s.UploadID)
	}
	delete(r.sessions, id)
	delete(r.segments, id)
	return nil
}

func (r *fakeRepo) CreateSegment(_ context.Context, segment *domain.UploadSegment) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.nextSegmentID++
	segment.ID = r.nextSegmentID
	now := time.Now()
	segment.CreatedAt = now
	segment.UpdatedAt = now
	if r.segments[segment.UploadSessionID] == nil {
		r.segments[segment.UploadSessionID] = map[int]*domain.UploadSegment{}
	}
	r.segments[segment.UploadSessionID][segment.SegmentIndex] = cloneSegment(segment)
	return nil
}

func (r *fakeRepo) GetSegment(_ context.Context, sessionID uint64, index int) (*domain.UploadSegment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	seg, ok := r.segments[sessionID][index]
	if !ok {
		return nil, nil
	}
	return cloneSegment(seg), nil
}

func (r *fakeRepo) ListSegments(_ context.Context, sessionID uint64) ([]*domain.UploadSegment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*domain.UploadSegment, 0, len(r.segments[sessionID]))
	for _, seg := range r.segments[sessionID] {
		out = append(out, cloneSegment(seg))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SegmentIndex < out[j].SegmentIndex })
	return out, nil
}

func (r *fakeRepo) DeleteSegments(_ context.Context, sessionID uint64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.segments[sessionID] = map[int]*domain.UploadSegment{}
	return nil
}

func (r *fakeRepo) ListStaleRecording(_ context.Context, lastSeenBefore time.Time, limit int) ([]*domain.UploadSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.UploadSession
	for _, s := range r.sessions {
		if s.Status == domain.SessionStatusRecording && s.LastSeenAt.Before(lastSeenBefore) {
			out = append(out, cloneSession(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeenAt.Before(out[j].LastSeenAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeRepo) ListRecoverable(_ context.Context, statuses []domain.SessionStatus, limit int) ([]*domain.UploadSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	want := map[domain.SessionStatus]struct{}{}
	for _, s := range statuses {
		want[s] = struct{}{}
	}
	var out []*domain.UploadSession
	for _, s := range r.sessions {
		if _, ok := want[s.Status]; ok {
			out = append(out, cloneSession(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastSeenAt.Before(out[j].LastSeenAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (r *fakeRepo) ListCleanupCandidates(_ context.Context, q domain.CleanupQuery, limit int) ([]*domain.UploadSession, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*domain.UploadSession
	for _, s := range r.sessions {
		match := false
		switch s.Status {
		case domain.SessionStatusAborted:
			match = !q.AbortedBefore.IsZero() && s.UpdatedAt.Before(q.AbortedBefore)
		case domain.SessionStatusCompleted:
			match = !q.CompletedBefore.IsZero() && s.CompletedAt != nil && s.CompletedAt.Before(q.CompletedBefore)
		case domain.SessionStatusFailed:
			match = !q.FailedBefore.IsZero() && s.UpdatedAt.Before(q.FailedBefore)
		case domain.SessionStatusInterrupted:
			match = !q.InterruptedBefore.IsZero() && s.LastSeenAt.Before(q.InterruptedBefore)
		}
		if match {
			out = append(out, cloneSession(s))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.Before(out[j].UpdatedAt) })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// test helpers on fakeRepo

func (r *fakeRepo) setLastSeen(uploadID string, t time.Time) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id, ok := r.byUpload[uploadID]; ok {
		r.sessions[id].LastSeenAt = t
	}
}

func (r *fakeRepo) dropSegment(sessionID uint64, index int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.segments[sessionID], index)
}

func (r *fakeRepo) segmentCount(sessionID uint64) int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.segments[sessionID])
}

// fakeFinalizer records the meeting lifecycle calls made by the upload service.
type fakeMeeting struct {
	status   string
	audioURL string
	duration float64
}

type fakeFinalizer struct {
	mu               sync.Mutex
	nextID           uint64
	meetings         map[uint64]*fakeMeeting
	createUploading  int
	finalizeUploaded int
	createUploaded   int
	interrupted      int
	discarded        int
}

func newFakeFinalizer() *fakeFinalizer {
	return &fakeFinalizer{meetings: map[uint64]*fakeMeeting{}}
}

func (f *fakeFinalizer) CreateUploadingMeeting(_ context.Context, p appmeeting.UploadingMeetingParams) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.meetings[f.nextID] = &fakeMeeting{status: "uploading", duration: p.Duration}
	f.createUploading++
	return f.nextID, nil
}

func (f *fakeFinalizer) FinalizeUploadedMeeting(_ context.Context, p appmeeting.FinalizeUploadedParams) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	m := f.meetings[p.MeetingID]
	if m == nil {
		m = &fakeMeeting{}
		f.meetings[p.MeetingID] = m
	}
	m.status = "uploaded"
	m.audioURL = p.AudioURL
	m.duration = p.Duration
	f.finalizeUploaded++
	return nil
}

func (f *fakeFinalizer) CreateUploadedMeeting(_ context.Context, p appmeeting.UploadedMeetingParams) (uint64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.nextID++
	f.meetings[f.nextID] = &fakeMeeting{status: "uploaded", audioURL: p.AudioURL, duration: p.Duration}
	f.createUploaded++
	return f.nextID, nil
}

func (f *fakeFinalizer) MarkMeetingInterrupted(_ context.Context, meetingID uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if m := f.meetings[meetingID]; m != nil {
		m.status = "interrupted"
	}
	f.interrupted++
	return nil
}

func (f *fakeFinalizer) DiscardMeeting(_ context.Context, meetingID uint64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.meetings, meetingID)
	f.discarded++
	return nil
}

func (f *fakeFinalizer) meeting(id uint64) *fakeMeeting {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.meetings[id]
}

// ---------------------------------------------------------------------------
// Harness
// ---------------------------------------------------------------------------

const (
	bytesPerSecond = audioByteRate // 32000 bytes == 1s of 16k/mono/s16le PCM
)

func newTestService(t *testing.T) (*Service, *fakeRepo, *fakeFinalizer, string) {
	t.Helper()
	dir := t.TempDir()
	repo := newFakeRepo()
	fin := newFakeFinalizer()
	cfg := Config{
		UploadDir:            dir,
		MinRecordingDuration: 5 * time.Second,
		InactiveTimeout:      30 * time.Minute,
		ResumeRetention:      7 * 24 * time.Hour,
		AbortedRetention:     time.Hour,
		MaxChunkBytes:        8 * 1024 * 1024,
		MaxSessionBytes:      4096 * 1024 * 1024,
	}
	return NewService(repo, fin, cfg, nil), repo, fin, dir
}

func pcm(seconds float64) []byte {
	return bytes.Repeat([]byte{0x01, 0x00}, int(float64(bytesPerSecond)*seconds)/2)
}

func checksumOf(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func appendChunk(t *testing.T, svc *Service, userID uint64, uploadID string, index int, data []byte) (*AppendResult, error) {
	t.Helper()
	return svc.AppendChunk(context.Background(), AppendParams{
		UserID:   userID,
		UploadID: uploadID,
		Index:    index,
		Checksum: checksumOf(data),
		Body:     bytes.NewReader(data),
	})
}

func mustInit(t *testing.T, svc *Service, userID uint64) *InitResult {
	t.Helper()
	res, err := svc.Init(context.Background(), InitParams{
		UserID:        userID,
		Filename:      "meeting.wav",
		Title:         "晨会",
		Language:      "zh",
		PublicBaseURL: "http://localhost:8080",
	})
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	return res
}

func asUploadError(t *testing.T, err error) *Error {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	ue, ok := err.(*Error)
	if !ok {
		t.Fatalf("expected *Error, got %T: %v", err, err)
	}
	return ue
}

// ---------------------------------------------------------------------------
// Tests covering the plan's acceptance criteria
// ---------------------------------------------------------------------------

// Recording stopped at 3s must NOT create a meeting; the data is discarded.
func TestComplete_ShortRecording_DiscardsWithoutMeeting(t *testing.T) {
	svc, repo, fin, _ := newTestService(t)
	const userID = 7
	init := mustInit(t, svc, userID)

	for i := 0; i < 3; i++ {
		if _, err := appendChunk(t, svc, userID, init.UploadID, i, pcm(1)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	res, err := svc.Complete(context.Background(), userID, init.UploadID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if res.Status != string(domain.SessionStatusAborted) {
		t.Fatalf("status = %s, want aborted", res.Status)
	}
	if res.MeetingID != nil {
		t.Fatalf("expected no meeting, got %d", *res.MeetingID)
	}
	if fin.createUploading != 0 || fin.createUploaded != 0 {
		t.Fatalf("no meeting should be created for a 3s recording (uploading=%d uploaded=%d)", fin.createUploading, fin.createUploaded)
	}
	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	if sess.Status != domain.SessionStatusAborted {
		t.Fatalf("session status = %s, want aborted", sess.Status)
	}
	if repo.segmentCount(sess.ID) != 0 {
		t.Fatalf("aborted session segments should be reclaimed")
	}
}

// A recording crossing 5s must be promoted to a meeting (status uploading) so it
// survives even before completion.
func TestAppend_PromotesToMeetingAtThreshold(t *testing.T) {
	svc, repo, fin, _ := newTestService(t)
	const userID = 9
	init := mustInit(t, svc, userID)

	// Four 1s chunks -> 4s, still below threshold, no meeting.
	for i := 0; i < 4; i++ {
		res, err := appendChunk(t, svc, userID, init.UploadID, i, pcm(1))
		if err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
		if res.MeetingID != nil {
			t.Fatalf("meeting promoted too early at %.1fs", res.DurationSec)
		}
	}
	if fin.createUploading != 0 {
		t.Fatalf("no meeting expected below 5s")
	}

	// Fifth chunk crosses 5s -> promote.
	res, err := appendChunk(t, svc, userID, init.UploadID, 4, pcm(1))
	if err != nil {
		t.Fatalf("append 4: %v", err)
	}
	if res.MeetingID == nil {
		t.Fatalf("expected meeting promotion at 5s")
	}
	if fin.createUploading != 1 {
		t.Fatalf("createUploading = %d, want 1", fin.createUploading)
	}
	if m := fin.meeting(*res.MeetingID); m == nil || m.status != "uploading" {
		t.Fatalf("meeting should be in uploading state, got %+v", m)
	}
	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	if sess.MeetingID == nil {
		t.Fatalf("session should hold the meeting id")
	}
}

// Heartbeat loss on a >5s recording must mark the meeting interrupted and then
// the server-side safety net assembles the audio and pushes it into the
// transcription pipeline (data is never lost even if the client never returns).
func TestRecoverInterrupted_FinalizesLostRecording(t *testing.T) {
	svc, repo, fin, dir := newTestService(t)
	const userID = 11
	init := mustInit(t, svc, userID)

	for i := 0; i < 6; i++ { // 6s recording
		if _, err := appendChunk(t, svc, userID, init.UploadID, i, pcm(1)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	if sess.MeetingID == nil {
		t.Fatalf("meeting should be promoted")
	}
	meetingID := *sess.MeetingID

	// Simulate a lost heartbeat well beyond the inactivity window.
	repo.setLastSeen(init.UploadID, time.Now().Add(-2*time.Hour))

	summary, err := svc.RecoverInterrupted(context.Background())
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if summary.Interrupted != 1 {
		t.Fatalf("Interrupted = %d, want 1", summary.Interrupted)
	}
	if summary.Finalized != 1 {
		t.Fatalf("Finalized = %d, want 1", summary.Finalized)
	}
	if fin.interrupted != 1 {
		t.Fatalf("MarkMeetingInterrupted calls = %d, want 1", fin.interrupted)
	}
	if fin.finalizeUploaded != 1 {
		t.Fatalf("FinalizeUploadedMeeting calls = %d, want 1", fin.finalizeUploaded)
	}
	m := fin.meeting(meetingID)
	if m == nil || m.status != "uploaded" || m.audioURL == "" {
		t.Fatalf("meeting should be finalized with audio, got %+v", m)
	}

	// Audio assembled on disk with a valid 44-byte WAV header + 6s of PCM.
	audioPath := filepath.Join(dir, "audio", init.UploadID+".wav")
	raw, err := os.ReadFile(audioPath)
	if err != nil {
		t.Fatalf("read assembled audio: %v", err)
	}
	wantBytes := 44 + 6*bytesPerSecond
	if len(raw) != wantBytes {
		t.Fatalf("assembled size = %d, want %d", len(raw), wantBytes)
	}
	if string(raw[0:4]) != "RIFF" || string(raw[8:12]) != "WAVE" {
		t.Fatalf("assembled file is not a WAV")
	}
	if got := binary.LittleEndian.Uint32(raw[24:28]); got != audioSampleRate {
		t.Fatalf("sample rate = %d, want %d", got, audioSampleRate)
	}
	sess, _ = repo.GetSessionByUploadID(context.Background(), init.UploadID)
	if sess.Status != domain.SessionStatusCompleted {
		t.Fatalf("session status = %s, want completed", sess.Status)
	}
}

// A long recording with a healthy heartbeat must never be marked interrupted nor
// have its segments reclaimed (the 24h-with-heartbeat criterion).
func TestMaintenance_HealthyRecordingIsNeverTouched(t *testing.T) {
	svc, repo, fin, _ := newTestService(t)
	const userID = 13
	init := mustInit(t, svc, userID)

	for i := 0; i < 6; i++ {
		if _, err := appendChunk(t, svc, userID, init.UploadID, i, pcm(1)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	// Heartbeat stays fresh.
	if _, err := svc.Heartbeat(context.Background(), userID, init.UploadID); err != nil {
		t.Fatalf("heartbeat: %v", err)
	}

	recovered, err := svc.RecoverInterrupted(context.Background())
	if err != nil {
		t.Fatalf("recover: %v", err)
	}
	if recovered.Interrupted != 0 || recovered.Finalized != 0 {
		t.Fatalf("healthy recording must not be recovered: %+v", recovered)
	}

	cleaned, err := svc.Cleanup(context.Background())
	if err != nil {
		t.Fatalf("cleanup: %v", err)
	}
	if cleaned.Reclaimed != 0 {
		t.Fatalf("healthy recording must not be reclaimed: %+v", cleaned)
	}

	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	if sess.Status != domain.SessionStatusRecording {
		t.Fatalf("session status = %s, want recording", sess.Status)
	}
	if repo.segmentCount(sess.ID) != 6 {
		t.Fatalf("segments must be preserved, got %d", repo.segmentCount(sess.ID))
	}
	if m := fin.meeting(*sess.MeetingID); m == nil || m.status != "uploading" {
		t.Fatalf("meeting should remain uploading, got %+v", m)
	}
}

// Cleanup must skip a recording/uploading session even if it is somehow passed
// to reclaimSession directly (defense in depth).
func TestReclaimSession_SkipsActiveSession(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	const userID = 15
	init := mustInit(t, svc, userID)
	if _, err := appendChunk(t, svc, userID, init.UploadID, 0, pcm(1)); err != nil {
		t.Fatalf("append: %v", err)
	}

	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	if err := svc.reclaimSession(context.Background(), sess); err != nil {
		t.Fatalf("reclaimSession: %v", err)
	}
	if got, _ := repo.GetSessionByID(context.Background(), sess.ID); got == nil {
		t.Fatalf("active session must not be deleted by reclaim")
	}
	if repo.segmentCount(sess.ID) != 1 {
		t.Fatalf("active session segments must be preserved")
	}
}

// After a crash the client resyncs via GET and resumes; re-sending stored chunks
// is idempotent, gaps and checksum conflicts are rejected.
func TestAppend_ResumeIsIdempotentAndOrdered(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	const userID = 17
	init := mustInit(t, svc, userID)

	chunk0 := pcm(1)
	chunk1 := pcm(1)
	chunk2 := pcm(1)
	for i, data := range [][]byte{chunk0, chunk1, chunk2} {
		if _, err := appendChunk(t, svc, userID, init.UploadID, i, data); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}

	state, err := svc.Get(context.Background(), userID, init.UploadID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if state.NextIndex != 3 {
		t.Fatalf("NextIndex = %d, want 3", state.NextIndex)
	}

	// Re-send already-stored chunk 1 with the same checksum -> idempotent.
	res, err := appendChunk(t, svc, userID, init.UploadID, 1, chunk1)
	if err != nil {
		t.Fatalf("duplicate append: %v", err)
	}
	if !res.Duplicate || res.NextIndex != 3 {
		t.Fatalf("duplicate append should be a no-op, got %+v", res)
	}

	// Same index, different content -> checksum conflict.
	_, err = svc.AppendChunk(context.Background(), AppendParams{
		UserID:   userID,
		UploadID: init.UploadID,
		Index:    1,
		Checksum: checksumOf([]byte("different")),
		Body:     bytes.NewReader([]byte("different")),
	})
	if ue := asUploadError(t, err); ue.Status != 409 {
		t.Fatalf("checksum conflict status = %d, want 409", ue.Status)
	}

	// Gap -> rejected.
	_, err = appendChunk(t, svc, userID, init.UploadID, 5, pcm(1))
	if ue := asUploadError(t, err); ue.Status != 409 {
		t.Fatalf("gap status = %d, want 409", ue.Status)
	}

	// Next in order -> accepted.
	res, err = appendChunk(t, svc, userID, init.UploadID, 3, pcm(1))
	if err != nil {
		t.Fatalf("append 3: %v", err)
	}
	if res.NextIndex != 4 {
		t.Fatalf("NextIndex = %d, want 4", res.NextIndex)
	}
}

// Complete must refuse to assemble audio with a hole in the chunk sequence.
func TestComplete_RejectsMissingChunk(t *testing.T) {
	svc, repo, _, _ := newTestService(t)
	const userID = 19
	init := mustInit(t, svc, userID)
	for i := 0; i < 6; i++ {
		if _, err := appendChunk(t, svc, userID, init.UploadID, i, pcm(1)); err != nil {
			t.Fatalf("append %d: %v", i, err)
		}
	}
	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	repo.dropSegment(sess.ID, 2) // simulate a lost segment row

	_, err := svc.Complete(context.Background(), userID, init.UploadID)
	miss, ok := err.(*MissingChunksError)
	if !ok {
		t.Fatalf("expected *MissingChunksError, got %T: %v", err, err)
	}
	if len(miss.Missing) != 1 || miss.Missing[0] != 2 {
		t.Fatalf("missing = %v, want [2]", miss.Missing)
	}
}

// A wrong owner must never see another user's session.
func TestLoadOwnedSession_RejectsOtherUser(t *testing.T) {
	svc, _, _, _ := newTestService(t)
	init := mustInit(t, svc, 21)
	_, err := svc.Get(context.Background(), 22, init.UploadID)
	if ue := asUploadError(t, err); ue.Status != 404 {
		t.Fatalf("status = %d, want 404", ue.Status)
	}
}

// Completing a fully-uploaded long recording assembles audio and creates the
// meeting through the pipeline when the session was never promoted (edge: client
// completes a 5s+ recording in a single flush before promotion).
func TestComplete_CreatesMeetingWhenNotPreviouslyPromoted(t *testing.T) {
	svc, repo, fin, dir := newTestService(t)
	const userID = 23
	init := mustInit(t, svc, userID)

	// Single 6s chunk: promotion happens during this append, so to exercise the
	// "create on complete" branch we clear the promoted meeting id first.
	if _, err := appendChunk(t, svc, userID, init.UploadID, 0, pcm(6)); err != nil {
		t.Fatalf("append: %v", err)
	}
	sess, _ := repo.GetSessionByUploadID(context.Background(), init.UploadID)
	sess.MeetingID = nil
	if err := repo.UpdateSession(context.Background(), sess); err != nil {
		t.Fatalf("update: %v", err)
	}
	fin.createUploading = 0

	res, err := svc.Complete(context.Background(), userID, init.UploadID)
	if err != nil {
		t.Fatalf("complete: %v", err)
	}
	if res.Status != string(domain.SessionStatusCompleted) || res.MeetingID == nil {
		t.Fatalf("expected completed meeting, got %+v", res)
	}
	if fin.createUploaded != 1 {
		t.Fatalf("CreateUploadedMeeting calls = %d, want 1", fin.createUploaded)
	}
	if _, err := os.Stat(filepath.Join(dir, "audio", init.UploadID+".wav")); err != nil {
		t.Fatalf("assembled audio missing: %v", err)
	}
}

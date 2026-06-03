package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// meetingUploadSession tracks the temp file and metadata for one in-progress
// chunked meeting upload.
type meetingUploadSession struct {
	id         string
	userID     uint64
	tmpPath    string
	file       *os.File
	ext        string
	filename   string
	title      string
	workflowID *uint64
	language   string
	size       int64
	nextIndex  int
	createdAt  time.Time
	mu         sync.Mutex
}

// meetingChunkUploader stores raw audio chunks to a temp file on disk so that
// arbitrarily long meeting recordings can be uploaded without exceeding the
// single-request size limits of nginx and the multipart parser.
type meetingChunkUploader struct {
	mu              sync.Mutex
	sessions        map[string]*meetingUploadSession
	maxChunkBytes   int64
	maxSessionBytes int64
	ttl             time.Duration
}

func newMeetingChunkUploader(maxChunkSizeMB, maxSessionSizeMB int64) *meetingChunkUploader {
	if maxChunkSizeMB <= 0 {
		maxChunkSizeMB = 8
	}
	if maxSessionSizeMB <= 0 {
		maxSessionSizeMB = 4096
	}
	return &meetingChunkUploader{
		sessions:        make(map[string]*meetingUploadSession),
		maxChunkBytes:   maxChunkSizeMB * 1024 * 1024,
		maxSessionBytes: maxSessionSizeMB * 1024 * 1024,
		ttl:             time.Hour,
	}
}

// sweepLocked removes stale sessions. Caller must hold u.mu.
func (u *meetingChunkUploader) sweepLocked(now time.Time) {
	for id, s := range u.sessions {
		if now.Sub(s.createdAt) > u.ttl {
			s.mu.Lock()
			if s.file != nil {
				_ = s.file.Close()
			}
			_ = os.Remove(s.tmpPath)
			s.mu.Unlock()
			delete(u.sessions, id)
		}
	}
}

func (u *meetingChunkUploader) init(userID uint64, filename, title string, workflowID *uint64, language string) (*meetingUploadSession, error) {
	ext := strings.ToLower(filepath.Ext(filename))
	if !isSupportedAudioExtension(ext) {
		return nil, &audioUploadError{statusCode: http.StatusBadRequest, message: "音频格式不支持，仅支持 wav/mp3"}
	}

	id := newUploadID()
	tmpPath := filepath.Join(os.TempDir(), fmt.Sprintf("meeting-upload-%s%s", id, ext))
	file, err := os.Create(tmpPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create upload temp file: %w", err)
	}

	session := &meetingUploadSession{
		id:         id,
		userID:     userID,
		tmpPath:    tmpPath,
		file:       file,
		ext:        ext,
		filename:   filename,
		title:      title,
		workflowID: workflowID,
		language:   language,
		createdAt:  time.Now(),
	}

	u.mu.Lock()
	u.sweepLocked(time.Now())
	u.sessions[id] = session
	u.mu.Unlock()
	return session, nil
}

func (u *meetingChunkUploader) get(id string, userID uint64) (*meetingUploadSession, bool) {
	u.mu.Lock()
	defer u.mu.Unlock()
	s, ok := u.sessions[id]
	if !ok || s.userID != userID {
		return nil, false
	}
	return s, true
}

func (u *meetingChunkUploader) discard(id string) {
	u.mu.Lock()
	s, ok := u.sessions[id]
	if ok {
		delete(u.sessions, id)
	}
	u.mu.Unlock()
	if !ok {
		return
	}
	s.mu.Lock()
	if s.file != nil {
		_ = s.file.Close()
		s.file = nil
	}
	_ = os.Remove(s.tmpPath)
	s.mu.Unlock()
}

// appendChunk writes one chunk to the session temp file. Chunks must arrive in
// order; out-of-order indices are rejected so the assembled file is correct.
func (u *meetingChunkUploader) appendChunk(s *meetingUploadSession, index int, body io.Reader) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return &audioUploadError{statusCode: http.StatusConflict, message: "upload session already finalized"}
	}
	if index != s.nextIndex {
		return &audioUploadError{statusCode: http.StatusConflict, message: fmt.Sprintf("unexpected chunk index %d, expected %d", index, s.nextIndex)}
	}

	limited := io.LimitReader(body, u.maxChunkBytes+1)
	written, err := io.Copy(s.file, limited)
	if err != nil {
		return fmt.Errorf("failed to write chunk: %w", err)
	}
	if written > u.maxChunkBytes {
		return &audioUploadError{statusCode: http.StatusRequestEntityTooLarge, message: fmt.Sprintf("单个分片不能超过 %d MB", u.maxChunkBytes/1024/1024)}
	}
	s.size += written
	if s.size > u.maxSessionBytes {
		return &audioUploadError{statusCode: http.StatusRequestEntityTooLarge, message: fmt.Sprintf("录音总大小不能超过 %d MB", u.maxSessionBytes/1024/1024)}
	}
	s.nextIndex++
	return nil
}

// complete closes the temp file and removes the session from tracking, returning
// the on-disk path of the assembled audio.
func (u *meetingChunkUploader) complete(s *meetingUploadSession) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.file == nil {
		return "", &audioUploadError{statusCode: http.StatusConflict, message: "upload session already finalized"}
	}
	if err := s.file.Close(); err != nil {
		return "", fmt.Errorf("failed to flush upload: %w", err)
	}
	s.file = nil
	if s.size == 0 {
		return "", &audioUploadError{statusCode: http.StatusBadRequest, message: "missing audio data"}
	}

	u.mu.Lock()
	delete(u.sessions, s.id)
	u.mu.Unlock()
	return s.tmpPath, nil
}

// RegisterChunkUpload registers the chunked meeting upload routes.
func (h *MeetingHandler) RegisterChunkUpload(group *gin.RouterGroup) {
	group.POST("/upload/init", h.ChunkUploadInit)
	group.POST("/upload/chunk", h.ChunkUploadAppend)
	group.POST("/upload/complete", h.ChunkUploadComplete)
	group.POST("/upload/abort", h.ChunkUploadAbort)
}

// ChunkUploadInit starts a chunked meeting upload session.
func (h *MeetingHandler) ChunkUploadInit(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}

	filename := strings.TrimSpace(c.PostForm("filename"))
	if filename == "" {
		filename = fmt.Sprintf("meeting-%d.wav", time.Now().UnixNano())
	}
	workflowID, err := parseMeetingWorkflowID(c.PostForm("workflow_id"))
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid workflow_id")
		return
	}
	language, _, _, err := parseASROptions(c)
	if err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}
	if err := h.validateMeetingWorkflow(c.Request.Context(), workflowID); err != nil {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, err.Error())
		return
	}

	userID := middleware.UserIDFromContext(c)
	session, err := h.chunkUpload.init(userID, filename, strings.TrimSpace(c.PostForm("title")), workflowID, language)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		response.Error(c, status, errcode.CodeBadRequest, messageText)
		return
	}

	response.Success(c, gin.H{
		"upload_id":      session.id,
		"max_chunk_size": h.chunkUpload.maxChunkBytes,
	})
}

// ChunkUploadAppend appends a single chunk (raw request body) to a session.
func (h *MeetingHandler) ChunkUploadAppend(c *gin.Context) {
	uploadID := strings.TrimSpace(c.Query("upload_id"))
	if uploadID == "" {
		uploadID = strings.TrimSpace(c.GetHeader("X-Upload-Id"))
	}
	indexRaw := strings.TrimSpace(c.Query("index"))
	if indexRaw == "" {
		indexRaw = strings.TrimSpace(c.GetHeader("X-Chunk-Index"))
	}
	index, err := strconv.Atoi(indexRaw)
	if err != nil || index < 0 {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid chunk index")
		return
	}

	userID := middleware.UserIDFromContext(c)
	session, ok := h.chunkUpload.get(uploadID, userID)
	if !ok {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "upload session not found")
		return
	}

	if err := h.chunkUpload.appendChunk(session, index, c.Request.Body); err != nil {
		status, messageText := resolveAudioUploadError(err)
		h.chunkUpload.discard(uploadID)
		response.Error(c, status, errcode.CodeBadRequest, messageText)
		return
	}

	response.Success(c, gin.H{"received": session.size, "next_index": session.nextIndex})
}

// ChunkUploadComplete assembles the uploaded chunks and creates the meeting.
func (h *MeetingHandler) ChunkUploadComplete(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	uploadID := strings.TrimSpace(c.Query("upload_id"))
	if uploadID == "" {
		uploadID = strings.TrimSpace(c.GetHeader("X-Upload-Id"))
	}
	userID := middleware.UserIDFromContext(c)
	session, ok := h.chunkUpload.get(uploadID, userID)
	if !ok {
		response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "upload session not found")
		return
	}

	tmpPath, err := h.chunkUpload.complete(session)
	if err != nil {
		status, messageText := resolveAudioUploadError(err)
		h.chunkUpload.discard(uploadID)
		response.Error(c, status, errcode.CodeBadRequest, messageText)
		return
	}

	audioFile, err := promoteAssembledAudio(c.Request.Context(), tmpPath, session.filename, session.ext, h.uploadDir, "audio")
	if err != nil {
		_ = os.Remove(tmpPath)
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}

	h.finalizeMeetingUpload(c, audioFile, session.title, session.workflowID, session.language)
}

// ChunkUploadAbort discards an in-progress upload session.
func (h *MeetingHandler) ChunkUploadAbort(c *gin.Context) {
	uploadID := strings.TrimSpace(c.Query("upload_id"))
	if uploadID == "" {
		uploadID = strings.TrimSpace(c.GetHeader("X-Upload-Id"))
	}
	h.chunkUpload.discard(uploadID)
	response.Success(c, gin.H{"aborted": true})
}

// promoteAssembledAudio moves an assembled temp file into the permanent upload
// directory and probes it, mirroring savePermanentUploadedAudio for the
// chunked flow.
func promoteAssembledAudio(ctx context.Context, tmpPath, originalFilename, ext, uploadRootDir, relativeDir string) (*storedAudioFile, error) {
	storedDir := filepath.Join(uploadRootDir, relativeDir)
	if err := os.MkdirAll(storedDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to prepare upload directory: %w", err)
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	absPath := filepath.Join(storedDir, filename)
	if err := moveOrCopyFile(tmpPath, absPath); err != nil {
		return nil, fmt.Errorf("failed to store audio file: %w", err)
	}

	info, statErr := os.Stat(absPath)
	var size int64
	if statErr == nil {
		size = info.Size()
	}

	audioFile := &storedAudioFile{
		OriginalFilename: originalFilename,
		AbsolutePath:     absPath,
		RelativePath:     path.Join(relativeDir, filename),
		Size:             size,
	}
	prepareStoredAudio(ctx, audioFile)
	return audioFile, nil
}

// moveOrCopyFile renames src to dst, falling back to a copy when the two paths
// live on different filesystems (os.TempDir vs uploads volume).
func moveOrCopyFile(src, dst string) error {
	if err := os.Rename(src, dst); err == nil {
		return nil
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	_ = os.Remove(src)
	return nil
}

func newUploadID() string {
	buffer := make([]byte, 16)
	if _, err := rand.Read(buffer); err == nil {
		return hex.EncodeToString(buffer)
	}
	return fmt.Sprintf("upload-%d", time.Now().UnixNano())
}

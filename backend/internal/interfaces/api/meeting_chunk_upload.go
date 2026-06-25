package api

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	appmeetingupload "github.com/lgt/asr/internal/application/meetingupload"
	"github.com/lgt/asr/internal/interfaces/middleware"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// RegisterChunkUpload registers the resumable, crash-safe meeting upload routes.
// Chunks are streamed to durable server storage during recording so an
// arbitrarily long meeting survives a client crash or network drop and is never
// lost in client memory.
func (h *MeetingHandler) RegisterChunkUpload(group *gin.RouterGroup) {
	group.POST("/upload/init", h.ChunkUploadInit)
	group.POST("/upload/chunk", h.ChunkUploadAppend)
	group.POST("/upload/heartbeat", h.ChunkUploadHeartbeat)
	group.POST("/upload/complete", h.ChunkUploadComplete)
	group.POST("/upload/abort", h.ChunkUploadAbort)
	group.GET("/upload/:id", h.ChunkUploadStatus)
}

// uploadIDFromRequest reads the upload id from the query string or header.
func uploadIDFromRequest(c *gin.Context) string {
	uploadID := strings.TrimSpace(c.Query("upload_id"))
	if uploadID == "" {
		uploadID = strings.TrimSpace(c.GetHeader("X-Upload-Id"))
	}
	return uploadID
}

// respondUploadError maps an upload service error onto an HTTP response.
func respondUploadError(c *gin.Context, err error) {
	var missing *appmeetingupload.MissingChunksError
	if errors.As(err, &missing) {
		response.Error(c, http.StatusConflict, errcode.CodeBadRequest, missing.Error())
		return
	}
	var uploadErr *appmeetingupload.Error
	if errors.As(err, &uploadErr) {
		code := errcode.CodeBadRequest
		if uploadErr.Status == http.StatusNotFound {
			code = errcode.CodeNotFound
		} else if uploadErr.Status >= http.StatusInternalServerError {
			code = errcode.CodeInternal
		}
		response.Error(c, uploadErr.Status, code, uploadErr.Message)
		return
	}
	response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
}

// ChunkUploadInit starts a resumable meeting upload session.
func (h *MeetingHandler) ChunkUploadInit(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}

	filename := strings.TrimSpace(c.PostForm("filename"))
	if filename == "" {
		filename = "meeting-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".wav"
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
	result, err := h.uploadService.Init(c.Request.Context(), appmeetingupload.InitParams{
		UserID:        userID,
		Filename:      filename,
		Title:         strings.TrimSpace(c.PostForm("title")),
		WorkflowID:    workflowID,
		Language:      language,
		PublicBaseURL: h.resolveUploadBaseURL(c),
	})
	if err != nil {
		respondUploadError(c, err)
		return
	}

	response.Success(c, gin.H{
		"upload_id":      result.UploadID,
		"next_index":     result.NextIndex,
		"max_chunk_size": result.MaxChunkSize,
	})
}

// ChunkUploadAppend stores one raw-PCM chunk (the request body) durably.
func (h *MeetingHandler) ChunkUploadAppend(c *gin.Context) {
	uploadID := uploadIDFromRequest(c)
	indexRaw := strings.TrimSpace(c.Query("index"))
	if indexRaw == "" {
		indexRaw = strings.TrimSpace(c.GetHeader("X-Chunk-Index"))
	}
	index, err := strconv.Atoi(indexRaw)
	if err != nil || index < 0 {
		response.Error(c, http.StatusBadRequest, errcode.CodeBadRequest, "invalid chunk index")
		return
	}
	checksum := strings.TrimSpace(c.Query("checksum"))
	if checksum == "" {
		checksum = strings.TrimSpace(c.GetHeader("X-Chunk-Checksum"))
	}

	userID := middleware.UserIDFromContext(c)
	result, err := h.uploadService.AppendChunk(c.Request.Context(), appmeetingupload.AppendParams{
		UserID:   userID,
		UploadID: uploadID,
		Index:    index,
		Checksum: checksum,
		Body:     c.Request.Body,
	})
	if err != nil {
		// Drain any remaining body so the connection can be reused cleanly.
		_, _ = io.Copy(io.Discard, c.Request.Body)
		respondUploadError(c, err)
		return
	}

	response.Success(c, gin.H{
		"received":   result.Received,
		"next_index": result.NextIndex,
		"duration":   result.DurationSec,
		"status":     result.Status,
		"meeting_id": result.MeetingID,
		"duplicate":  result.Duplicate,
	})
}

// ChunkUploadHeartbeat keeps a recording session alive while the client is idle
// (e.g. a long pause) so the server does not treat it as a lost connection.
func (h *MeetingHandler) ChunkUploadHeartbeat(c *gin.Context) {
	uploadID := uploadIDFromRequest(c)
	userID := middleware.UserIDFromContext(c)
	state, err := h.uploadService.Heartbeat(c.Request.Context(), userID, uploadID)
	if err != nil {
		respondUploadError(c, err)
		return
	}
	response.Success(c, uploadStateBody(state))
}

// ChunkUploadComplete assembles the uploaded chunks and finalizes the meeting.
func (h *MeetingHandler) ChunkUploadComplete(c *gin.Context) {
	if !h.feature.meeting() {
		h.feature.denyFeature(c, "当前版本未开放会议纪要")
		return
	}
	uploadID := uploadIDFromRequest(c)
	userID := middleware.UserIDFromContext(c)
	result, err := h.uploadService.Complete(c.Request.Context(), userID, uploadID)
	if err != nil {
		respondUploadError(c, err)
		return
	}
	response.Success(c, gin.H{
		"meeting_id": result.MeetingID,
		"status":     result.Status,
		"duration":   result.DurationSec,
	})
}

// ChunkUploadAbort discards an in-progress upload session.
func (h *MeetingHandler) ChunkUploadAbort(c *gin.Context) {
	uploadID := uploadIDFromRequest(c)
	userID := middleware.UserIDFromContext(c)
	if err := h.uploadService.Abort(c.Request.Context(), userID, uploadID); err != nil {
		respondUploadError(c, err)
		return
	}
	response.Success(c, gin.H{"aborted": true})
}

// ChunkUploadStatus returns the durable state of a session so a client can
// resync (after a restart) and resume from the exact next chunk index.
func (h *MeetingHandler) ChunkUploadStatus(c *gin.Context) {
	uploadID := strings.TrimSpace(c.Param("id"))
	userID := middleware.UserIDFromContext(c)
	state, err := h.uploadService.Get(c.Request.Context(), userID, uploadID)
	if err != nil {
		respondUploadError(c, err)
		return
	}
	response.Success(c, uploadStateBody(state))
}

func uploadStateBody(state *appmeetingupload.StateResult) gin.H {
	return gin.H{
		"upload_id":   state.UploadID,
		"status":      state.Status,
		"next_index":  state.NextIndex,
		"duration":    state.DurationSec,
		"total_bytes": state.TotalBytes,
		"meeting_id":  state.MeetingID,
	}
}

// resolveUploadBaseURL captures the absolute public base URL at init time so the
// background recovery and cleanup tasks (which have no request context) can
// build absolute audio URLs for assembled recordings.
func (h *MeetingHandler) resolveUploadBaseURL(c *gin.Context) string {
	if h.publicBaseURL != "" {
		return h.publicBaseURL
	}
	return publicRequestBaseURL(c)
}

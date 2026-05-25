package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/internal/infrastructure/audiofile"
)

type storedAudioFile struct {
	OriginalFilename string
	AbsolutePath     string
	RelativePath     string
	Size             int64
	Duration         float64
}

const defaultMaxAudioSizeMB int64 = 200

type audioUploadError struct {
	statusCode int
	message    string
}

func (e *audioUploadError) Error() string {
	return e.message
}

func saveTemporaryUploadedAudio(c *gin.Context, fieldName, prefix string, maxAudioSizeMB int64) (*storedAudioFile, error) {
	fileHeader, ext, err := parseUploadedAudio(c, fieldName, maxAudioSizeMB)
	if err != nil {
		return nil, err
	}

	absPath := filepath.Join(os.TempDir(), fmt.Sprintf("%s-%d%s", prefix, time.Now().UnixNano(), ext))
	if err := writeUploadedFile(fileHeader, absPath); err != nil {
		return nil, fmt.Errorf("failed to save audio file: %w", err)
	}

	audioFile := &storedAudioFile{
		OriginalFilename: fileHeader.Filename,
		AbsolutePath:     absPath,
		Size:             fileHeader.Size,
	}
	prepareStoredAudio(c.Request.Context(), audioFile)
	return audioFile, nil
}

func savePermanentUploadedAudio(c *gin.Context, fieldName, uploadRootDir, relativeDir string, maxAudioSizeMB int64) (*storedAudioFile, error) {
	fileHeader, ext, err := parseUploadedAudio(c, fieldName, maxAudioSizeMB)
	if err != nil {
		return nil, err
	}

	storedDir := filepath.Join(uploadRootDir, relativeDir)
	if err := os.MkdirAll(storedDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to prepare upload directory: %w", err)
	}

	filename := fmt.Sprintf("%d%s", time.Now().UnixNano(), ext)
	absPath := filepath.Join(storedDir, filename)
	if err := writeUploadedFile(fileHeader, absPath); err != nil {
		return nil, fmt.Errorf("failed to save audio file: %w", err)
	}

	audioFile := &storedAudioFile{
		OriginalFilename: fileHeader.Filename,
		AbsolutePath:     absPath,
		RelativePath:     path.Join(relativeDir, filename),
		Size:             fileHeader.Size,
	}
	prepareStoredAudio(c.Request.Context(), audioFile)

	return audioFile, nil
}

func parseUploadedAudio(c *gin.Context, fieldName string, maxAudioSizeMB int64) (*multipart.FileHeader, string, error) {
	fileHeader, err := c.FormFile(fieldName)
	if err != nil {
		return nil, "", &audioUploadError{statusCode: http.StatusBadRequest, message: "missing audio file"}
	}

	maxAudioSizeMB = effectiveMaxAudioSizeMB(maxAudioSizeMB)
	maxBytes := maxAudioSizeBytes(maxAudioSizeMB)
	if fileHeader.Size > maxBytes {
		return nil, "", &audioUploadError{statusCode: http.StatusRequestEntityTooLarge, message: audioTooLargeMessage(maxAudioSizeMB)}
	}

	ext := strings.ToLower(filepath.Ext(fileHeader.Filename))
	if !isSupportedAudioExtension(ext) {
		return nil, "", &audioUploadError{statusCode: http.StatusBadRequest, message: "音频格式不支持，仅支持 wav/mp3"}
	}

	return fileHeader, ext, nil
}

func effectiveMaxAudioSizeMB(maxAudioSizeMB int64) int64 {
	if maxAudioSizeMB <= 0 {
		return defaultMaxAudioSizeMB
	}
	return maxAudioSizeMB
}

func maxAudioSizeBytes(maxAudioSizeMB int64) int64 {
	return effectiveMaxAudioSizeMB(maxAudioSizeMB) * 1024 * 1024
}

func audioTooLargeMessage(maxAudioSizeMB int64) string {
	return fmt.Sprintf("音频文件不能超过 %d MB，请压缩或切分后再上传", effectiveMaxAudioSizeMB(maxAudioSizeMB))
}

func writeUploadedFile(fileHeader *multipart.FileHeader, absPath string) error {
	src, err := fileHeader.Open()
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.Create(absPath)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return nil
}

func resolveAudioUploadError(err error) (int, string) {
	var requestErr *audioUploadError
	if errors.As(err, &requestErr) {
		return requestErr.statusCode, requestErr.message
	}
	return http.StatusInternalServerError, "failed to prepare audio file"
}

func bestEffortAudioDuration(ctx context.Context, localPath string) float64 {
	duration, err := audiofile.ProbeDuration(ctx, localPath)
	if err != nil {
		return 0
	}
	return duration
}

func prepareStoredAudio(ctx context.Context, audioFile *storedAudioFile) {
	if audioFile == nil || audioFile.AbsolutePath == "" {
		return
	}

	prepared, err := audiofile.PrepareForProcessing(ctx, audioFile.AbsolutePath)
	if err != nil {
		audioFile.Duration = bestEffortAudioDuration(ctx, audioFile.AbsolutePath)
		return
	}

	audioFile.AbsolutePath = prepared.Path
	audioFile.Duration = prepared.Duration
	if audioFile.RelativePath != "" {
		audioFile.RelativePath = strings.TrimSuffix(audioFile.RelativePath, path.Ext(audioFile.RelativePath)) + filepath.Ext(prepared.Path)
	}
}

func buildUploadedFileURL(c *gin.Context, publicBaseURL, relativePath string) (string, error) {
	baseURL := strings.TrimSpace(publicBaseURL)
	if baseURL == "" {
		baseURL = publicRequestBaseURL(c)
	}
	if baseURL == "" {
		return "", fmt.Errorf("unable to determine public upload base url")
	}

	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", fmt.Errorf("invalid upload public base url")
	}
	parsed.Path = path.Join(parsed.Path, "/uploads", relativePath)
	return parsed.String(), nil
}

func publicRequestBaseURL(c *gin.Context) string {
	if origin := strings.TrimSpace(c.GetHeader("Origin")); origin != "" {
		if parsed, err := url.Parse(origin); err == nil && parsed.Scheme != "" && parsed.Host != "" {
			return fmt.Sprintf("%s://%s", parsed.Scheme, parsed.Host)
		}
	}

	scheme := c.GetHeader("X-Forwarded-Proto")
	if scheme == "" {
		if c.Request.TLS != nil {
			scheme = "https"
		}
	}
	if scheme == "" {
		scheme = "http"
	}

	host := strings.TrimSpace(c.GetHeader("X-Forwarded-Host"))
	if host == "" {
		host = c.Request.Host
	}
	if host == "" {
		return ""
	}

	return fmt.Sprintf("%s://%s", scheme, host)
}

func isSupportedAudioExtension(ext string) bool {
	switch ext {
	case ".wav", ".mp3":
		return true
	default:
		return false
	}
}

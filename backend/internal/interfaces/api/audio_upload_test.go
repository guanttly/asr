package api

import (
	"bytes"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestSaveTemporaryUploadedAudioPreparesFile(t *testing.T) {
	t.Helper()
	skipWithoutFFmpegAndFFprobe(t)
	gin.SetMode(gin.TestMode)

	sourcePath := filepath.Join(t.TempDir(), "snippet.mp3")
	writeUploadTestTone(t, sourcePath)

	ctx := newAudioUploadTestContext(t, "file", "snippet.mp3", sourcePath)
	audioFile, err := saveTemporaryUploadedAudio(ctx, "file", "asr-realtime", 100)
	if err != nil {
		t.Fatalf("saveTemporaryUploadedAudio returned error: %v", err)
	}
	defer os.Remove(audioFile.AbsolutePath)

	if filepath.Ext(audioFile.AbsolutePath) != ".wav" {
		t.Fatalf("expected normalized wav temp file, got %s", audioFile.AbsolutePath)
	}
	if audioFile.Duration <= 0 {
		t.Fatalf("expected positive duration, got %v", audioFile.Duration)
	}
	if _, err := os.Stat(audioFile.AbsolutePath); err != nil {
		t.Fatalf("stat prepared temp audio: %v", err)
	}
	if audioFile.OriginalFilename != "snippet.mp3" {
		t.Fatalf("expected original filename preserved, got %s", audioFile.OriginalFilename)
	}
}

func TestSavePermanentUploadedAudioPreparesFile(t *testing.T) {
	t.Helper()
	skipWithoutFFmpegAndFFprobe(t)
	gin.SetMode(gin.TestMode)

	sourcePath := filepath.Join(t.TempDir(), "meeting.mp3")
	writeUploadTestTone(t, sourcePath)

	ctx := newAudioUploadTestContext(t, "file", "meeting.mp3", sourcePath)
	uploadRootDir := filepath.Join(t.TempDir(), "uploads")
	audioFile, err := savePermanentUploadedAudio(ctx, "file", uploadRootDir, "audio", 100)
	if err != nil {
		t.Fatalf("savePermanentUploadedAudio returned error: %v", err)
	}
	defer os.Remove(audioFile.AbsolutePath)

	if filepath.Ext(audioFile.AbsolutePath) != ".wav" {
		t.Fatalf("expected normalized wav file, got %s", audioFile.AbsolutePath)
	}
	if filepath.Ext(audioFile.RelativePath) != ".wav" {
		t.Fatalf("expected wav relative path, got %s", audioFile.RelativePath)
	}
	if audioFile.Duration <= 0 {
		t.Fatalf("expected positive duration, got %v", audioFile.Duration)
	}
	if _, err := os.Stat(audioFile.AbsolutePath); err != nil {
		t.Fatalf("stat prepared stored audio: %v", err)
	}
	if audioFile.OriginalFilename != "meeting.mp3" {
		t.Fatalf("expected original filename preserved, got %s", audioFile.OriginalFilename)
	}
}

func TestParseUploadedAudioAllowsFileAtLimit(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	sourcePath := filepath.Join(t.TempDir(), "limit.wav")
	if err := os.WriteFile(sourcePath, make([]byte, 1024*1024), 0o644); err != nil {
		t.Fatalf("write source audio: %v", err)
	}

	ctx := newAudioUploadTestContext(t, "file", "limit.wav", sourcePath)
	fileHeader, _, err := parseUploadedAudio(ctx, "file", 1)
	if err != nil {
		t.Fatalf("parseUploadedAudio returned error: %v", err)
	}
	if fileHeader.Size != 1024*1024 {
		t.Fatalf("expected size at limit, got %d", fileHeader.Size)
	}
}

func TestParseUploadedAudioRejectsFileAboveLimit(t *testing.T) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	sourcePath := filepath.Join(t.TempDir(), "too-large.wav")
	if err := os.WriteFile(sourcePath, make([]byte, 1024*1024+1), 0o644); err != nil {
		t.Fatalf("write source audio: %v", err)
	}

	ctx := newAudioUploadTestContext(t, "file", "too-large.wav", sourcePath)
	_, _, err := parseUploadedAudio(ctx, "file", 1)
	if err == nil {
		t.Fatal("expected size limit error")
	}
	var uploadErr *audioUploadError
	if !errors.As(err, &uploadErr) {
		t.Fatalf("expected audioUploadError, got %T", err)
	}
	if uploadErr.statusCode != http.StatusRequestEntityTooLarge {
		t.Fatalf("expected 413, got %d", uploadErr.statusCode)
	}
	if uploadErr.message != "音频文件不能超过 1 MB，请压缩或切分后再上传" {
		t.Fatalf("unexpected message: %s", uploadErr.message)
	}
}

func skipWithoutFFmpegAndFFprobe(t *testing.T) {
	t.Helper()
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not available")
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		t.Skip("ffprobe not available")
	}
}

func writeUploadTestTone(t *testing.T, outputPath string) {
	t.Helper()
	cmd := exec.Command("ffmpeg",
		"-hide_banner",
		"-loglevel", "error",
		"-y",
		"-f", "lavfi",
		"-i", "sine=frequency=1000:duration=1",
		"-ar", "8000",
		"-ac", "1",
		outputPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generate upload test audio: %v: %s", err, string(output))
	}
}

func newAudioUploadTestContext(t *testing.T, fieldName, uploadName, sourcePath string) *gin.Context {
	t.Helper()

	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile(fieldName, uploadName)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}

	source, err := os.Open(sourcePath)
	if err != nil {
		t.Fatalf("open source audio: %v", err)
	}
	defer source.Close()

	if _, err := io.Copy(part, source); err != nil {
		t.Fatalf("copy source audio: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer: %v", err)
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "/", body)
	ctx.Request.Header.Set("Content-Type", writer.FormDataContentType())
	return ctx
}

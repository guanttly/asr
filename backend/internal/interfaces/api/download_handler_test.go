package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestDownloadHandlerListReturnsSortedFiles(t *testing.T) {
	gin.SetMode(gin.TestMode)

	downloadDir := t.TempDir()
	older := filepath.Join(downloadDir, "older.exe")
	newer := filepath.Join(downloadDir, "语音速录助手_0.2.5_x64-setup.exe")
	archive := filepath.Join(downloadDir, "asr-terminal-0.2.5.run")
	readme := filepath.Join(downloadDir, "README.txt")
	hidden := filepath.Join(downloadDir, ".hidden")
	folder := filepath.Join(downloadDir, "folder")

	if err := os.WriteFile(older, []byte("old"), 0o644); err != nil {
		t.Fatalf("write older file: %v", err)
	}
	if err := os.WriteFile(newer, []byte("newer-package"), 0o644); err != nil {
		t.Fatalf("write newer file: %v", err)
	}
	if err := os.WriteFile(archive, []byte("bundle"), 0o644); err != nil {
		t.Fatalf("write archive file: %v", err)
	}
	if err := os.WriteFile(readme, []byte("skip readme"), 0o644); err != nil {
		t.Fatalf("write readme file: %v", err)
	}
	if err := os.WriteFile(hidden, []byte("skip"), 0o644); err != nil {
		t.Fatalf("write hidden file: %v", err)
	}
	if err := os.MkdirAll(folder, 0o755); err != nil {
		t.Fatalf("create subdir: %v", err)
	}

	olderTime := time.Now().Add(-2 * time.Hour)
	newerTime := time.Now().Add(-30 * time.Minute)
	archiveTime := time.Now().Add(-90 * time.Minute)
	if err := os.Chtimes(older, olderTime, olderTime); err != nil {
		t.Fatalf("chtimes older: %v", err)
	}
	if err := os.Chtimes(newer, newerTime, newerTime); err != nil {
		t.Fatalf("chtimes newer: %v", err)
	}
	if err := os.Chtimes(archive, archiveTime, archiveTime); err != nil {
		t.Fatalf("chtimes archive: %v", err)
	}

	router := gin.New()
	NewDownloadHandler(downloadDir, "/downloads/files").Register(router.Group("/api/admin"))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/downloads", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}

	var envelope struct {
		Code int `json:"code"`
		Data struct {
			Items []struct {
				Name        string `json:"name"`
				SizeBytes   int64  `json:"size_bytes"`
				DownloadURL string `json:"download_url"`
			} `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(resp.Body.Bytes(), &envelope); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}

	if envelope.Code != 0 {
		t.Fatalf("expected success code, got %d", envelope.Code)
	}
	if len(envelope.Data.Items) != 3 {
		t.Fatalf("expected 3 visible package files, got %d", len(envelope.Data.Items))
	}
	if envelope.Data.Items[0].Name != filepath.Base(newer) {
		t.Fatalf("expected newest file first, got %s", envelope.Data.Items[0].Name)
	}
	if envelope.Data.Items[0].DownloadURL != "/downloads/files/%E8%AF%AD%E9%9F%B3%E9%80%9F%E5%BD%95%E5%8A%A9%E6%89%8B_0.2.5_x64-setup.exe" {
		t.Fatalf("unexpected encoded download url: %s", envelope.Data.Items[0].DownloadURL)
	}
	if envelope.Data.Items[1].Name != filepath.Base(archive) {
		t.Fatalf("expected archive file second, got %s", envelope.Data.Items[1].Name)
	}
	if envelope.Data.Items[2].Name != filepath.Base(older) {
		t.Fatalf("expected older file third, got %s", envelope.Data.Items[2].Name)
	}
	for _, item := range envelope.Data.Items {
		if item.Name == filepath.Base(readme) {
			t.Fatal("readme should not be exposed as downloadable package")
		}
	}
}

func TestDownloadHandlerListReturnsEmptyWhenDirectoryMissing(t *testing.T) {
	gin.SetMode(gin.TestMode)

	router := gin.New()
	NewDownloadHandler(filepath.Join(t.TempDir(), "missing"), "/downloads/files").Register(router.Group("/api/admin"))

	req := httptest.NewRequest(http.MethodGet, "/api/admin/downloads", nil)
	resp := httptest.NewRecorder()
	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.Code)
	}
	if body := resp.Body.String(); body == "" {
		t.Fatal("expected response body")
	}
}

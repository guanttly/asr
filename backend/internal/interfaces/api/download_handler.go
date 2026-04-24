package api

import (
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/lgt/asr/pkg/response"
)

type downloadableArtifact struct {
	Name        string    `json:"name"`
	SizeBytes   int64     `json:"size_bytes"`
	ModifiedAt  time.Time `json:"modified_at"`
	DownloadURL string    `json:"download_url"`
}

// DownloadHandler exposes package download listings for mounted desktop builds.
type DownloadHandler struct {
	dir            string
	publicBasePath string
}

func NewDownloadHandler(dir, publicBasePath string) *DownloadHandler {
	resolvedDir := strings.TrimSpace(dir)
	if resolvedDir == "" {
		resolvedDir = "downloads"
	}

	resolvedBasePath := strings.TrimSpace(publicBasePath)
	if resolvedBasePath == "" {
		resolvedBasePath = "/downloads/files"
	}

	return &DownloadHandler{
		dir:            resolvedDir,
		publicBasePath: strings.TrimRight(resolvedBasePath, "/"),
	}
}

func (h *DownloadHandler) Register(group *gin.RouterGroup) {
	group.GET("/downloads", h.List)
}

func (h *DownloadHandler) RegisterPublic(group *gin.RouterGroup) {
	group.GET("/downloads", h.List)
}

func (h *DownloadHandler) List(c *gin.Context) {
	entries, err := os.ReadDir(h.dir)
	if err != nil {
		if os.IsNotExist(err) {
			response.Success(c, gin.H{"items": []downloadableArtifact{}})
			return
		}
		response.Error(c, http.StatusInternalServerError, http.StatusInternalServerError, "failed to read download directory")
		return
	}

	items := make([]downloadableArtifact, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		if !isPublicDownloadArtifact(entry.Name()) {
			continue
		}

		info, infoErr := entry.Info()
		if infoErr != nil {
			continue
		}

		items = append(items, downloadableArtifact{
			Name:        entry.Name(),
			SizeBytes:   info.Size(),
			ModifiedAt:  info.ModTime(),
			DownloadURL: h.buildDownloadURL(entry.Name()),
		})
	}

	sort.Slice(items, func(i, j int) bool {
		if items[i].ModifiedAt.Equal(items[j].ModifiedAt) {
			return strings.ToLower(items[i].Name) < strings.ToLower(items[j].Name)
		}
		return items[i].ModifiedAt.After(items[j].ModifiedAt)
	})

	response.Success(c, gin.H{"items": items})
}

func isPublicDownloadArtifact(name string) bool {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return false
	}

	lowerName := strings.ToLower(trimmed)
	if lowerName == "readme" || strings.HasPrefix(lowerName, "readme.") {
		return false
	}

	switch strings.ToLower(filepath.Ext(lowerName)) {
	case ".exe", ".msi", ".zip", ".7z", ".gz", ".tgz", ".tar", ".run":
		return true
	default:
		return false
	}
}

func (h *DownloadHandler) buildDownloadURL(name string) string {
	basePath := h.publicBasePath
	if basePath == "" {
		basePath = "/downloads/files"
	}
	return basePath + "/" + url.PathEscape(name)
}

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
	Platform    string    `json:"platform"`
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
			Platform:    classifyArtifactPlatform(entry.Name()),
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

// classifyArtifactPlatform 根据安装包文件名推断目标 Windows 版本：
//   - 含 "_win7_" / "-win7-" / "electron" 视为 Windows 7 兼容包（Electron 22 打出来的，
//     文件名由 desktop-electron/electron-builder.yml 强制带 _win7_ 标记）。
//   - 含 "_win10_" / "-win10-" / "win10+" / "win11" / "tauri" 视为 Win10/11 推荐版。
//   - 其他 .exe / .msi 默认归为 Win10/11 推荐版——本产品的 Windows 安装包只有两条产线：
//     Win7 走 Electron（必带 _win7_）、Win10/11 走 Tauri NSIS（默认输出
//     "<productName>_<version>_x64-setup.exe"，没有平台前缀）。
//   - 其他（.zip/.run/.tar.gz 等）归为 "other"，前端展示在通用区。
func classifyArtifactPlatform(name string) string {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "_win7_"),
		strings.Contains(lower, "-win7-"),
		strings.Contains(lower, ".win7."),
		strings.Contains(lower, "win-7"),
		strings.Contains(lower, "electron"):
		return "win7"
	case strings.Contains(lower, "_win10_"),
		strings.Contains(lower, "-win10-"),
		strings.Contains(lower, "win10+"),
		strings.Contains(lower, "win11"),
		strings.Contains(lower, "tauri"):
		return "win10+"
	}
	switch strings.ToLower(filepath.Ext(lower)) {
	case ".exe", ".msi":
		return "win10+"
	default:
		return "other"
	}
}

func (h *DownloadHandler) buildDownloadURL(name string) string {
	basePath := h.publicBasePath
	if basePath == "" {
		basePath = "/downloads/files"
	}
	return basePath + "/" + url.PathEscape(name)
}

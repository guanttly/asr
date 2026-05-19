package api

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"github.com/gin-gonic/gin"
	appcatalog "github.com/lgt/asr/internal/application/catalog"
	"github.com/lgt/asr/pkg/errcode"
	"github.com/lgt/asr/pkg/response"
)

// TermCatalogHandler exposes read-only browsing of the terminology catalog:
// a directory tree of markdown files plus per-department Excel downloads.
type TermCatalogHandler struct {
	service *appcatalog.Service
}

// NewTermCatalogHandler builds a handler.
func NewTermCatalogHandler(service *appcatalog.Service) *TermCatalogHandler {
	return &TermCatalogHandler{service: service}
}

// Register wires routes under the admin group.
func (h *TermCatalogHandler) Register(group *gin.RouterGroup) {
	catalog := group.Group("/term-catalog")
	catalog.GET("/tree", h.GetTree)
	catalog.GET("/file", h.GetFile)
	catalog.GET("/export.xlsx", h.ExportXLSX)
}

// GetTree returns the directory tree of the active catalog source.
func (h *TermCatalogHandler) GetTree(c *gin.Context) {
	tree, err := h.service.Tree()
	if err != nil {
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, gin.H{
		"items":  tree,
		"source": h.service.ActivePath(),
	})
}

// GetFile streams parsed details for a single markdown file. The path comes
// in via ?path= and is validated against directory traversal.
func (h *TermCatalogHandler) GetFile(c *gin.Context) {
	pathParam := c.Query("path")
	detail, err := h.service.GetFile(pathParam)
	if err != nil {
		if errors.Is(err, appcatalog.ErrFileNotFound) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "catalog file not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	response.Success(c, detail)
}

// ExportXLSX returns the built-in workbook for a department directory. The
// path comes in via ?path= and points at a catalog directory such as radiology.
func (h *TermCatalogHandler) ExportXLSX(c *gin.Context) {
	pathParam := c.Query("path")
	filename, content, count, err := h.service.ExportXLSX(pathParam)
	if err != nil {
		if errors.Is(err, appcatalog.ErrFileNotFound) {
			response.Error(c, http.StatusNotFound, errcode.CodeNotFound, "catalog xlsx not found")
			return
		}
		response.Error(c, http.StatusInternalServerError, errcode.CodeInternal, err.Error())
		return
	}
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename*=UTF-8''%s", url.PathEscape(filename)))
	c.Header("X-Term-Count", fmt.Sprintf("%d", count))
	c.Data(http.StatusOK, "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", content)
}

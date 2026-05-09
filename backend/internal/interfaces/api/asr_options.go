package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	appwf "github.com/lgt/asr/internal/application/workflow"
	wfdomain "github.com/lgt/asr/internal/domain/workflow"
)

func parseASROptions(c *gin.Context) (string, *bool, []string, error) {
	language := strings.TrimSpace(c.PostForm("language"))
	if language == "" {
		language = "auto"
	}
	useITN, err := parseOptionalBoolForm(c, "use_itn")
	if err != nil {
		return "", nil, nil, err
	}
	return language, useITN, parseHotwords(c.PostForm("hotwords")), nil
}

func parseOptionalBoolForm(c *gin.Context, key string) (*bool, error) {
	raw := strings.TrimSpace(c.PostForm(key))
	if raw == "" {
		return nil, nil
	}
	parsed, err := strconv.ParseBool(raw)
	if err != nil {
		switch strings.ToLower(raw) {
		case "1", "yes", "y", "on":
			value := true
			return &value, nil
		case "0", "no", "n", "off":
			value := false
			return &value, nil
		default:
			return nil, fmt.Errorf("invalid %s", key)
		}
	}
	return &parsed, nil
}

func parseHotwords(raw string) []string {
	fields := strings.FieldsFunc(raw, func(value rune) bool {
		switch value {
		case ',', '，', ';', '；', '\n', '\r', '\t':
			return true
		default:
			return false
		}
	})
	seen := map[string]struct{}{}
	items := make([]string, 0, len(fields))
	for _, field := range fields {
		word := strings.TrimSpace(field)
		if word == "" {
			continue
		}
		if _, ok := seen[word]; ok {
			continue
		}
		seen[word] = struct{}{}
		items = append(items, word)
	}
	return items
}

func workflowTermDictID(ctx context.Context, workflowSvc *appwf.Service, workflowID *uint64) (*uint64, error) {
	if workflowSvc == nil || workflowID == nil || *workflowID == 0 {
		return nil, nil
	}
	workflow, err := workflowSvc.GetWorkflow(ctx, *workflowID)
	if err != nil {
		return nil, err
	}
	for _, node := range workflow.Nodes {
		if node.NodeType != wfdomain.NodeTermCorrection || !node.Enabled {
			continue
		}
		var config struct {
			DictID uint64 `json:"dict_id"`
		}
		if err := json.Unmarshal(node.Config, &config); err != nil {
			continue
		}
		if config.DictID > 0 {
			return &config.DictID, nil
		}
	}
	return nil, nil
}

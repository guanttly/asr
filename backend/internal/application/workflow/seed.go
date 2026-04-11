package workflow

import (
	"context"
	"encoding/json"

	domain "github.com/lgt/asr/internal/domain/workflow"
)

type workflowSeed struct {
	Name        string
	Description string
	Nodes       []domain.Node
}

// EnsureSeedTemplates creates default system workflow templates when they do not exist yet.
func (s *Service) EnsureSeedTemplates(ctx context.Context) error {
	sysType := domain.OwnerSystem
	existing, _, err := s.workflowRepo.List(ctx, &sysType, nil, false, 0, 1000)
	if err != nil {
		return err
	}

	existingNames := make(map[string]struct{}, len(existing))
	for _, item := range existing {
		existingNames[item.Name] = struct{}{}
	}

	for _, seed := range defaultWorkflowSeeds() {
		if _, ok := existingNames[seed.Name]; ok {
			continue
		}

		wf := &domain.Workflow{
			Name:        seed.Name,
			Description: seed.Description,
			OwnerType:   domain.OwnerSystem,
			OwnerID:     0,
			IsPublished: true,
		}
		if err := s.workflowRepo.Create(ctx, wf); err != nil {
			return err
		}

		nodes := make([]domain.Node, len(seed.Nodes))
		for i := range seed.Nodes {
			nodes[i] = seed.Nodes[i]
			nodes[i].WorkflowID = wf.ID
			if nodes[i].Position <= 0 {
				nodes[i].Position = i + 1
			}
		}

		if err := s.nodeRepo.BatchSave(ctx, wf.ID, nodes); err != nil {
			return err
		}
		if err := s.syncWorkflowProfile(ctx, wf, nodes); err != nil {
			return err
		}
	}

	return nil
}

func defaultWorkflowSeeds() []workflowSeed {
	return []workflowSeed{
		{
			Name:        "批量转写整理",
			Description: "适合批量音频转写。以非实时 ASR 作为声明式源节点，后续叠加口语清洗、规则整理与术语纠正。",
			Nodes: []domain.Node{
				seedNode(domain.NodeBatchASR, true, map[string]any{}),
				seedNode(domain.NodeFillerFilter, true, map[string]any{
					"filter_words": []string{"嗯", "啊", "呃", "那个", "就是", "然后"},
					"custom_words": []string{},
				}),
				seedNode(domain.NodeCustomRegex, true, map[string]any{
					"rules": []map[string]any{
						{
							"pattern":     "([，。！？,.])\\1+",
							"replacement": "$1",
							"enabled":     true,
						},
					},
				}),
				seedNode(domain.NodeTermCorrection, false, map[string]any{
					"dict_id": 0,
				}),
				seedNode(domain.NodeLLMCorrection, false, map[string]any{
					"endpoint":        "",
					"model":           "",
					"api_key":         "",
					"prompt_template": "",
					"temperature":     0.3,
					"max_tokens":      4096,
				}),
			},
		},
		{
			Name:        "实时转写整理",
			Description: "适合实时语音识别结束后的文本整理。以实时 ASR 作为声明式源节点，不再与批量入口混用。",
			Nodes: []domain.Node{
				seedNode(domain.NodeRealtimeASR, true, map[string]any{}),
				seedNode(domain.NodeFillerFilter, true, map[string]any{
					"filter_words": []string{"嗯", "啊", "呃", "那个", "就是", "然后"},
					"custom_words": []string{},
				}),
				seedNode(domain.NodeCustomRegex, true, map[string]any{
					"rules": []map[string]any{
						{
							"pattern":     "([，。！？,.])\\1+",
							"replacement": "$1",
							"enabled":     true,
						},
					},
				}),
				seedNode(domain.NodeLLMCorrection, false, map[string]any{
					"endpoint":        "",
					"model":           "",
					"api_key":         "",
					"prompt_template": "",
					"temperature":     0.3,
					"max_tokens":      4096,
				}),
			},
		},
		{
			Name:        "会议纪要工作流",
			Description: "面向会议纪要生成。包含会议纪要节点后会自动被识别为会议类型工作流。",
			Nodes: []domain.Node{
				seedNode(domain.NodeBatchASR, true, map[string]any{}),
				seedNode(domain.NodeSpeakerDiarize, false, map[string]any{
					"service_url":             "",
					"enable_voiceprint_match": false,
					"fail_on_error":           false,
				}),
				seedNode(domain.NodeFillerFilter, true, map[string]any{
					"filter_words": []string{"嗯", "啊", "呃", "那个", "就是", "然后"},
					"custom_words": []string{},
				}),
				seedNode(domain.NodeTermCorrection, false, map[string]any{
					"dict_id": 0,
				}),
				seedNode(domain.NodeLLMCorrection, false, map[string]any{
					"endpoint":        "",
					"model":           "",
					"api_key":         "",
					"prompt_template": "",
					"temperature":     0.3,
					"max_tokens":      4096,
				}),
				seedNode(domain.NodeMeetingSummary, true, map[string]any{
					"output_format": "markdown",
					"max_tokens":    200000,
				}),
			},
		},
		{
			Name:        "规则优先精修",
			Description: "适合批量转写的规则优先文本清洗模板。以非实时 ASR 作为入口声明，再做正则、过滤与术语纠正。",
			Nodes: []domain.Node{
				seedNode(domain.NodeBatchASR, true, map[string]any{}),
				seedNode(domain.NodeCustomRegex, true, map[string]any{
					"rules": []map[string]any{
						{
							"pattern":     "(?m)^\\s+|\\s+$",
							"replacement": "",
							"enabled":     true,
						},
						{
							"pattern":     "([，。！？,.])\\1+",
							"replacement": "$1",
							"enabled":     true,
						},
					},
				}),
				seedNode(domain.NodeFillerFilter, true, map[string]any{
					"filter_words": []string{"嗯", "啊", "呃", "那个", "就是", "然后"},
					"custom_words": []string{},
				}),
				seedNode(domain.NodeTermCorrection, false, map[string]any{
					"dict_id": 0,
				}),
			},
		},
	}
}

func seedNode(nodeType domain.NodeType, enabled bool, config map[string]any) domain.Node {
	return domain.Node{
		NodeType: nodeType,
		Enabled:  enabled,
		Config:   mustJSON(config),
	}
}

func mustJSON(value any) string {
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(data)
}

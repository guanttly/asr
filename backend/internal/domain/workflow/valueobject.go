package workflow

// NodeType defines the type of a workflow node.
type NodeType string

const (
	NodeBatchASR       NodeType = "batch_asr"
	NodeRealtimeASR    NodeType = "realtime_asr"
	NodeTermCorrection NodeType = "term_correction"
	NodeFillerFilter   NodeType = "filler_filter"
	NodeLLMCorrection  NodeType = "llm_correction"
	NodeSpeakerDiarize NodeType = "speaker_diarize"
	NodeMeetingSummary NodeType = "meeting_summary"
	NodeCustomRegex    NodeType = "custom_regex"
)

// AllNodeTypes returns all valid node types.
func AllNodeTypes() []NodeType {
	return []NodeType{
		NodeBatchASR,
		NodeRealtimeASR,
		NodeTermCorrection,
		NodeFillerFilter,
		NodeLLMCorrection,
		NodeSpeakerDiarize,
		NodeMeetingSummary,
		NodeCustomRegex,
	}
}

func (n NodeType) Valid() bool {
	for _, t := range AllNodeTypes() {
		if n == t {
			return true
		}
	}
	return false
}

func (n NodeType) Label() string {
	switch n {
	case NodeBatchASR:
		return "非实时语音转写"
	case NodeRealtimeASR:
		return "实时语音转写"
	case NodeTermCorrection:
		return "术语纠正"
	case NodeFillerFilter:
		return "语气词过滤"
	case NodeLLMCorrection:
		return "LLM 纠错"
	case NodeSpeakerDiarize:
		return "说话人分离"
	case NodeMeetingSummary:
		return "会议纪要生成"
	case NodeCustomRegex:
		return "自定义正则替换"
	default:
		return string(n)
	}
}

func (n NodeType) Description() string {
	switch n {
	case NodeBatchASR:
		return "声明这条工作流面向批量音频转写入口。节点本身不执行 ASR，只用于类型推导和入口约束。"
	case NodeRealtimeASR:
		return "声明这条工作流面向实时语音识别入口。节点本身不执行 ASR，只用于类型推导和入口约束。"
	case NodeTermCorrection:
		return "对转写文本应用术语词库纠正。"
	case NodeFillerFilter:
		return "过滤口语化语气词与停顿词。"
	case NodeLLMCorrection:
		return "调用 OpenAI 兼容接口对文本做进一步纠错。"
	case NodeSpeakerDiarize:
		return "利用音频上下文补充说话人分离信息。"
	case NodeMeetingSummary:
		return "将文本产出为会议纪要摘要。"
	case NodeCustomRegex:
		return "按自定义正则规则批量替换文本。"
	default:
		return ""
	}
}

func (n NodeType) Role() string {
	if n.IsSource() {
		return "source"
	}
	if n.IsSink() {
		return "sink"
	}
	return "transform"
}

func (n NodeType) IsSource() bool {
	return n == NodeBatchASR || n == NodeRealtimeASR
}

func (n NodeType) IsSink() bool {
	return n == NodeMeetingSummary
}

type WorkflowType string

const (
	WorkflowTypeLegacy   WorkflowType = "legacy"
	WorkflowTypeBatch    WorkflowType = "batch_transcription"
	WorkflowTypeRealtime WorkflowType = "realtime_transcription"
	WorkflowTypeMeeting  WorkflowType = "meeting"
)

func (t WorkflowType) Label() string {
	switch t {
	case WorkflowTypeBatch:
		return "批量转写"
	case WorkflowTypeRealtime:
		return "实时语音识别"
	case WorkflowTypeMeeting:
		return "会议纪要"
	case WorkflowTypeLegacy:
		return "旧版文本后处理"
	default:
		return string(t)
	}
}

type WorkflowSourceKind string

const (
	SourceKindLegacyText  WorkflowSourceKind = "legacy_text"
	SourceKindBatchASR    WorkflowSourceKind = "batch_asr"
	SourceKindRealtimeASR WorkflowSourceKind = "realtime_asr"
)

func (k WorkflowSourceKind) Label() string {
	switch k {
	case SourceKindBatchASR:
		return "非实时语音转写"
	case SourceKindRealtimeASR:
		return "实时语音转写"
	case SourceKindLegacyText:
		return "旧版文本输入"
	default:
		return string(k)
	}
}

type WorkflowTargetKind string

const (
	TargetKindTranscript     WorkflowTargetKind = "transcript"
	TargetKindMeetingSummary WorkflowTargetKind = "meeting_summary"
)

func (k WorkflowTargetKind) Label() string {
	switch k {
	case TargetKindMeetingSummary:
		return "会议纪要"
	case TargetKindTranscript:
		return "整理后文本"
	default:
		return string(k)
	}
}

// OwnerType defines the ownership of a workflow.
type OwnerType string

const (
	OwnerSystem OwnerType = "system"
	OwnerUser   OwnerType = "user"
)

// TriggerType defines what triggered a workflow execution.
type TriggerType string

const (
	TriggerBatchTask TriggerType = "batch_task"
	TriggerRealtime  TriggerType = "realtime"
	TriggerManual    TriggerType = "manual"
)

// ExecutionStatus defines the status of a workflow execution.
type ExecutionStatus string

const (
	ExecStatusPending   ExecutionStatus = "pending"
	ExecStatusRunning   ExecutionStatus = "running"
	ExecStatusCompleted ExecutionStatus = "completed"
	ExecStatusFailed    ExecutionStatus = "failed"
)

// NodeResultStatus defines the status of a single node execution.
type NodeResultStatus string

const (
	NodeResultPending NodeResultStatus = "pending"
	NodeResultSuccess NodeResultStatus = "success"
	NodeResultFailed  NodeResultStatus = "failed"
	NodeResultSkipped NodeResultStatus = "skipped"
)

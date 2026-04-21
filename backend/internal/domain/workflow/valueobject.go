package workflow

// NodeType defines the type of a workflow node.
type NodeType string

const (
	NodeBatchASR        NodeType = "batch_asr"
	NodeRealtimeASR     NodeType = "realtime_asr"
	NodeVoiceWake       NodeType = "voice_wake"
	NodeTermCorrection  NodeType = "term_correction"
	NodeFillerFilter    NodeType = "filler_filter"
	NodeSensitiveFilter NodeType = "sensitive_filter"
	NodeLLMCorrection   NodeType = "llm_correction"
	NodeVoiceIntent     NodeType = "voice_intent"
	NodeSpeakerDiarize  NodeType = "speaker_diarize"
	NodeMeetingSummary  NodeType = "meeting_summary"
	NodeCustomRegex     NodeType = "custom_regex"
)

// AllNodeTypes returns all valid node types.
func AllNodeTypes() []NodeType {
	return []NodeType{
		NodeBatchASR,
		NodeRealtimeASR,
		NodeVoiceWake,
		NodeTermCorrection,
		NodeFillerFilter,
		NodeSensitiveFilter,
		NodeLLMCorrection,
		NodeVoiceIntent,
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
	case NodeVoiceWake:
		return "唤醒词识别"
	case NodeTermCorrection:
		return "术语纠正"
	case NodeFillerFilter:
		return "语气词过滤"
	case NodeSensitiveFilter:
		return "敏感词过滤"
	case NodeLLMCorrection:
		return "LLM 纠错"
	case NodeVoiceIntent:
		return "语音控制意图识别"
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
	case NodeVoiceWake:
		return "从转写文本中识别唤醒词、同音词和尾随控制指令，为语音控制工作流提供入口判断。"
	case NodeTermCorrection:
		return "对转写文本应用术语词库纠正。"
	case NodeFillerFilter:
		return "过滤口语化语气词与停顿词。"
	case NodeSensitiveFilter:
		return "按敏感词列表做替换或掩码处理。"
	case NodeLLMCorrection:
		return "调用 OpenAI 兼容接口对文本做进一步纠错。"
	case NodeVoiceIntent:
		return "根据控制指令库与节点提示词，将语音转写文本识别为结构化控制意图。"
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
	return n == NodeBatchASR || n == NodeRealtimeASR || n == NodeVoiceWake
}

func (n NodeType) IsSink() bool {
	return n == NodeMeetingSummary || n == NodeVoiceIntent
}

type WorkflowType string

const (
	WorkflowTypeLegacy   WorkflowType = "legacy"
	WorkflowTypeBatch    WorkflowType = "batch_transcription"
	WorkflowTypeRealtime WorkflowType = "realtime_transcription"
	WorkflowTypeMeeting  WorkflowType = "meeting"
	WorkflowTypeVoice    WorkflowType = "voice_control"
)

func (t WorkflowType) Label() string {
	switch t {
	case WorkflowTypeBatch:
		return "批量转写"
	case WorkflowTypeRealtime:
		return "实时语音识别"
	case WorkflowTypeMeeting:
		return "会议纪要"
	case WorkflowTypeVoice:
		return "语音控制"
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
	SourceKindVoiceWake   WorkflowSourceKind = "voice_wake"
)

func (k WorkflowSourceKind) NodeType() (NodeType, bool) {
	switch k {
	case SourceKindBatchASR:
		return NodeBatchASR, true
	case SourceKindRealtimeASR:
		return NodeRealtimeASR, true
	case SourceKindVoiceWake:
		return NodeVoiceWake, true
	default:
		return "", false
	}
}

func (k WorkflowSourceKind) Label() string {
	switch k {
	case SourceKindBatchASR:
		return "非实时语音转写"
	case SourceKindRealtimeASR:
		return "实时语音转写"
	case SourceKindVoiceWake:
		return "唤醒词识别"
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
	TargetKindVoiceCommand   WorkflowTargetKind = "voice_command"
)

func (k WorkflowTargetKind) FixedSinkNodeType() (NodeType, bool) {
	switch k {
	case TargetKindMeetingSummary:
		return NodeMeetingSummary, true
	case TargetKindVoiceCommand:
		return NodeVoiceIntent, true
	default:
		return "", false
	}
}

func (k WorkflowTargetKind) Label() string {
	switch k {
	case TargetKindMeetingSummary:
		return "会议纪要"
	case TargetKindVoiceCommand:
		return "控制指令结果"
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

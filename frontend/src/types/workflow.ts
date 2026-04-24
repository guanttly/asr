export const WORKFLOW_BINDING_KEYS = {
  REALTIME: 'realtime',
  BATCH: 'batch',
  MEETING: 'meeting',
  VOICE_CONTROL: 'voice_control',
} as const

export type WorkflowBindingKey = typeof WORKFLOW_BINDING_KEYS[keyof typeof WORKFLOW_BINDING_KEYS]

export const WORKFLOW_OWNER_TYPES = {
  SYSTEM: 'system',
  USER: 'user',
} as const

export type WorkflowOwnerType = typeof WORKFLOW_OWNER_TYPES[keyof typeof WORKFLOW_OWNER_TYPES]

export const WORKFLOW_TYPES = {
  LEGACY: 'legacy',
  BATCH: 'batch_transcription',
  REALTIME: 'realtime_transcription',
  MEETING: 'meeting',
  VOICE_CONTROL: 'voice_control',
} as const

export type WorkflowType = typeof WORKFLOW_TYPES[keyof typeof WORKFLOW_TYPES]
export type ActiveWorkflowType = Exclude<WorkflowType, typeof WORKFLOW_TYPES.LEGACY>

export const WORKFLOW_SOURCE_KINDS = {
  LEGACY_TEXT: 'legacy_text',
  BATCH_ASR: 'batch_asr',
  REALTIME_ASR: 'realtime_asr',
  VOICE_WAKE: 'voice_wake',
} as const

export type WorkflowSourceKind = typeof WORKFLOW_SOURCE_KINDS[keyof typeof WORKFLOW_SOURCE_KINDS]

export const WORKFLOW_TARGET_KINDS = {
  TRANSCRIPT: 'transcript',
  MEETING_SUMMARY: 'meeting_summary',
  VOICE_COMMAND: 'voice_command',
} as const

export type WorkflowTargetKind = typeof WORKFLOW_TARGET_KINDS[keyof typeof WORKFLOW_TARGET_KINDS]

export interface WorkflowPreviewNode {
  id?: number
  label?: string
  node_type?: string
  enabled?: boolean
}

export interface WorkflowCatalogItem {
  id: number
  name: string
  description?: string
  owner_type?: WorkflowOwnerType
  workflow_type?: WorkflowType
  source_kind?: WorkflowSourceKind
  target_kind?: WorkflowTargetKind
  is_legacy?: boolean
  validation_message?: string
  nodes?: WorkflowPreviewNode[]
}

export interface WorkflowPreviewItem {
  id: number
  label: string
  description?: string
  owner_type?: WorkflowOwnerType
  workflow_type?: WorkflowType
  source_kind?: WorkflowSourceKind
  target_kind?: WorkflowTargetKind
  is_legacy?: boolean
  validation_message?: string
  nodes?: WorkflowPreviewNode[]
}

export type WorkflowBindings = Record<WorkflowBindingKey, number | null>

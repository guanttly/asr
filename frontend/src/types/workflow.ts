export type WorkflowBindingKey = 'realtime' | 'batch' | 'meeting' | 'voice_control'
export type WorkflowOwnerType = 'system' | 'user'
export type WorkflowType = 'legacy' | 'batch_transcription' | 'realtime_transcription' | 'meeting' | 'voice_control'
export type ActiveWorkflowType = Exclude<WorkflowType, 'legacy'>
export type WorkflowSourceKind = 'legacy_text' | 'batch_asr' | 'realtime_asr' | 'voice_wake'
export type WorkflowTargetKind = 'transcript' | 'meeting_summary' | 'voice_command'

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

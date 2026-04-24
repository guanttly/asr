export const TRANSCRIPTION_TASK_TYPES = {
  REALTIME: 'realtime',
  BATCH: 'batch',
} as const

export type TranscriptionTaskType = typeof TRANSCRIPTION_TASK_TYPES[keyof typeof TRANSCRIPTION_TASK_TYPES]

export const TRANSCRIPTION_TASK_TYPE_LABELS: Record<TranscriptionTaskType, string> = {
  [TRANSCRIPTION_TASK_TYPES.REALTIME]: '实时',
  [TRANSCRIPTION_TASK_TYPES.BATCH]: '批量',
}
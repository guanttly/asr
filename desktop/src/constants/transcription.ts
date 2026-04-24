export const TRANSCRIPTION_TASK_TYPES = {
  REALTIME: 'realtime',
  BATCH: 'batch',
} as const

export type TranscriptionTaskType = typeof TRANSCRIPTION_TASK_TYPES[keyof typeof TRANSCRIPTION_TASK_TYPES]
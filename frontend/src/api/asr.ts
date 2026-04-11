import request from './request'

export function createTranscriptionTask(payload: { audio_url?: string, stream_session_id?: string, type: 'realtime' | 'batch', dict_id?: number, workflow_id?: number, result_text?: string, duration?: number }) {
  return request.post('/api/asr/tasks', payload)
}

export function uploadRealtimeSessionTask(payload: FormData) {
  return request.post('/api/asr/realtime-tasks/upload', payload, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    timeout: 0,
  })
}

export function uploadTranscriptionFile(payload: FormData) {
  return request.post('/api/asr/tasks/upload', payload, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    timeout: 0,
  })
}

export function transcribeRealtimeSegment(payload: FormData) {
  return request.post('/api/asr/realtime-segments', payload, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    timeout: 0,
  })
}

export function startRealtimeStreamSession() {
  return request.post('/api/asr/stream-sessions')
}

export function pushRealtimeStreamChunk(sessionId: string | number, payload: ArrayBuffer | Blob | Uint8Array) {
  return request.post(`/api/asr/stream-sessions/${sessionId}/chunks`, payload, {
    headers: {
      'Content-Type': 'application/octet-stream',
    },
    timeout: 0,
  })
}

export function commitRealtimeStreamSession(sessionId: string | number) {
  return request.post(`/api/asr/stream-sessions/${sessionId}/commit`)
}

export function finishRealtimeStreamSession(sessionId: string | number) {
  return request.post(`/api/asr/stream-sessions/${sessionId}/finish`)
}

export function getTranscriptionTasks(params?: { offset?: number, limit?: number }) {
  return request.get('/api/asr/tasks', { params })
}

export function getTranscriptionTaskDetail(taskId: string | number) {
  return request.get(`/api/asr/tasks/${taskId}`)
}

export function deleteTranscriptionTask(taskId: string | number) {
  return request.delete(`/api/asr/tasks/${taskId}`)
}

export function getTranscriptionTaskExecutions(taskId: string | number) {
  return request.get(`/api/asr/tasks/${taskId}/executions`)
}

export function resumeTranscriptionTaskPostProcess(taskId: string | number) {
  return request.post(`/api/asr/tasks/${taskId}/resume-post-process`)
}

export function syncTranscriptionTask(taskId: string | number) {
  return request.post(`/api/asr/tasks/${taskId}/sync`)
}

import request from './request'

export function createTranscriptionTask(payload: { audio_url?: string, type: 'realtime' | 'batch', dict_id?: number, workflow_id?: number, result_text?: string, duration?: number }) {
  return request.post('/api/asr/tasks', payload)
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

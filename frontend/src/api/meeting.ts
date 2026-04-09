import request from './request'

export function getMeetings(params?: { offset?: number, limit?: number }) {
  return request.get('/api/meetings', { params })
}

export function createMeeting(payload: { title: string, audio_url: string, duration?: number, workflow_id?: number }) {
  return request.post('/api/meetings', payload)
}

export function uploadMeetingFile(payload: FormData) {
  return request.post('/api/meetings/upload', payload, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    timeout: 0,
  })
}

export function deleteMeeting(id: string | number) {
  return request.delete(`/api/meetings/${id}`)
}

export function getMeetingDetail(id: string | number) {
  return request.get(`/api/meetings/${id}`)
}

export function regenerateMeetingSummary(id: string | number, payload: { workflow_id?: number }) {
  return request.post(`/api/meetings/${id}/summary`, payload)
}

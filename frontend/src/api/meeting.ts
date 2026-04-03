import request from './request'

export function getMeetings(params?: { offset?: number, limit?: number }) {
  return request.get('/api/meetings', { params })
}

export function createMeeting(payload: { title: string, audio_url: string, duration?: number }) {
  return request.post('/api/meetings', payload)
}

export function getMeetingDetail(id: string | number) {
  return request.get(`/api/meetings/${id}`)
}
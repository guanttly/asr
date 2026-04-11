import request from './request'

export interface VoiceprintItem {
  id: string
  speaker_name: string
  department?: string
  notes?: string
  audio_duration: number
  created_at?: string
  updated_at?: string
}

export function getVoiceprints() {
  return request.get('/api/meetings/voiceprints')
}

export function createVoiceprint(payload: FormData) {
  return request.post('/api/meetings/voiceprints', payload, {
    headers: {
      'Content-Type': 'multipart/form-data',
    },
    timeout: 0,
  })
}

export function deleteVoiceprint(id: string) {
  return request.delete(`/api/meetings/voiceprints/${encodeURIComponent(id)}`)
}
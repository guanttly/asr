import request from './request'

export interface VoiceCommandDictItem {
  id: number
  name: string
  group_key: string
  description?: string
  is_base: boolean
}

export interface VoiceCommandEntryItem {
  id: number
  intent: string
  label: string
  utterances: string[]
  enabled: boolean
  sort_order: number
}

export function getVoiceCommandDicts(params?: { offset?: number, limit?: number }) {
  return request.get('/api/admin/voice-command-dicts', { params })
}

export function createVoiceCommandDict(payload: { name: string, group_key: string, description?: string, is_base?: boolean }) {
  return request.post('/api/admin/voice-command-dicts', payload)
}

export function updateVoiceCommandDict(id: number, payload: { name: string, group_key: string, description?: string, is_base?: boolean }) {
  return request.put(`/api/admin/voice-command-dicts/${id}`, payload)
}

export function deleteVoiceCommandDict(id: number) {
  return request.delete(`/api/admin/voice-command-dicts/${id}`)
}

export function getVoiceCommandEntries(dictId: number) {
  return request.get(`/api/admin/voice-command-dicts/${dictId}/entries`)
}

export function createVoiceCommandEntry(dictId: number, payload: { intent: string, label: string, utterances: string[], enabled: boolean, sort_order?: number }) {
  return request.post(`/api/admin/voice-command-dicts/${dictId}/entries`, payload)
}

export function updateVoiceCommandEntry(dictId: number, entryId: number, payload: { intent: string, label: string, utterances: string[], enabled: boolean, sort_order?: number }) {
  return request.put(`/api/admin/voice-command-dicts/${dictId}/entries/${entryId}`, payload)
}

export function deleteVoiceCommandEntry(dictId: number, entryId: number) {
  return request.delete(`/api/admin/voice-command-dicts/${dictId}/entries/${entryId}`)
}
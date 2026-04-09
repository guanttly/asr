import request from './request'

export function getTermDicts(params?: { offset?: number, limit?: number }) {
  return request.get('/api/admin/term-dicts', { params })
}

export function createTermDict(payload: { name: string, domain: string }) {
  return request.post('/api/admin/term-dicts', payload)
}

export function updateTermDict(dictId: string | number, payload: { name: string, domain: string }) {
  return request.put(`/api/admin/term-dicts/${dictId}`, payload)
}

export function deleteTermDict(dictId: string | number) {
  return request.delete(`/api/admin/term-dicts/${dictId}`)
}

export function getTermEntries(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/entries`)
}

export function createTermEntry(dictId: string | number, payload: { correct_term: string, wrong_variants: string[], pinyin?: string }) {
  return request.post(`/api/admin/term-dicts/${dictId}/entries`, payload)
}

export function updateTermEntry(dictId: string | number, entryId: string | number, payload: { correct_term: string, wrong_variants: string[], pinyin?: string }) {
  return request.put(`/api/admin/term-dicts/${dictId}/entries/${entryId}`, payload)
}

export function deleteTermEntry(dictId: string | number, entryId: string | number) {
  return request.delete(`/api/admin/term-dicts/${dictId}/entries/${entryId}`)
}

export function getTermRules(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/rules`)
}

export function createTermRule(dictId: string | number, payload: { layer: number, pattern: string, replacement: string, enabled: boolean }) {
  return request.post(`/api/admin/term-dicts/${dictId}/rules`, payload)
}

export function updateTermRule(dictId: string | number, ruleId: string | number, payload: { layer: number, pattern: string, replacement: string, enabled: boolean }) {
  return request.put(`/api/admin/term-dicts/${dictId}/rules/${ruleId}`, payload)
}

export function deleteTermRule(dictId: string | number, ruleId: string | number) {
  return request.delete(`/api/admin/term-dicts/${dictId}/rules/${ruleId}`)
}

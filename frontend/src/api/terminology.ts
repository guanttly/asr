import request from './request'

export function getTermDicts(params?: { offset?: number, limit?: number }) {
  return request.get('/api/admin/term-dicts', { params })
}

export function createTermDict(payload: { name: string, domain: string }) {
  return request.post('/api/admin/term-dicts', payload)
}

export function getTermEntries(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/entries`)
}

export function createTermEntry(dictId: string | number, payload: { correct_term: string, wrong_variants: string[] }) {
  return request.post(`/api/admin/term-dicts/${dictId}/entries`, payload)
}

export function getTermRules(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/rules`)
}

export function createTermRule(dictId: string | number, payload: { layer: number, pattern: string, replacement: string, enabled: boolean }) {
  return request.post(`/api/admin/term-dicts/${dictId}/rules`, payload)
}
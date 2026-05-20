import request from './request'

export interface TermDictPayload {
  name: string
  domain: string
  rule_processing_enabled?: boolean
  text_replacement_enabled?: boolean
}

export function getTermDicts(params?: { offset?: number, limit?: number }) {
  return request.get('/api/admin/term-dicts', { params })
}

export function createTermDict(payload: TermDictPayload) {
  return request.post('/api/admin/term-dicts', payload)
}

export function updateTermDict(dictId: string | number, payload: TermDictPayload) {
  return request.put(`/api/admin/term-dicts/${dictId}`, payload)
}

export function deleteTermDict(dictId: string | number) {
  return request.delete(`/api/admin/term-dicts/${dictId}`)
}

export function getTermEntries(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/entries`)
}

export function createTermEntry(dictId: string | number, payload: { correct_term: string, wrong_variants: string[] }) {
  return request.post(`/api/admin/term-dicts/${dictId}/entries`, payload)
}

export function updateTermEntry(dictId: string | number, entryId: string | number, payload: { correct_term: string, wrong_variants: string[] }) {
  return request.put(`/api/admin/term-dicts/${dictId}/entries/${entryId}`, payload)
}

export function deleteTermEntry(dictId: string | number, entryId: string | number) {
  return request.delete(`/api/admin/term-dicts/${dictId}/entries/${entryId}`)
}

export function clearTermEntries(dictId: string | number) {
  return request.delete<{ deleted: number }>(`/api/admin/term-dicts/${dictId}/entries`)
}

export function importTermEntries(dictId: string | number, payload: FormData) {
  return request.post<{ imported: number, skipped: number }>(`/api/admin/term-dicts/${dictId}/import`, payload, {
    timeout: 120000,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  })
}

export function exportTermEntries(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/export`, {
    responseType: 'blob',
  })
}

export function downloadTermImportTemplate() {
  return request.get('/api/admin/term-dicts/import-template', {
    responseType: 'blob',
  })
}

export function getTermRules(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/rules`)
}

export function importTermRules(dictId: string | number, payload: FormData) {
  return request.post<{ imported: number }>(`/api/admin/term-dicts/${dictId}/rules/import`, payload, {
    timeout: 120000,
    headers: {
      'Content-Type': 'multipart/form-data',
    },
  })
}

export function exportTermRules(dictId: string | number) {
  return request.get(`/api/admin/term-dicts/${dictId}/rules/export`, {
    responseType: 'blob',
  })
}

export function createTermRule(dictId: string | number, payload: { match_type: string, pattern: string, replacement: string, enabled: boolean, sort_order: number, priority?: number, conflict_group?: string }) {
  return request.post(`/api/admin/term-dicts/${dictId}/rules`, payload)
}

export function updateTermRule(dictId: string | number, ruleId: string | number, payload: { match_type: string, pattern: string, replacement: string, enabled: boolean, sort_order: number, priority?: number, conflict_group?: string }) {
  return request.put(`/api/admin/term-dicts/${dictId}/rules/${ruleId}`, payload)
}

export function deleteTermRule(dictId: string | number, ruleId: string | number) {
  return request.delete(`/api/admin/term-dicts/${dictId}/rules/${ruleId}`)
}

export function clearTermRules(dictId: string | number) {
  return request.delete<{ deleted: number }>(`/api/admin/term-dicts/${dictId}/rules`)
}

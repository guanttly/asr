import request from './request'

export interface SensitiveDictItem {
	id: number
	name: string
	scene: string
	description: string
	is_base: boolean
}

export interface SensitiveEntryItem {
	id: number
	word: string
	enabled: boolean
}

export function getSensitiveDicts(params?: { offset?: number, limit?: number }) {
	return request.get('/api/admin/sensitive-dicts', { params })
}

export function createSensitiveDict(payload: { name: string, scene: string, description?: string, is_base: boolean }) {
	return request.post('/api/admin/sensitive-dicts', payload)
}

export function updateSensitiveDict(dictId: string | number, payload: { name: string, scene: string, description?: string, is_base: boolean }) {
	return request.put(`/api/admin/sensitive-dicts/${dictId}`, payload)
}

export function deleteSensitiveDict(dictId: string | number) {
	return request.delete(`/api/admin/sensitive-dicts/${dictId}`)
}

export function getSensitiveEntries(dictId: string | number) {
	return request.get(`/api/admin/sensitive-dicts/${dictId}/entries`)
}

export function createSensitiveEntry(dictId: string | number, payload: { word: string, enabled: boolean }) {
	return request.post(`/api/admin/sensitive-dicts/${dictId}/entries`, payload)
}

export function updateSensitiveEntry(dictId: string | number, entryId: string | number, payload: { word: string, enabled: boolean }) {
	return request.put(`/api/admin/sensitive-dicts/${dictId}/entries/${entryId}`, payload)
}

export function deleteSensitiveEntry(dictId: string | number, entryId: string | number) {
	return request.delete(`/api/admin/sensitive-dicts/${dictId}/entries/${entryId}`)
}
import request from './request'

export interface FillerDictItem {
	id: number
	name: string
	scene: string
	description: string
	is_base: boolean
}

export interface FillerEntryItem {
	id: number
	word: string
	enabled: boolean
}

export function getFillerDicts(params?: { offset?: number, limit?: number }) {
	return request.get('/api/admin/filler-dicts', { params })
}

export function createFillerDict(payload: { name: string, scene: string, description?: string, is_base: boolean }) {
	return request.post('/api/admin/filler-dicts', payload)
}

export function updateFillerDict(dictId: string | number, payload: { name: string, scene: string, description?: string, is_base: boolean }) {
	return request.put(`/api/admin/filler-dicts/${dictId}`, payload)
}

export function deleteFillerDict(dictId: string | number) {
	return request.delete(`/api/admin/filler-dicts/${dictId}`)
}

export function getFillerEntries(dictId: string | number) {
	return request.get(`/api/admin/filler-dicts/${dictId}/entries`)
}

export function createFillerEntry(dictId: string | number, payload: { word: string, enabled: boolean }) {
	return request.post(`/api/admin/filler-dicts/${dictId}/entries`, payload)
}

export function updateFillerEntry(dictId: string | number, entryId: string | number, payload: { word: string, enabled: boolean }) {
	return request.put(`/api/admin/filler-dicts/${dictId}/entries/${entryId}`, payload)
}

export function deleteFillerEntry(dictId: string | number, entryId: string | number) {
	return request.delete(`/api/admin/filler-dicts/${dictId}/entries/${entryId}`)
}
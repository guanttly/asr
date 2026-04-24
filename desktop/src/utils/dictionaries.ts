import { authedFetch, readResponseEnvelope } from './auth'

export interface TermDict {
  id: number
  name: string
  domain: string
}

export interface SensitiveDict {
  id: number
  name: string
  scene: string
  description?: string
  is_base?: boolean
}

export interface DictListResult<T> {
  items: T[]
  total: number
}

function buildQuery(params: Record<string, string | number | undefined>) {
  const search = new URLSearchParams()
  Object.entries(params).forEach(([key, value]) => {
    if (value == null || value === '')
      return
    search.set(key, String(value))
  })
  const query = search.toString()
  return query ? `?${query}` : ''
}

// ------------------------ 术语词库 ------------------------

export async function listTermDicts(params?: { offset?: number, limit?: number }) {
  const response = await authedFetch(`/api/admin/term-dicts${buildQuery({
    offset: params?.offset,
    limit: params?.limit ?? 100,
  })}`)
  const payload = await readResponseEnvelope<DictListResult<TermDict>>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载术语词库失败')
  return payload.data || { items: [], total: 0 }
}

export async function createTermDict(payload: { name: string, domain: string }): Promise<TermDict> {
  const response = await authedFetch('/api/admin/term-dicts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const envelope = await readResponseEnvelope<TermDict>(response)
  if (!response.ok || !envelope.data)
    throw new Error(envelope.message || '创建术语词库失败')
  return envelope.data
}

export async function createTermEntry(dictID: number, body: {
  correct_term: string
  wrong_variants?: string[]
  pinyin?: string
}) {
  const response = await authedFetch(`/api/admin/term-dicts/${dictID}/entries`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
  })
  const envelope = await readResponseEnvelope<unknown>(response)
  if (!response.ok)
    throw new Error(envelope.message || '加入术语词库失败')
}

// ------------------------ 敏感词库 ------------------------

export async function listSensitiveDicts(params?: { offset?: number, limit?: number }) {
  const response = await authedFetch(`/api/admin/sensitive-dicts${buildQuery({
    offset: params?.offset,
    limit: params?.limit ?? 100,
  })}`)
  const payload = await readResponseEnvelope<DictListResult<SensitiveDict>>(response)
  if (!response.ok)
    throw new Error(payload.message || '加载敏感词库失败')
  return payload.data || { items: [], total: 0 }
}

export async function createSensitiveDict(payload: { name: string, scene: string, description?: string, is_base?: boolean }): Promise<SensitiveDict> {
  const response = await authedFetch('/api/admin/sensitive-dicts', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(payload),
  })
  const envelope = await readResponseEnvelope<SensitiveDict>(response)
  if (!response.ok || !envelope.data)
    throw new Error(envelope.message || '创建敏感词库失败')
  return envelope.data
}

export async function createSensitiveEntry(dictID: number, body: { word: string, enabled?: boolean }) {
  const response = await authedFetch(`/api/admin/sensitive-dicts/${dictID}/entries`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ enabled: true, ...body }),
  })
  const envelope = await readResponseEnvelope<unknown>(response)
  if (!response.ok)
    throw new Error(envelope.message || '加入敏感词库失败')
}

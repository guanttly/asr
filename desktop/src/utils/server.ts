const RAW_DEFAULT_SERVER_URL = typeof import.meta.env.VITE_DEFAULT_SERVER_URL === 'string'
  ? import.meta.env.VITE_DEFAULT_SERVER_URL
  : 'http://127.0.0.1:10010'

export function normalizeServerUrl(raw?: null | string) {
  let value = (raw || '').trim()
  if (!value)
    value = RAW_DEFAULT_SERVER_URL.trim()
  if (!/^https?:\/\//i.test(value))
    value = `http://${value}`
  return value.replace(/\/+$/, '')
}

export const DEFAULT_SERVER_URL = normalizeServerUrl(RAW_DEFAULT_SERVER_URL)

export function buildServerCandidates(raw?: null | string) {
  const primary = normalizeServerUrl(raw)
  const candidates = [primary]
  if (primary.startsWith('https://'))
    candidates.push(`http://${primary.slice('https://'.length)}`)
  return [...new Set(candidates)]
}

export function describeNetworkError(error: unknown, candidates: string[]) {
  const fallback = candidates.find(candidate => candidate.startsWith('http://')) || DEFAULT_SERVER_URL
  const detail = error instanceof Error && error.message.trim()
    ? error.message.trim()
    : '无法连接服务器'
  return `${detail}。请确认服务地址和协议是否正确；当前网关默认使用 HTTP，例如 ${fallback}`
}
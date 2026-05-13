const RAW_DEFAULT_SERVER_URL = typeof import.meta.env.VITE_DEFAULT_SERVER_URL === 'string'
  ? import.meta.env.VITE_DEFAULT_SERVER_URL
  : 'http://127.0.0.1:10010'

const RAW_FALLBACK_SERVER_URL = typeof import.meta.env.VITE_FALLBACK_SERVER_URL === 'string'
  ? import.meta.env.VITE_FALLBACK_SERVER_URL
  : ''

function normalizeOptionalServerUrl(raw?: null | string) {
  const value = (raw || '').trim()
  if (!value)
    return ''
  return normalizeServerUrl(value)
}

function parseServerUrl(raw?: null | string) {
  try {
    return new URL(normalizeServerUrl(raw))
  }
  catch {
    return null
  }
}

function appendCandidate(candidates: string[], raw?: null | string) {
  const candidate = normalizeOptionalServerUrl(raw)
  if (!candidate || candidates.includes(candidate))
    return
  candidates.push(candidate)
}

export function normalizeServerUrl(raw?: null | string) {
  let value = (raw || '').trim()
  if (!value)
    value = RAW_DEFAULT_SERVER_URL.trim()
  if (!/^https?:\/\//i.test(value))
    value = `http://${value}`
  return value.replace(/\/+$/, '')
}

export const DEFAULT_SERVER_URL = normalizeServerUrl(RAW_DEFAULT_SERVER_URL)
export const FALLBACK_SERVER_URL = normalizeOptionalServerUrl(RAW_FALLBACK_SERVER_URL)

export function buildServerCandidates(raw?: null | string) {
  const primary = normalizeServerUrl(raw)
  const candidates = [primary]
  const packagedCandidates = [DEFAULT_SERVER_URL, FALLBACK_SERVER_URL].filter(Boolean)

  if (packagedCandidates.includes(primary)) {
    packagedCandidates.forEach(candidate => appendCandidate(candidates, candidate))
    return candidates
  }

  const primaryUrl = parseServerUrl(primary)
  const packagedUrls = packagedCandidates
    .map(candidate => parseServerUrl(candidate))
    .filter((candidate): candidate is URL => candidate != null)

  if (!primaryUrl || packagedUrls.length === 0)
    return candidates

  const siblingCandidates = packagedUrls.filter((candidate) => {
    return candidate.hostname.toLowerCase() === primaryUrl.hostname.toLowerCase()
      && candidate.port === primaryUrl.port
  })

  if (siblingCandidates.length === 0)
    return candidates

  siblingCandidates.forEach(candidate => appendCandidate(candidates, candidate.toString()))
  packagedCandidates.forEach(candidate => appendCandidate(candidates, candidate))
  return candidates
}

export function describeNetworkError(error: unknown, candidates: string[]) {
  const current = candidates[0] || DEFAULT_SERVER_URL
  const alternates = candidates.slice(1)
  const detail = error instanceof Error && error.message.trim()
    ? error.message.trim()
    : '无法连接服务器'
  const fallbackDetail = alternates.length > 0
    ? `；还尝试过 ${alternates.join('、')}`
    : ''
  return `${detail}。请确认服务地址和协议是否正确；当前客户端正在使用 ${current}${fallbackDetail}`
}
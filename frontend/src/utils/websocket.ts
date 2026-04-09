function isLoopbackHost(hostname: string) {
  const normalized = hostname.trim().toLowerCase()
  return normalized === 'localhost' || normalized === '127.0.0.1' || normalized === '::1'
}

function currentWebSocketOrigin() {
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:'
  return `${protocol}//${window.location.host}`
}

export function resolveWebSocketURL(path: string) {
  const configuredBase = String(import.meta.env.VITE_WS_BASE_URL || '').trim()

  if (!configuredBase)
    return new URL(path, `${currentWebSocketOrigin()}/`)

  const configuredURL = new URL(configuredBase)
  if (!isLoopbackHost(configuredURL.hostname) || isLoopbackHost(window.location.hostname))
    return new URL(path, `${configuredURL.toString().replace(/\/$/, '')}/`)

  return new URL(path, `${currentWebSocketOrigin()}/`)
}

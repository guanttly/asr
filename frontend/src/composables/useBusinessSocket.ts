import { computed, ref, shallowRef } from 'vue'

export interface BusinessSocketEvent<T = unknown> {
  type: string
  topic?: string
  business?: string
  payload?: T
  timestamp?: string
}

export type BusinessSocketStatus = 'idle' | 'connecting' | 'connected' | 'reconnecting' | 'error'

type BusinessEventHandler = (event: BusinessSocketEvent) => void

const socket = shallowRef<WebSocket | null>(null)
const connected = ref(false)
const status = ref<BusinessSocketStatus>('idle')
const lastError = ref('')
const lastMessageAt = ref<string | null>(null)
const lastPongAt = ref<string | null>(null)
const desiredTopics = new Set<string>()
const handlers = new Map<string, Set<BusinessEventHandler>>()
const desiredTopicCount = ref(0)

let manualClose = false
let reconnectTimer: ReturnType<typeof setTimeout> | null = null
let heartbeatTimer: ReturnType<typeof setInterval> | null = null
let authToken = ''
let connectingPromise: Promise<void> | null = null
let socketPath = '/ws/events'

const subscribedTopicCount = computed(() => desiredTopicCount.value)

function syncDesiredTopicCount() {
  desiredTopicCount.value = desiredTopics.size
}

function nowISO() {
  return new Date().toISOString()
}

function stopHeartbeat() {
  if (heartbeatTimer)
    clearInterval(heartbeatTimer)
  heartbeatTimer = null
}

function startHeartbeat() {
  stopHeartbeat()
  heartbeatTimer = setInterval(() => {
    if (socket.value?.readyState === WebSocket.OPEN)
      sendJSON({ type: 'ping' })
  }, 20000)
}

function clearReconnectTimer() {
  if (reconnectTimer)
    clearTimeout(reconnectTimer)
  reconnectTimer = null
}

function scheduleReconnect() {
  if (manualClose || !authToken)
    return
  clearReconnectTimer()
  status.value = 'reconnecting'
  reconnectTimer = setTimeout(() => {
    connect(authToken, socketPath).catch(() => undefined)
  }, 1500)
}

function buildWebSocketURL(path: string, token: string) {
  const wsURL = new URL(`${import.meta.env.VITE_WS_BASE_URL}${path}`)
  if (token)
    wsURL.searchParams.set('token', token)
  return wsURL
}

function sendJSON(payload: Record<string, unknown>) {
  if (socket.value?.readyState === WebSocket.OPEN)
    socket.value.send(JSON.stringify(payload))
}

function resubscribeTopics() {
  if (desiredTopics.size === 0)
    return
  sendJSON({
    type: 'subscribe',
    topics: Array.from(desiredTopics),
  })
}

function dispatchEvent(event: BusinessSocketEvent) {
  const targets = new Set<BusinessEventHandler>()
  const topic = event.topic || ''

  const pushHandlers = (key: string) => {
    const topicHandlers = handlers.get(key)
    if (!topicHandlers)
      return
    for (const handler of topicHandlers)
      targets.add(handler)
  }

  pushHandlers(topic)
  pushHandlers('*')

  if (topic) {
    const parts = topic.split('.')
    for (let index = 1; index < parts.length; index += 1)
      pushHandlers(`${parts.slice(0, index).join('.')}.*`)
  }

  for (const handler of targets)
    handler(event)
}

async function connect(token: string, path = '/ws/events') {
  authToken = token.trim()
  socketPath = path
  if (!authToken) {
    disconnect()
    return
  }

  if (socket.value?.readyState === WebSocket.OPEN) {
    connected.value = true
    status.value = 'connected'
    lastError.value = ''
    resubscribeTopics()
    return
  }

  if (connectingPromise)
    return connectingPromise

  manualClose = false
  clearReconnectTimer()
  status.value = status.value === 'reconnecting' ? 'reconnecting' : 'connecting'
  lastError.value = ''

  connectingPromise = new Promise<void>((resolve, reject) => {
    let settled = false
    const wsURL = buildWebSocketURL(path, authToken)
    socket.value = new WebSocket(wsURL.toString())

    socket.value.onopen = () => {
      connected.value = true
      status.value = 'connected'
      lastError.value = ''
      startHeartbeat()
      resubscribeTopics()
      settled = true
      resolve()
      connectingPromise = null
    }

    socket.value.onmessage = (event) => {
      lastMessageAt.value = nowISO()
      try {
        const parsed = JSON.parse(event.data) as BusinessSocketEvent
        if (parsed.type === 'pong')
          lastPongAt.value = parsed.timestamp || nowISO()
        dispatchEvent(parsed)
      }
      catch {
        dispatchEvent({ type: 'error', payload: { message: event.data } })
      }
    }

    socket.value.onerror = () => {
      connected.value = false
      stopHeartbeat()
      lastError.value = 'business websocket connection failed'
      status.value = manualClose ? 'idle' : 'error'
      connectingPromise = null
      if (!settled)
        reject(new Error('business websocket connection failed'))
    }

    socket.value.onclose = () => {
      connected.value = false
      stopHeartbeat()
      socket.value = null
      connectingPromise = null
      if (manualClose || !authToken) {
        status.value = 'idle'
        return
      }
      if (!settled)
        reject(new Error('business websocket connection closed before ready'))
      scheduleReconnect()
    }
  })

  return connectingPromise
}

function disconnect() {
  manualClose = true
  clearReconnectTimer()
  stopHeartbeat()
  socket.value?.close()
  socket.value = null
  connected.value = false
  status.value = 'idle'
  lastError.value = ''
  connectingPromise = null
}

function subscribe(topics: string[], handler: BusinessEventHandler, token?: string) {
  const normalizedTopics = Array.from(new Set(topics.map(topic => topic.trim()).filter(Boolean)))
  for (const topic of normalizedTopics) {
    desiredTopics.add(topic)
    const handlerSet = handlers.get(topic) || new Set<BusinessEventHandler>()
    handlerSet.add(handler)
    handlers.set(topic, handlerSet)
  }
  syncDesiredTopicCount()

  if (token)
    void connect(token).catch(() => undefined)
  else if (authToken)
    void connect(authToken).catch(() => undefined)

  if (connected.value && normalizedTopics.length > 0)
    sendJSON({ type: 'subscribe', topics: normalizedTopics })

  return () => {
    for (const topic of normalizedTopics) {
      const handlerSet = handlers.get(topic)
      if (!handlerSet)
        continue
      handlerSet.delete(handler)
      if (handlerSet.size === 0) {
        handlers.delete(topic)
        desiredTopics.delete(topic)
        if (connected.value)
          sendJSON({ type: 'unsubscribe', topics: [topic] })
      }
    }
    syncDesiredTopicCount()
  }
}

export function useBusinessSocket() {
  return {
    socket,
    connected,
    status,
    lastError,
    lastMessageAt,
    lastPongAt,
    subscribedTopicCount,
    connect,
    disconnect,
    subscribe,
    sendJSON,
  }
}

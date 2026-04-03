import { ref, shallowRef } from 'vue'

import type { StreamingMessage } from '@/types/asr'

const MAX_STREAM_MESSAGES = 500

function pushCapped(messages: StreamingMessage[], message: StreamingMessage) {
  messages.push(message)
  if (messages.length > MAX_STREAM_MESSAGES)
    messages.splice(0, messages.length - MAX_STREAM_MESSAGES)
}

export function useWebSocket(path = '/ws/transcribe') {
  const socket = shallowRef<WebSocket | null>(null)
  const connected = ref(false)
  const messages = ref<StreamingMessage[]>([])
  const totalMessages = ref(0)
  const manualClose = ref(false)
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null

  const connect = (token?: string) => new Promise<void>((resolve, reject) => {
    manualClose.value = false
    socket.value?.close()

    const wsURL = new URL(`${import.meta.env.VITE_WS_BASE_URL}${path}`)
    if (token)
      wsURL.searchParams.set('token', token)

    socket.value = new WebSocket(wsURL.toString())

    socket.value.onopen = () => {
      connected.value = true
      resolve()
    }

    socket.value.onmessage = (event: MessageEvent<string>) => {
      try {
        totalMessages.value += 1
        pushCapped(messages.value, JSON.parse(event.data) as StreamingMessage)
      }
      catch {
        totalMessages.value += 1
        pushCapped(messages.value, {
          type: 'sentence',
          text: event.data,
          is_final: true,
          sequence: messages.value.length + 1,
        })
      }
    }

    socket.value.onerror = () => {
      reject(new Error('websocket connection failed'))
    }

    socket.value.onclose = () => {
      connected.value = false
      if (!manualClose.value)
        reconnectTimer = setTimeout(() => connect(token).catch(() => undefined), 1500)
    }
  })

  const send = (payload: string | Blob | ArrayBufferLike | ArrayBufferView) => {
    if (socket.value?.readyState === WebSocket.OPEN)
      socket.value.send(payload)
  }

  const sendJSON = (payload: Record<string, unknown>) => {
    send(JSON.stringify(payload))
  }

  const disconnect = () => {
    manualClose.value = true
    if (reconnectTimer)
      clearTimeout(reconnectTimer)
    socket.value?.close()
    connected.value = false
  }

  return { socket, connected, messages, totalMessages, connect, send, sendJSON, disconnect }
}
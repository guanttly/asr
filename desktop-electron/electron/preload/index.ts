import { contextBridge, ipcRenderer } from 'electron'

const subscriptions = new Map<string, Set<(payload: unknown) => void>>()

ipcRenderer.on('asr-event', (_event, payload: { event: string, data: unknown }) => {
  const handlers = subscriptions.get(payload.event)
  if (!handlers || handlers.size === 0)
    return
  for (const handler of handlers) {
    try {
      handler(payload.data)
    }
    catch (err) {
      console.error('[bridge] handler threw', err)
    }
  }
})

const windowLabel = (() => {
  const search = new URLSearchParams(window.location.search)
  return search.get('label') || 'main'
})()

contextBridge.exposeInMainWorld('__electronBridge__', {
  windowLabel,
  invoke: (channel: string, args?: unknown) => ipcRenderer.invoke('asr:invoke', { channel, args, windowLabel }),
  listen: (event: string, handler: (payload: unknown) => void) => {
    let bucket = subscriptions.get(event)
    if (!bucket) {
      bucket = new Set()
      subscriptions.set(event, bucket)
    }
    bucket.add(handler)
    return () => {
      bucket?.delete(handler)
      if (bucket && bucket.size === 0)
        subscriptions.delete(event)
    }
  },
})

// Vue 端的 App.vue 通过 window.__ASR_WINDOW__ 兜底判断 settings 窗口
;(window as unknown as { __ASR_WINDOW__: string }).__ASR_WINDOW__ = windowLabel

// `@tauri-apps/api/event` shim，仅复刻 desktop/src 中实际用到的 listen()。
import { invoke } from './tauri-core'

export interface EventCallback<T> {
  (event: { event: string, payload: T, id: number }): void
}

export type UnlistenFn = () => void

let nextEventId = 1

export async function listen<T = unknown>(eventName: string, handler: EventCallback<T>): Promise<UnlistenFn> {
  if (!window.__electronBridge__) {
    throw new Error(`electron bridge not ready when listening to "${eventName}"`)
  }
  const id = nextEventId++
  const unlisten = window.__electronBridge__.listen<T>(eventName, (payload) => {
    handler({ event: eventName, payload, id })
  })
  // 让主进程感知该窗口订阅了哪些事件（如热键事件需要根据焦点判断）
  void invoke('event:subscribe', { event: eventName }).catch(() => undefined)
  return unlisten
}

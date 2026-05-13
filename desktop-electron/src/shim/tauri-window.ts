// `@tauri-apps/api/window` shim，仅复刻 desktop/src 用到的方法集合。
import { invoke } from './tauri-core'

export interface WindowApi {
  label: string
  startDragging: () => Promise<void>
  stopDragging: () => Promise<void>
  minimize: () => Promise<void>
  unminimize: () => Promise<void>
  close: () => Promise<void>
  hide: () => Promise<void>
  show: () => Promise<void>
  setFocus: () => Promise<void>
  isVisible: () => Promise<boolean>
  setAlwaysOnTop: (value: boolean) => Promise<void>
  setSize: (width: number, height: number) => Promise<void>
}

let cached: WindowApi | null = null

export function getCurrentWindow(): WindowApi {
  if (cached)
    return cached

  const label = window.__electronBridge__?.windowLabel
    ?? (window as { __ASR_WINDOW__?: string }).__ASR_WINDOW__
    ?? 'main'

  const callWin = (action: string, payload?: Record<string, unknown>) =>
    invoke<void>('window:action', { action, ...(payload ?? {}) })

  cached = {
    label,
    startDragging: () => callWin('startDragging'),
    stopDragging: () => callWin('stopDragging'),
    minimize: () => callWin('minimize'),
    unminimize: () => callWin('unminimize'),
    close: () => callWin('close'),
    hide: () => callWin('hide'),
    show: () => callWin('show'),
    setFocus: () => callWin('setFocus'),
    isVisible: () => invoke<boolean>('window:action', { action: 'isVisible' }),
    setAlwaysOnTop: (value: boolean) => callWin('setAlwaysOnTop', { value }),
    setSize: (width: number, height: number) => callWin('setSize', { width, height }),
  }
  return cached
}

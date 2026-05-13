// Tauri 2 `@tauri-apps/api/core` -> Electron IPC 桥
// preload 已通过 contextBridge 把 invoke 挂到 window.__electronBridge__ 上。

export async function invoke<T = unknown>(cmd: string, args?: Record<string, unknown>): Promise<T> {
  if (!window.__electronBridge__) {
    throw new Error(`electron bridge not ready when invoking "${cmd}"`)
  }
  return window.__electronBridge__.invoke<T>(cmd, args ?? {})
}

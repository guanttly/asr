/// <reference types="vite/client" />

declare const __APP_VERSION__: string
declare const __APP_BUILD_CODE__: string
declare const __APP_BUILD_DATE__: string

interface ElectronBridgeApi {
  invoke: <T = unknown>(channel: string, args?: unknown) => Promise<T>
  listen: <T = unknown>(event: string, handler: (payload: T) => void) => () => void
  windowLabel: string
}

interface Window {
  __electronBridge__: ElectronBridgeApi
  __ASR_WINDOW__?: string
}

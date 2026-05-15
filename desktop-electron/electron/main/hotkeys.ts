import { app, globalShortcut, BrowserWindow } from 'electron'

export interface HotkeyModifiersPayload {
  ctrl: boolean
  alt: boolean
  shift: boolean
  meta: boolean
}

export interface HotkeyTriggerPayload {
  type: 'keyboard' | 'mouse'
  code?: string
  button?: string
}

export interface HotkeyBindingPayload {
  action: string
  enabled: boolean
  modifiers: HotkeyModifiersPayload
  trigger: HotkeyTriggerPayload
}

export interface HotkeyConfigureResult {
  supported: boolean
  registered: number
  message: string
}

const HOTKEY_EVENT = 'desktop-hotkey-action'

let emitter: ((action: string) => boolean | void) | null = null

export function setHotkeyEmitter(fn: (action: string) => boolean | void) {
  emitter = fn
}

function emitAction(action: string) {
  if (emitter) {
    try {
      if (emitter(action) === true)
        return
    }
    catch (err) {
      console.warn('[hotkeys] action emitter failed', action, err)
    }
  }

  // 兜底：广播给所有可见窗口（与 Tauri Emitter 行为一致）
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed())
      win.webContents.send('asr-event', { event: HOTKEY_EVENT, data: action })
  }
}

function modifiersToAccelerator(modifiers: HotkeyModifiersPayload): string[] {
  const parts: string[] = []
  if (modifiers.ctrl)
    parts.push('Control')
  if (modifiers.alt)
    parts.push('Alt')
  if (modifiers.shift)
    parts.push('Shift')
  if (modifiers.meta)
    parts.push('Super')
  return parts
}

// KeyboardEvent.code -> Electron Accelerator key name
const KEY_CODE_MAP: Record<string, string> = {
  Space: 'Space',
  Enter: 'Return',
  Escape: 'Escape',
  Tab: 'Tab',
  Backspace: 'Backspace',
  Delete: 'Delete',
  Insert: 'Insert',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  Comma: ',',
  Period: '.',
  Slash: '/',
  Backslash: '\\',
  Semicolon: ';',
  Quote: '\'',
  BracketLeft: '[',
  BracketRight: ']',
  Backquote: '`',
  Minus: '-',
  Equal: '=',
}

function codeToKey(code: string): string | null {
  if (KEY_CODE_MAP[code])
    return KEY_CODE_MAP[code]
  if (/^Key[A-Z]$/.test(code))
    return code.slice(3)
  if (/^Digit[0-9]$/.test(code))
    return code.slice(5)
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(code))
    return code
  if (/^Numpad[0-9]$/.test(code))
    return `num${code.slice(6)}`
  if (code === 'NumpadEnter')
    return 'Return'
  return null
}

function buildAccelerator(binding: HotkeyBindingPayload): string | null {
  if (binding.trigger.type !== 'keyboard' || !binding.trigger.code)
    return null

  const key = codeToKey(binding.trigger.code)
  if (!key)
    return null

  const parts = modifiersToAccelerator(binding.modifiers)
  parts.push(key)
  return parts.join('+')
}

export function configureHotkeys(bindings: HotkeyBindingPayload[]): HotkeyConfigureResult {
  if (!app.isReady()) {
    return {
      supported: false,
      registered: 0,
      message: '应用尚未就绪，无法注册全局热键',
    }
  }

  globalShortcut.unregisterAll()

  const messages: string[] = []
  let registered = 0
  let mouseSkipped = 0

  for (const binding of bindings) {
    if (!binding.enabled)
      continue

    if (binding.trigger.type === 'mouse') {
      mouseSkipped++
      continue
    }

    const accel = buildAccelerator(binding)
    if (!accel) {
      messages.push(`无法解析 ${binding.action} 的按键组合`)
      continue
    }

    try {
      const ok = globalShortcut.register(accel, () => emitAction(binding.action))
      if (ok)
        registered++
      else
        messages.push(`系统拒绝注册 ${accel}（可能被其他应用占用）`)
    }
    catch (err) {
      messages.push(`注册 ${accel} 失败: ${(err as Error).message}`)
    }
  }

  if (mouseSkipped > 0)
    messages.push(`已忽略 ${mouseSkipped} 个鼠标按键热键（Win7 Electron 端不支持鼠标全局热键）`)

  return {
    supported: true,
    registered,
    message: messages.join('；') || `已注册 ${registered} 个全局热键`,
  }
}

export function disposeHotkeys() {
  globalShortcut.unregisterAll()
}

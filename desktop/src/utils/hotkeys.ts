export const HOTKEY_ACTIONS = {
  TOGGLE_SETTINGS_WINDOW: 'toggleSettingsWindow',
  TOGGLE_FLOATING_WINDOW: 'toggleFloatingWindow',
  TOGGLE_VOICE_COMMAND_MODE: 'toggleVoiceCommandMode',
  TOGGLE_RECORDING: 'toggleRecording',
  CYCLE_SCENE_MODE: 'cycleSceneMode',
  ACTIVATE_REPORT_MODE: 'activateReportMode',
  ACTIVATE_MEETING_MODE: 'activateMeetingMode',
} as const

export type HotkeyActionId = typeof HOTKEY_ACTIONS[keyof typeof HOTKEY_ACTIONS]

export const HOTKEY_MOUSE_BUTTONS = {
  BACK: 'mouse4',
  FORWARD: 'mouse5',
} as const

export type HotkeyMouseButton = typeof HOTKEY_MOUSE_BUTTONS[keyof typeof HOTKEY_MOUSE_BUTTONS]

export interface HotkeyModifiers {
  ctrl: boolean
  alt: boolean
  shift: boolean
  meta: boolean
}

export interface KeyboardHotkeyTrigger {
  type: 'keyboard'
  code: string
}

export interface MouseHotkeyTrigger {
  type: 'mouse'
  button: HotkeyMouseButton
}

export type HotkeyTrigger = KeyboardHotkeyTrigger | MouseHotkeyTrigger

export interface HotkeyBinding {
  enabled: boolean
  modifiers: HotkeyModifiers
  trigger: HotkeyTrigger | null
}

export type HotkeyBindings = Record<HotkeyActionId, HotkeyBinding>

export interface HotkeyActionDefinition {
  id: HotkeyActionId
  title: string
  description: string
  optional?: boolean
}

export interface HotkeyBackendBindingPayload {
  action: HotkeyActionId
  enabled: boolean
  modifiers: HotkeyModifiers
  trigger: HotkeyTrigger
}

function createEmptyModifiers(): HotkeyModifiers {
  return {
    ctrl: false,
    alt: false,
    shift: false,
    meta: false,
  }
}

function createBinding(trigger: HotkeyTrigger | null, modifiers: Partial<HotkeyModifiers> = {}, enabled = true): HotkeyBinding {
  return {
    enabled: enabled && trigger !== null,
    modifiers: {
      ...createEmptyModifiers(),
      ...modifiers,
    },
    trigger,
  }
}

export const HOTKEY_ACTION_DEFINITIONS: HotkeyActionDefinition[] = [
  {
    id: HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW,
    title: '显示或隐藏设置页',
    description: '快速打开设置页处理登录、服务地址和调试信息。',
  },
  {
    id: HOTKEY_ACTIONS.TOGGLE_FLOATING_WINDOW,
    title: '显示或隐藏悬浮球',
    description: '不打断当前应用焦点地呼出或收起悬浮球。',
  },
  {
    id: HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE,
    title: '开启或关闭指令识别',
    description: '可在未录音时直接进入指令模式，必要时自动开启录音。',
  },
  {
    id: HOTKEY_ACTIONS.TOGGLE_RECORDING,
    title: '开启或关闭录音',
    description: '延续当前默认场景开始或结束录音。',
  },
  {
    id: HOTKEY_ACTIONS.CYCLE_SCENE_MODE,
    title: '切换场景并激活',
    description: '在报告模式与会议模式之间切换，并立即进入对应模式。',
  },
  {
    id: HOTKEY_ACTIONS.ACTIVATE_REPORT_MODE,
    title: '报告模式并激活',
    description: '补充场景：直接切到报告模式，不再需要先循环场景。',
    optional: true,
  },
  {
    id: HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE,
    title: '会议模式并激活',
    description: '补充场景：直接切到会议模式，适合一键进入会议纪要流。',
    optional: true,
  },
]

export function createDefaultHotkeyBindings(): HotkeyBindings {
  return {
    [HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW]: createBinding({ type: 'keyboard', code: 'KeyS' }, { alt: true, shift: true }),
    [HOTKEY_ACTIONS.TOGGLE_FLOATING_WINDOW]: createBinding({ type: 'keyboard', code: 'KeyF' }, { alt: true, shift: true }),
    [HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE]: createBinding({ type: 'keyboard', code: 'KeyV' }, { alt: true, shift: true }),
    [HOTKEY_ACTIONS.TOGGLE_RECORDING]: createBinding({ type: 'keyboard', code: 'Space' }, { ctrl: true, shift: true }),
    [HOTKEY_ACTIONS.CYCLE_SCENE_MODE]: createBinding({ type: 'keyboard', code: 'KeyM' }, { alt: true, shift: true }),
    [HOTKEY_ACTIONS.ACTIVATE_REPORT_MODE]: createBinding(null, {}, false),
    [HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE]: createBinding(null, {}, false),
  }
}

export function cloneHotkeyBinding(binding?: HotkeyBinding | null): HotkeyBinding {
  return normalizeHotkeyBinding(binding)
}

export function cloneHotkeyBindings(bindings?: Partial<HotkeyBindings> | null): HotkeyBindings {
  return normalizeHotkeyBindings(bindings)
}

function normalizeKeyboardCode(code?: string | null) {
  const value = (code || '').trim()
  return value || null
}

function normalizeMouseButton(button?: string | null): HotkeyMouseButton | null {
  if (button === HOTKEY_MOUSE_BUTTONS.BACK)
    return HOTKEY_MOUSE_BUTTONS.BACK
  if (button === HOTKEY_MOUSE_BUTTONS.FORWARD)
    return HOTKEY_MOUSE_BUTTONS.FORWARD
  return null
}

export function normalizeHotkeyBinding(binding?: Partial<HotkeyBinding> | null): HotkeyBinding {
  const modifiers: HotkeyModifiers = {
    ctrl: binding?.modifiers?.ctrl === true,
    alt: binding?.modifiers?.alt === true,
    shift: binding?.modifiers?.shift === true,
    meta: binding?.modifiers?.meta === true,
  }

  if (binding?.trigger?.type === 'keyboard') {
    const code = normalizeKeyboardCode(binding.trigger.code)
    return createBinding(code ? { type: 'keyboard', code } : null, modifiers, binding?.enabled === true)
  }

  if (binding?.trigger?.type === 'mouse') {
    const button = normalizeMouseButton(binding.trigger.button)
    return createBinding(button ? { type: 'mouse', button } : null, modifiers, binding?.enabled === true)
  }

  return createBinding(null, modifiers, false)
}

export function normalizeHotkeyBindings(bindings?: Partial<HotkeyBindings> | null): HotkeyBindings {
  const defaults = createDefaultHotkeyBindings()
  return {
    [HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW] ?? defaults[HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW]),
    [HOTKEY_ACTIONS.TOGGLE_FLOATING_WINDOW]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.TOGGLE_FLOATING_WINDOW] ?? defaults[HOTKEY_ACTIONS.TOGGLE_FLOATING_WINDOW]),
    [HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE] ?? defaults[HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE]),
    [HOTKEY_ACTIONS.TOGGLE_RECORDING]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.TOGGLE_RECORDING] ?? defaults[HOTKEY_ACTIONS.TOGGLE_RECORDING]),
    [HOTKEY_ACTIONS.CYCLE_SCENE_MODE]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.CYCLE_SCENE_MODE] ?? defaults[HOTKEY_ACTIONS.CYCLE_SCENE_MODE]),
    [HOTKEY_ACTIONS.ACTIVATE_REPORT_MODE]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.ACTIVATE_REPORT_MODE] ?? defaults[HOTKEY_ACTIONS.ACTIVATE_REPORT_MODE]),
    [HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE]: normalizeHotkeyBinding(bindings?.[HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE] ?? defaults[HOTKEY_ACTIONS.ACTIVATE_MEETING_MODE]),
  }
}

export function replaceHotkeyBindings(target: HotkeyBindings, source?: Partial<HotkeyBindings> | null) {
  const normalized = normalizeHotkeyBindings(source)
  for (const action of Object.values(HOTKEY_ACTIONS)) {
    target[action] = normalized[action]
  }
}

export function serializeHotkeyBindings(bindings?: Partial<HotkeyBindings> | null) {
  return JSON.stringify(normalizeHotkeyBindings(bindings))
}

const keyboardCodeLabelMap: Record<string, string> = {
  Space: 'Space',
  Escape: 'Esc',
  Backspace: 'Backspace',
  Enter: 'Enter',
  Tab: 'Tab',
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  Minus: '-',
  Equal: '=',
  BracketLeft: '[',
  BracketRight: ']',
  Semicolon: ';',
  Quote: '\'',
  Comma: ',',
  Period: '.',
  Slash: '/',
  Backslash: '\\',
  Backquote: '`',
  Insert: 'Insert',
  Delete: 'Delete',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
}

function formatKeyboardCode(code: string) {
  if (keyboardCodeLabelMap[code])
    return keyboardCodeLabelMap[code]
  if (/^Key[A-Z]$/.test(code))
    return code.slice(3)
  if (/^Digit[0-9]$/.test(code))
    return code.slice(5)
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(code))
    return code
  return code
}

function formatMouseButton(button: HotkeyMouseButton) {
  return button === HOTKEY_MOUSE_BUTTONS.BACK ? 'Mouse4' : 'Mouse5'
}

export function formatHotkeyBinding(binding?: Partial<HotkeyBinding> | null) {
  const normalized = normalizeHotkeyBinding(binding)
  if (!normalized.enabled || !normalized.trigger)
    return '未设置'

  const parts: string[] = []
  if (normalized.modifiers.ctrl)
    parts.push('Ctrl')
  if (normalized.modifiers.alt)
    parts.push('Alt')
  if (normalized.modifiers.shift)
    parts.push('Shift')
  if (normalized.modifiers.meta)
    parts.push('Win')

  if (normalized.trigger.type === 'keyboard')
    parts.push(formatKeyboardCode(normalized.trigger.code))
  else
    parts.push(formatMouseButton(normalized.trigger.button))

  return parts.join('+')
}

export function getHotkeyBindingSignature(binding?: Partial<HotkeyBinding> | null) {
  const normalized = normalizeHotkeyBinding(binding)
  if (!normalized.enabled || !normalized.trigger)
    return ''
  return formatHotkeyBinding(normalized).toLowerCase()
}

export function findConflictingHotkeyAction(bindings: Partial<HotkeyBindings> | null | undefined, candidate: Partial<HotkeyBinding> | null, excludeAction?: HotkeyActionId) {
  const signature = getHotkeyBindingSignature(candidate)
  if (!signature)
    return null

  const normalized = normalizeHotkeyBindings(bindings)
  for (const action of Object.values(HOTKEY_ACTIONS)) {
    if (action === excludeAction)
      continue
    if (getHotkeyBindingSignature(normalized[action]) === signature)
      return action
  }
  return null
}

export function isHotkeyActionId(value: string): value is HotkeyActionId {
  return Object.values(HOTKEY_ACTIONS).includes(value as HotkeyActionId)
}

export function toBackendHotkeyBindings(bindings?: Partial<HotkeyBindings> | null): HotkeyBackendBindingPayload[] {
  const normalized = normalizeHotkeyBindings(bindings)
  const payloads: HotkeyBackendBindingPayload[] = []
  for (const action of Object.values(HOTKEY_ACTIONS)) {
    const binding = normalized[action]
    if (!binding.enabled || !binding.trigger)
      continue

    payloads.push({
      action,
      enabled: true,
      modifiers: { ...binding.modifiers },
      trigger: binding.trigger.type === 'keyboard'
        ? { type: 'keyboard', code: binding.trigger.code }
        : { type: 'mouse', button: binding.trigger.button },
    })
  }
  return payloads
}
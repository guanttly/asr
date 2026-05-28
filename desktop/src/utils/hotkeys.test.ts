import { describe, expect, it } from 'vitest'

import {
  HOTKEY_ACTIONS,
  createDefaultHotkeyBindings,
  findConflictingHotkeyAction,
  formatHotkeyBinding,
  formatHotkeySyncFailureMessage,
  normalizeHotkeyBinding,
  toBackendHotkeyBindings,
} from './hotkeys'

describe('desktop hotkey utilities', () => {
  it('keeps the documented default global shortcuts stable', () => {
    const defaults = createDefaultHotkeyBindings()
    expect(formatHotkeyBinding(defaults[HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW])).toBe('Alt+Shift+S')
    expect(formatHotkeyBinding(defaults[HOTKEY_ACTIONS.TOGGLE_FLOATING_WINDOW])).toBe('Alt+Shift+F')
    expect(formatHotkeyBinding(defaults[HOTKEY_ACTIONS.TOGGLE_VOICE_COMMAND_MODE])).toBe('Alt+Shift+V')
    expect(formatHotkeyBinding(defaults[HOTKEY_ACTIONS.TOGGLE_RECORDING])).toBe('Ctrl+Shift+Space')
    expect(formatHotkeyBinding(defaults[HOTKEY_ACTIONS.CYCLE_SCENE_MODE])).toBe('Alt+Shift+M')
  })

  it('detects duplicate shortcut combinations before saving', () => {
    const defaults = createDefaultHotkeyBindings()
    const conflict = findConflictingHotkeyAction(
      defaults,
      defaults[HOTKEY_ACTIONS.TOGGLE_RECORDING],
      HOTKEY_ACTIONS.TOGGLE_SETTINGS_WINDOW,
    )
    expect(conflict).toBe(HOTKEY_ACTIONS.TOGGLE_RECORDING)
  })

  it('clears bindings and excludes disabled actions from the native payload', () => {
    const cleared = normalizeHotkeyBinding(null)
    expect(formatHotkeyBinding(cleared)).toBe('未设置')

    const bindings = createDefaultHotkeyBindings()
    bindings[HOTKEY_ACTIONS.TOGGLE_RECORDING] = cleared
    expect(toBackendHotkeyBindings(bindings).map(item => item.action)).not.toContain(HOTKEY_ACTIONS.TOGGLE_RECORDING)
  })

  it('tells users local config is retained when sync fails', () => {
    const message = formatHotkeySyncFailureMessage(new Error('原生热键注册失败'))
    expect(message).toContain('已保留本地热键配置')
    expect(message).toContain('重新同步')
  })
})
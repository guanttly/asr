import type { HotplugDeviceChangeState } from './hotplugRecovery'

import { afterEach, describe, expect, it, vi } from 'vitest'
import {
  createRecoveryController,
  decideHotplugAction,
  DEFAULT_RECOVERY_DELAYS_MS,
  nextRecoveryDelayMs,
} from './hotplugRecovery'

function state(overrides: Partial<HotplugDeviceChangeState> = {}): HotplugDeviceChangeState {
  return {
    recording: true,
    stoppedByUser: false,
    restoring: false,
    alreadyLost: false,
    hasInputDevice: true,
    trackHealthy: true,
    ...overrides,
  }
}

describe('decideHotplugAction', () => {
  it('ignores device changes when not actively capturing', () => {
    expect(decideHotplugAction(state({ recording: false }))).toBe('ignore')
    expect(decideHotplugAction(state({ stoppedByUser: true }))).toBe('ignore')
    expect(decideHotplugAction(state({ restoring: true }))).toBe('ignore')
  })

  it('marks the device as lost when no audio input remains', () => {
    expect(decideHotplugAction(state({ hasInputDevice: false }))).toBe('mark-lost')
  })

  it('attempts recovery when a device returns after being lost', () => {
    expect(decideHotplugAction(state({ alreadyLost: true }))).toBe('attempt-recover')
  })

  it('marks lost when the current track went unhealthy (muted but not ended)', () => {
    expect(decideHotplugAction(state({ trackHealthy: false }))).toBe('mark-lost')
  })

  it('ignores benign device changes while the capture track stays healthy', () => {
    expect(decideHotplugAction(state())).toBe('ignore')
  })
})

describe('nextRecoveryDelayMs', () => {
  it('follows the backoff sequence and caps at the final entry', () => {
    expect(nextRecoveryDelayMs(0, [10, 20, 40])).toBe(10)
    expect(nextRecoveryDelayMs(1, [10, 20, 40])).toBe(20)
    expect(nextRecoveryDelayMs(2, [10, 20, 40])).toBe(40)
    expect(nextRecoveryDelayMs(99, [10, 20, 40])).toBe(40)
  })

  it('falls back to the default delays', () => {
    expect(nextRecoveryDelayMs(0)).toBe(DEFAULT_RECOVERY_DELAYS_MS[0])
  })
})

describe('createRecoveryController', () => {
  afterEach(() => {
    vi.useRealTimers()
  })

  it('keeps retrying with backoff until recovery succeeds (no permanent stall)', async () => {
    vi.useFakeTimers()
    let attempts = 0
    const onRecovered = vi.fn()
    const controller = createRecoveryController({
      attempt: async () => {
        attempts += 1
        return attempts >= 3
      },
      isActive: () => true,
      onRecovered,
      delaysMs: [10, 20, 40],
    })

    controller.kick()
    await vi.advanceTimersByTimeAsync(200)

    expect(attempts).toBe(3)
    expect(onRecovered).toHaveBeenCalledTimes(1)
  })

  it('stops retrying once it is no longer active', async () => {
    vi.useFakeTimers()
    let attempts = 0
    let active = true
    const controller = createRecoveryController({
      attempt: async () => {
        attempts += 1
        return false
      },
      isActive: () => active,
      delaysMs: [10],
    })

    controller.kick()
    await vi.advanceTimersByTimeAsync(35)
    active = false
    const settled = attempts
    await vi.advanceTimersByTimeAsync(200)

    expect(attempts).toBe(settled)
  })

  it('cancel() stops all further retries and never reports recovery', async () => {
    vi.useFakeTimers()
    let attempts = 0
    const onRecovered = vi.fn()
    const controller = createRecoveryController({
      attempt: async () => {
        attempts += 1
        return false
      },
      isActive: () => true,
      onRecovered,
      delaysMs: [10],
    })

    controller.kick()
    await vi.advanceTimersByTimeAsync(15)
    controller.cancel()
    const settled = attempts
    await vi.advanceTimersByTimeAsync(200)

    expect(attempts).toBe(settled)
    expect(onRecovered).not.toHaveBeenCalled()
  })
})

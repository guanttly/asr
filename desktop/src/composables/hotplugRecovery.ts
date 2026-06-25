// 麦克风热插拔恢复的纯逻辑：与浏览器音频 API 解耦，便于在 node 环境下做确定性单测。

export type HotplugAction = 'ignore' | 'mark-lost' | 'attempt-recover'

export interface HotplugDeviceChangeState {
  /** 当前是否处于录音中。 */
  recording: boolean
  /** 用户是否已主动停止（停止后不应再恢复）。 */
  stoppedByUser: boolean
  /** 是否正在重建采集管道（重建期间忽略并发的设备变更）。 */
  restoring: boolean
  /** 是否已处于「设备丢失」状态。 */
  alreadyLost: boolean
  /** 系统是否仍存在可用的音频输入设备。 */
  hasInputDevice: boolean
  /** 当前采集 track 是否健康（live 且未被静音）。 */
  trackHealthy: boolean
}

/**
 * 依据一次 `devicechange` 事件时的状态，决定录音器应采取的动作。
 * - `ignore`：无需处理。
 * - `mark-lost`：判定采集已中断，进入等待重连状态。
 * - `attempt-recover`：尝试重建采集管道。
 */
export function decideHotplugAction(state: HotplugDeviceChangeState): HotplugAction {
  if (!state.recording || state.stoppedByUser || state.restoring)
    return 'ignore'
  if (!state.hasInputDevice)
    return 'mark-lost'
  if (state.alreadyLost)
    return 'attempt-recover'
  if (!state.trackHealthy)
    return 'mark-lost'
  return 'ignore'
}

/** 默认的重连退避序列（毫秒）。最后一项作为封顶值持续重试。 */
export const DEFAULT_RECOVERY_DELAYS_MS = [400, 800, 1600, 3000]

/** 计算第 `attempt` 次重连前应等待的毫秒数（带封顶）。 */
export function nextRecoveryDelayMs(attempt: number, delays: number[] = DEFAULT_RECOVERY_DELAYS_MS): number {
  if (delays.length === 0)
    return 0
  const index = Math.max(0, Math.min(attempt, delays.length - 1))
  return delays[index]
}

export interface RecoveryControllerDeps {
  /** 执行一次重连尝试；返回 true 表示已恢复，false 表示需继续重试。 */
  attempt: () => Promise<boolean>
  /** 是否仍需要继续恢复（录音中、未被用户停止、仍处于丢失态）。 */
  isActive: () => boolean
  /** 恢复成功后的回调。 */
  onRecovered?: () => void
  /** 自定义退避序列。 */
  delaysMs?: number[]
  /** 可注入的定时器（便于测试）。 */
  setTimer?: (handler: () => void, ms: number) => ReturnType<typeof setTimeout>
  clearTimer?: (handle: ReturnType<typeof setTimeout>) => void
}

export interface RecoveryController {
  /** 立即触发一次重连（复位退避），用于 devicechange / unmute 等事件加速恢复。 */
  kick: () => void
  /** 取消并停止后续所有重连。 */
  cancel: () => void
}

/**
 * 创建一个带退避重试的麦克风重连控制器。即便不再有新的设备事件，也会按退避
 * 序列持续重试，直到恢复成功或 `isActive()` 变为 false，从而避免「首次重连失败后
 * 永久卡死」的问题。
 */
export function createRecoveryController(deps: RecoveryControllerDeps): RecoveryController {
  const delays = deps.delaysMs ?? DEFAULT_RECOVERY_DELAYS_MS
  const setTimer = deps.setTimer ?? ((handler, ms) => setTimeout(handler, ms))
  const clearTimer = deps.clearTimer ?? (handle => clearTimeout(handle))

  let timer: ReturnType<typeof setTimeout> | null = null
  let attemptCount = 0
  let inFlight = false
  let cancelled = false

  const clearPending = () => {
    if (timer != null) {
      clearTimer(timer)
      timer = null
    }
  }

  const schedule = () => {
    clearPending()
    const ms = nextRecoveryDelayMs(attemptCount, delays)
    attemptCount += 1
    timer = setTimer(() => {
      timer = null
      void run()
    }, ms)
  }

  async function run() {
    if (cancelled || inFlight)
      return
    if (!deps.isActive()) {
      clearPending()
      return
    }
    inFlight = true
    let recovered = false
    try {
      recovered = await deps.attempt()
    }
    catch {
      recovered = false
    }
    finally {
      inFlight = false
    }
    if (cancelled)
      return
    if (recovered) {
      clearPending()
      attemptCount = 0
      deps.onRecovered?.()
      return
    }
    if (!deps.isActive()) {
      clearPending()
      return
    }
    schedule()
  }

  return {
    kick() {
      if (cancelled)
        return
      attemptCount = 0
      clearPending()
      void run()
    },
    cancel() {
      cancelled = true
      clearPending()
    },
  }
}

// 跨窗口「正在录制的会议」状态与控制信道。
//
// 录音发生在主窗口（RecorderWindow / MicButton），而会议列表与删除按钮在 settings
// 窗口（MeetingsList）。两个窗口是各自独立的 Pinia 实例，`isRecording` 不跨窗口同步。
// 本模块复用 settings 同步已验证可靠的「localStorage + storage 事件」机制：
//   - 主窗口在会议被服务端「转正」后写入当前录制会议的 meetingId；停止/重置时清除。
//   - settings 窗口在删除时读取该标记，判断被删的是否正是「正在录制」的会议。
//   - settings 写入「停止请求」，主窗口通过 storage 事件接收并停止录音 + 丢弃上传
//     （= 删除会议 + 清理临时文件），再写回「停止结果」，settings 通过 storage 事件等待。
//
// storage 事件只在「其它窗口」触发，天然避免自触发；用 nonce 保证重复请求/结果也能触发。

const ACTIVE_KEY = 'asr-desktop-active-meeting'
const DISCARD_REQUEST_KEY = 'asr-desktop-meeting-discard-request'
const DISCARD_RESULT_KEY = 'asr-desktop-meeting-discard-result'

export interface ActiveMeetingInfo {
  meetingId: number
  uploadId?: string
}

interface DiscardRequest {
  meetingId: number
  nonce: string
}

interface DiscardResult {
  meetingId: number
  nonce: string
  ok: boolean
}

function newNonce(): string {
  return `${Date.now()}-${Math.random().toString(36).slice(2)}`
}

// publishActiveMeeting 由主窗口调用：记录/清除当前正在录制的会议（null 表示无）。
export function publishActiveMeeting(info: ActiveMeetingInfo | null): void {
  try {
    if (!info || !Number.isFinite(info.meetingId)) {
      globalThis.localStorage?.removeItem(ACTIVE_KEY)
      return
    }
    globalThis.localStorage?.setItem(ACTIVE_KEY, JSON.stringify({
      meetingId: info.meetingId,
      uploadId: info.uploadId,
    }))
  }
  catch {
    // localStorage 不可用时忽略：删除时会退化为普通删除流程。
  }
}

// readActiveMeeting 由 settings 窗口调用：读取当前正在录制的会议（同源 localStorage 共享）。
export function readActiveMeeting(): ActiveMeetingInfo | null {
  try {
    const raw = globalThis.localStorage?.getItem(ACTIVE_KEY)
    if (!raw)
      return null
    const parsed = JSON.parse(raw) as Partial<ActiveMeetingInfo>
    if (parsed && typeof parsed.meetingId === 'number' && Number.isFinite(parsed.meetingId)) {
      return {
        meetingId: parsed.meetingId,
        uploadId: typeof parsed.uploadId === 'string' ? parsed.uploadId : undefined,
      }
    }
    return null
  }
  catch {
    return null
  }
}

// requestActiveMeetingDiscard 由 settings 窗口调用：请求主窗口停止正在录制的会议并丢弃
// 上传（删除会议 + 清理临时文件）。返回主窗口是否成功处理；超时（主窗口不在/无响应）返回
// false，调用方据此回退到直接删除会议记录。
export function requestActiveMeetingDiscard(meetingId: number, timeoutMs = 10_000): Promise<boolean> {
  return new Promise<boolean>((resolve) => {
    if (typeof window === 'undefined' || !globalThis.localStorage) {
      resolve(false)
      return
    }
    const nonce = newNonce()
    let settled = false
    const onStorage = (event: StorageEvent) => {
      if (event.key !== DISCARD_RESULT_KEY || !event.newValue)
        return
      try {
        const parsed = JSON.parse(event.newValue) as DiscardResult
        if (parsed?.nonce === nonce && parsed?.meetingId === meetingId)
          finish(Boolean(parsed.ok))
      }
      catch {
        // 忽略无法解析的结果。
      }
    }
    const timer = setTimeout(() => finish(false), timeoutMs)
    function finish(ok: boolean) {
      if (settled)
        return
      settled = true
      clearTimeout(timer)
      window.removeEventListener('storage', onStorage)
      resolve(ok)
    }
    window.addEventListener('storage', onStorage)
    try {
      globalThis.localStorage.setItem(DISCARD_REQUEST_KEY, JSON.stringify({ meetingId, nonce } satisfies DiscardRequest))
    }
    catch {
      finish(false)
    }
  })
}

// listenForMeetingDiscardRequest 由主窗口调用：监听 settings 的停止请求，交给 handler 处理
// （停止录音 + 丢弃上传），再把处理结果写回供 settings 等待。返回取消监听的函数。
export function listenForMeetingDiscardRequest(
  handler: (meetingId: number) => Promise<boolean> | boolean,
): () => void {
  if (typeof window === 'undefined')
    return () => {}
  const onStorage = (event: StorageEvent) => {
    if (event.key !== DISCARD_REQUEST_KEY || !event.newValue)
      return
    let request: DiscardRequest
    try {
      request = JSON.parse(event.newValue) as DiscardRequest
    }
    catch {
      return
    }
    if (typeof request?.meetingId !== 'number' || typeof request?.nonce !== 'string')
      return
    void (async () => {
      let ok = false
      try {
        ok = await handler(request.meetingId)
      }
      catch {
        ok = false
      }
      try {
        globalThis.localStorage?.setItem(DISCARD_RESULT_KEY, JSON.stringify({
          meetingId: request.meetingId,
          nonce: request.nonce,
          ok,
        } satisfies DiscardResult))
      }
      catch {
        // 无法写回结果时忽略：settings 端会因超时回退到直接删除。
      }
    })()
  }
  window.addEventListener('storage', onStorage)
  return () => window.removeEventListener('storage', onStorage)
}

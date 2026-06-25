import type { MeetingChunkUploadFields } from './transcription'
import {
  abortMeetingUpload,
  appendMeetingLiveChunk,
  completeMeetingLiveUpload,
  getMeetingUploadStatus,
  heartbeatMeetingUpload,
  initMeetingLiveUpload,
  MeetingUploadError,
} from './transcription'

// 桌面端会议录音「边录边传」客户端。
//
// 背景：以前整段长录音都暂存在内存的 sessionAudioChunks 里，停止时才一次性上传。
// 一旦崩溃 / 强退 / 断网，未上传的录音全部丢失，且 24h 长录音会持续占用内存。
//
// 本类把录音过程中累计的 PCM 每 ~5s 切成一个分片立即上传到服务端落盘，内存随之
// 释放；服务端在收到足够时长后会把会议「转正」（promote），即使客户端随后崩溃，
// 服务端的恢复任务与下次启动时的本地标记都能把已落盘的分片合并成会议。

// PCM 参数：16kHz / 单声道 / 16-bit little-endian。每秒 16000*2 = 32000 字节。
const BYTES_PER_SECOND = 16000 * 2
// 每累计约 5 秒音频就切一个分片上传，平衡请求频率与内存占用。
const FLUSH_BYTES = 5 * BYTES_PER_SECOND
// 心跳间隔：必须明显短于服务端「不活跃超时」(默认 60 分钟)，以免活跃录音被误判为中断。
const HEARTBEAT_INTERVAL_MS = 20_000
// 与服务端一致：低于该时长的录音直接丢弃，不生成会议。
const MIN_MEETING_DURATION_SECONDS = 5
// 单个分片的最大重试次数（指数退避）。超过后判定该会话「中断」，交由服务端恢复。
const MAX_UPLOAD_ATTEMPTS = 5

// localStorage 标记键：记录所有「尚未确认完成」的上传会话，供下次启动时恢复。
const MARKER_KEY = 'meeting-live-upload:pending'

export interface MeetingLiveUploadResult {
  meetingId: number | null
  status: string
  duration: number
  // discarded 为 true 表示因时长过短被丢弃（未生成会议）。
  discarded: boolean
}

export interface MeetingLiveUploadOptions {
  // 在真正发起 init 之前解析标题 / 工作流 / 语言等元信息（通常需要异步绑定工作流）。
  resolveInitFields?: () => Promise<MeetingChunkUploadFields> | MeetingChunkUploadFields
  // 上传过程中的非致命错误回调（仅用于上报，不应抛出）。
  onError?: (error: unknown) => void
  // 会议被服务端「转正」（promote，达到最小时长后分配 meetingId）时回调一次，
  // 便于把「正在录制的会议」状态发布给其它窗口（如 settings 的会议列表）。
  onMeetingPromoted?: (meetingId: number, uploadId: string) => void
  // 诊断日志回调。
  debug?: (event: string, detail?: unknown) => void
}

interface PendingMarker {
  uploadId: string
  title?: string
  startedAt: number
}

function readMarkers(): PendingMarker[] {
  try {
    const raw = globalThis.localStorage?.getItem(MARKER_KEY)
    if (!raw)
      return []
    const parsed = JSON.parse(raw)
    return Array.isArray(parsed) ? parsed.filter((m): m is PendingMarker => Boolean(m?.uploadId)) : []
  }
  catch {
    return []
  }
}

function writeMarkers(markers: PendingMarker[]): void {
  try {
    if (markers.length === 0)
      globalThis.localStorage?.removeItem(MARKER_KEY)
    else
      globalThis.localStorage?.setItem(MARKER_KEY, JSON.stringify(markers))
  }
  catch {
    // localStorage 不可用时忽略：服务端恢复任务仍是最终兜底。
  }
}

function addMarker(marker: PendingMarker): void {
  const next = readMarkers().filter(m => m.uploadId !== marker.uploadId)
  next.push(marker)
  writeMarkers(next)
}

function removeMarker(uploadId: string): void {
  writeMarkers(readMarkers().filter(m => m.uploadId !== uploadId))
}

function concatBuffers(parts: ArrayBuffer[]): Uint8Array {
  let total = 0
  for (const part of parts)
    total += part.byteLength
  const out = new Uint8Array(total)
  let offset = 0
  for (const part of parts) {
    out.set(new Uint8Array(part), offset)
    offset += part.byteLength
  }
  return out
}

async function sha256Hex(data: Uint8Array): Promise<string> {
  const subtle = globalThis.crypto?.subtle
  if (!subtle)
    return ''
  const digest = await subtle.digest('SHA-256', data as unknown as BufferSource)
  const bytes = new Uint8Array(digest)
  let hex = ''
  for (let i = 0; i < bytes.length; i++)
    hex += bytes[i].toString(16).padStart(2, '0')
  return hex
}

function delay(ms: number): Promise<void> {
  return new Promise(resolve => setTimeout(resolve, ms))
}

function backoffMs(attempt: number): number {
  return Math.min(8_000, 500 * 2 ** (attempt - 1))
}

export class MeetingLiveUpload {
  private readonly options: MeetingLiveUploadOptions
  private uploadId: string | null = null
  private initPromise: Promise<void> | null = null
  private pending: ArrayBuffer[] = []
  private pendingBytes = 0
  private totalBytes = 0
  private nextIndex = 0
  // FIFO 上传链：保证分片严格按序写入，避免服务端因乱序而产生空洞。
  private queue: Promise<void> = Promise.resolve()
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  private started = false
  private finished = false
  private cancelled = false
  // broken 表示某个分片重试耗尽，链路已不可信；此后停止接收新音频，交由服务端恢复。
  private broken = false
  private title?: string
  // 服务端「转正」后分配的 meetingId（首次出现时回调一次，之后不再重复通知）。
  private promotedMeetingId: number | null = null

  constructor(options: MeetingLiveUploadOptions = {}) {
    this.options = options
  }

  get durationSeconds(): number {
    return this.totalBytes / BYTES_PER_SECOND
  }

  // start 触发 init（解析元信息→创建会话→写本地标记→启动心跳）。可安全地以
  // fire-and-forget 方式调用：init 期间到来的分片会先缓冲，待 init 完成后再上传。
  start(): void {
    if (this.started)
      return
    this.started = true
    this.initPromise = this.runInit()
  }

  private async runInit(): Promise<void> {
    try {
      const fields = (await this.options.resolveInitFields?.()) ?? {}
      this.title = fields.title
      const init = await initMeetingLiveUpload(fields)
      this.uploadId = init.uploadId
      this.nextIndex = init.nextIndex || 0
      addMarker({ uploadId: init.uploadId, title: fields.title, startedAt: Date.now() })
      this.startHeartbeat()
      this.options.debug?.('init', { uploadId: init.uploadId })
    }
    catch (error) {
      this.broken = true
      this.options.onError?.(error)
      this.options.debug?.('init-failed', { error: String(error) })
    }
  }

  // pushPcm 接收一段 PCM（调用方需保证传入的 ArrayBuffer 不会被复用/修改）。
  pushPcm(chunk: ArrayBuffer): void {
    if (this.finished || this.cancelled || this.broken)
      return
    this.pending.push(chunk)
    this.pendingBytes += chunk.byteLength
    this.totalBytes += chunk.byteLength
    if (this.pendingBytes >= FLUSH_BYTES)
      this.cut(false)
  }

  private cut(final: boolean): void {
    if (this.pendingBytes === 0)
      return
    if (!final && this.pendingBytes < FLUSH_BYTES)
      return
    const data = concatBuffers(this.pending)
    this.pending = []
    this.pendingBytes = 0
    const index = this.nextIndex++
    this.queue = this.queue
      .then(() => this.uploadChunk(index, data))
      .catch((error) => {
        this.broken = true
        this.options.onError?.(error)
        this.options.debug?.('chunk-failed', { index, error: String(error) })
      })
  }

  private async uploadChunk(index: number, data: Uint8Array): Promise<void> {
    if (this.cancelled)
      return
    if (this.initPromise)
      await this.initPromise
    if (!this.uploadId)
      throw new Error('meeting upload session not initialized')
    const checksum = await sha256Hex(data)
    let attempt = 0
    for (;;) {
      try {
        const result = await appendMeetingLiveChunk(this.uploadId, index, data, checksum)
        this.notePromotion(result.meetingId)
        return
      }
      catch (error) {
        if (error instanceof MeetingUploadError) {
          // 409：服务端已存在该分片（幂等重发），视为成功，避免阻塞后续分片。
          if (error.status === 409)
            return
          // 其余 4xx（除超时/限流）不会因重试而成功，直接判定失败。
          if (error.status >= 400 && error.status < 500 && error.status !== 408 && error.status !== 429)
            throw error
        }
        attempt++
        if (attempt >= MAX_UPLOAD_ATTEMPTS)
          throw error
        await delay(backoffMs(attempt))
      }
    }
  }

  // notePromotion 在服务端首次返回 meetingId（会议被转正）时回调一次。
  private notePromotion(meetingId: number | null): void {
    if (meetingId == null || this.promotedMeetingId != null || !this.uploadId)
      return
    this.promotedMeetingId = meetingId
    try {
      this.options.onMeetingPromoted?.(meetingId, this.uploadId)
    }
    catch {
      // 通知失败不影响上传本身。
    }
  }

  private startHeartbeat(): void {
    // 若会话已结束（init 解析晚于 finish/detach 的竞态），不再启动定时器，避免泄漏。
    if (this.heartbeatTimer || this.finished || this.cancelled)
      return
    this.heartbeatTimer = setInterval(() => {
      if (!this.uploadId || this.finished || this.cancelled)
        return
      void heartbeatMeetingUpload(this.uploadId).catch(() => {
        // 心跳失败无需处理：分片上传本身也会刷新 last_seen，服务端超时窗口足够宽。
      })
    }, HEARTBEAT_INTERVAL_MS)
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }

  // finish：正常停止录音时调用。冲洗残余缓冲→等待上传链清空→请求服务端合并。
  // 时长不足时服务端会丢弃（这里也提前丢弃以省一次往返）。网络异常导致无法 complete
  // 时不 abort，保留本地标记 + 服务端恢复任务作为兜底。
  async finish(): Promise<MeetingLiveUploadResult> {
    if (this.finished || this.cancelled)
      return this.terminalResult()
    this.finished = true
    this.stopHeartbeat()
    this.cut(true)
    await this.queue.catch(() => {})
    if (this.initPromise)
      await this.initPromise.catch(() => {})
    const duration = this.durationSeconds
    if (!this.uploadId) {
      this.options.debug?.('finish-no-session', { duration })
      return { meetingId: null, status: 'failed', duration, discarded: true }
    }
    if (duration < MIN_MEETING_DURATION_SECONDS) {
      await abortMeetingUpload(this.uploadId)
      removeMarker(this.uploadId)
      this.options.debug?.('finish-discarded-short', { duration })
      return { meetingId: null, status: 'aborted', duration, discarded: true }
    }
    try {
      const result = await completeMeetingLiveUpload(this.uploadId)
      removeMarker(this.uploadId)
      this.options.debug?.('finish-completed', { meetingId: result.meetingId, status: result.status })
      return {
        meetingId: result.meetingId,
        status: result.status,
        duration: result.duration || duration,
        discarded: result.status === 'aborted',
      }
    }
    catch (error) {
      // 不 abort：保留已落盘分片，标记留待下次启动恢复，服务端维护任务也会兜底。
      this.options.onError?.(error)
      this.options.debug?.('finish-deferred', { error: String(error) })
      return { meetingId: null, status: 'interrupted', duration, discarded: false }
    }
  }

  // detach：异常/被动停止（如 reset）时调用。停止本地处理但不 abort，保留服务端
  // 数据与本地标记，避免数据丢失；剩余缓冲仍尽力后台上传。
  detach(): void {
    if (this.finished || this.cancelled)
      return
    this.finished = true
    this.stopHeartbeat()
    this.cut(true)
    void this.queue.catch(() => {})
    this.options.debug?.('detached', { duration: this.durationSeconds })
  }

  // cancel：用户明确丢弃录音时调用。停止并 abort 服务端会话、清除本地标记。
  async cancel(): Promise<void> {
    if (this.cancelled)
      return
    this.cancelled = true
    this.finished = true
    this.stopHeartbeat()
    this.pending = []
    this.pendingBytes = 0
    await this.queue.catch(() => {})
    if (this.initPromise)
      await this.initPromise.catch(() => {})
    if (this.uploadId) {
      await abortMeetingUpload(this.uploadId)
      removeMarker(this.uploadId)
    }
  }

  private terminalResult(): MeetingLiveUploadResult {
    return {
      meetingId: null,
      status: this.cancelled ? 'aborted' : 'completed',
      duration: this.durationSeconds,
      discarded: this.cancelled,
    }
  }
}

// recoverPendingMeetingUploads 在应用启动（登录后）调用：把上次会话遗留的、尚未
// 确认完成的上传会话尽力补完。短录音 abort、可合并的 complete、已不存在的清标记；
// 暂时性失败（离线/未登录）保留标记下次再试，服务端维护任务是最终兜底。
export async function recoverPendingMeetingUploads(
  debug?: (event: string, detail?: unknown) => void,
): Promise<void> {
  const markers = readMarkers()
  if (markers.length === 0)
    return
  debug?.('recover-start', { count: markers.length })
  for (const marker of markers) {
    try {
      const state = await getMeetingUploadStatus(marker.uploadId)
      if (!state) {
        // 会话已不存在（已完成或被清理）：清除标记。
        removeMarker(marker.uploadId)
        continue
      }
      if (state.status === 'completed' || state.status === 'aborted' || state.status === 'expired' || state.status === 'failed') {
        removeMarker(marker.uploadId)
        continue
      }
      if (state.totalBytes < MIN_MEETING_DURATION_SECONDS * BYTES_PER_SECOND) {
        await abortMeetingUpload(marker.uploadId)
        removeMarker(marker.uploadId)
        debug?.('recover-discarded-short', { uploadId: marker.uploadId, totalBytes: state.totalBytes })
        continue
      }
      const result = await completeMeetingLiveUpload(marker.uploadId)
      removeMarker(marker.uploadId)
      debug?.('recover-completed', { uploadId: marker.uploadId, meetingId: result.meetingId, status: result.status })
    }
    catch (error) {
      // 暂时性失败：保留标记，下次启动再试；服务端恢复任务也会处理。
      debug?.('recover-failed', { uploadId: marker.uploadId, error: String(error) })
    }
  }
}

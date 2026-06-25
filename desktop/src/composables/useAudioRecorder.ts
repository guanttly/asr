import type { RecoveryController } from './hotplugRecovery'
import { ref, shallowRef } from 'vue'
import { useAppStore } from '@/stores/app'
import { debugLog } from '@/utils/debug'
import { createRecoveryController, decideHotplugAction } from './hotplugRecovery'

const TARGET_SR = 16000
const CHUNK_MS = 200
const AUDIO_CONSTRAINTS: MediaStreamConstraints = {
  audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
  video: false,
}

type FloatBuffer = Float32Array<ArrayBufferLike>

interface RecorderStartOptions {
  onDeviceLost?: () => void
  onDeviceRestored?: () => void
}

export function isRecorderDeviceNotFound(error: unknown): boolean {
  return error instanceof Error && (error.name === 'NotFoundError' || error.name === 'DevicesNotFoundError')
}

export function mapRecorderError(error: unknown): Error {
  if (error instanceof Error) {
    switch (error.name) {
      case 'NotAllowedError':
      case 'PermissionDeniedError':
        return new Error('未授予麦克风权限，无法开始录音')
      case 'NotFoundError':
      case 'DevicesNotFoundError':
        return new Error('未检测到可用麦克风设备')
      case 'NotReadableError':
      case 'TrackStartError':
        return new Error('麦克风当前被其他应用占用，无法开始录音')
      default:
        return error
    }
  }

  return new Error('初始化录音失败')
}

function resampleLinear(input: FloatBuffer, srcSr: number, dstSr: number): FloatBuffer {
  if (srcSr === dstSr) return input
  const ratio = dstSr / srcSr
  const outLen = Math.max(0, Math.round(input.length * ratio))
  const out = new Float32Array(outLen)
  for (let i = 0; i < outLen; i++) {
    const x = i / ratio
    const x0 = Math.floor(x)
    const x1 = Math.min(x0 + 1, input.length - 1)
    const t = x - x0
    out[i] = input[x0] * (1 - t) + input[x1] * t
  }
  return out
}

function concatFloat32(a: FloatBuffer, b: FloatBuffer): FloatBuffer {
  const out = new Float32Array(a.length + b.length)
  out.set(a, 0)
  out.set(b, a.length)
  return out
}

function float32ToInt16(float32: FloatBuffer): ArrayBuffer {
  const int16 = new Int16Array(float32.length)
  for (let i = 0; i < float32.length; i++) {
    const s = Math.max(-1, Math.min(1, float32[i]))
    int16[i] = s < 0 ? s * 0x8000 : s * 0x7FFF
  }
  return int16.buffer
}

export function useAudioRecorder() {
  const appStore = useAppStore()
  const mediaStream = shallowRef<MediaStream | null>(null)
  const audioCtx = shallowRef<AudioContext | null>(null)
  const isRecording = ref(false)
  const isPaused = ref(false)

  let processor: ScriptProcessorNode | null = null
  let source: MediaStreamAudioSourceNode | null = null
  let buffer: FloatBuffer = new Float32Array(0)
  let onChunkCallback: ((chunk: ArrayBuffer) => void) | null = null
  let onDeviceLostCallback: (() => void) | null = null
  let onDeviceRestoredCallback: (() => void) | null = null
  let stoppedByUser = false
  let deviceLostDuringRecording = false
  let restoringPipeline = false
  let listeningForDeviceChanges = false
  let recoveryController: RecoveryController | null = null

  const setMicrophoneDetected = (detected: boolean) => {
    appStore.microphoneDetected = detected
  }

  const hasAudioInputDevice = async () => {
    if (!navigator.mediaDevices?.enumerateDevices)
      return true
    try {
      const devices = await navigator.mediaDevices.enumerateDevices()
      return devices.some(device => device.kind === 'audioinput')
    }
    catch {
      return true
    }
  }

  const releaseAudioPipeline = (stopTracks: boolean) => {
    processor?.disconnect()
    source?.disconnect()
    void audioCtx.value?.close()
    mediaStream.value?.getTracks().forEach((track) => {
      track.removeEventListener?.('ended', handleTrackEnded)
      track.removeEventListener?.('mute', handleTrackMuted)
      track.removeEventListener?.('unmute', handleTrackUnmuted)
      if (stopTracks)
        track.stop()
    })
    processor = null
    source = null
    audioCtx.value = null
    mediaStream.value = null
  }

  // 当前采集 track 是否健康：仍 live 且未被系统静音。拔掉默认设备时，部分平台
  // 只会把 track 置为 muted 而不触发 ended，需据此识别采集已中断。
  const isCurrentTrackHealthy = () => {
    const tracks = mediaStream.value?.getAudioTracks?.() ?? []
    return tracks.length > 0 && tracks.some(track => track.readyState === 'live' && !track.muted)
  }

  const addDeviceChangeListener = () => {
    if (listeningForDeviceChanges || !navigator.mediaDevices?.addEventListener)
      return
    navigator.mediaDevices.addEventListener('devicechange', handleDeviceChange)
    listeningForDeviceChanges = true
  }

  const removeDeviceChangeListener = () => {
    if (!listeningForDeviceChanges || !navigator.mediaDevices?.removeEventListener)
      return
    navigator.mediaDevices.removeEventListener('devicechange', handleDeviceChange)
    listeningForDeviceChanges = false
  }

  // 进入「设备丢失」状态（幂等）：释放采集管道、提示用户，并启动带退避的自动重连。
  function markDeviceLost(reason: string) {
    if (stoppedByUser || !isRecording.value)
      return
    if (deviceLostDuringRecording) {
      recoveryController?.kick()
      return
    }
    deviceLostDuringRecording = true
    isPaused.value = true
    setMicrophoneDetected(false)
    releaseAudioPipeline(true)
    onDeviceLostCallback?.()
    void debugLog('audio.error', `microphone lost during recording: ${reason}`)
    recoveryController?.kick()
  }

  // 单次重连尝试：成功重建管道返回 true；无设备或失败返回 false 以触发退避重试。
  const attemptRecovery = async (): Promise<boolean> => {
    if (stoppedByUser || !isRecording.value)
      return true
    const detected = await hasAudioInputDevice()
    if (!detected) {
      setMicrophoneDetected(false)
      return false
    }
    restoringPipeline = true
    try {
      await openAudioPipeline()
      return true
    }
    catch (error) {
      setMicrophoneDetected(!isRecorderDeviceNotFound(error))
      void debugLog('audio.error', 'failed to restore microphone stream', error instanceof Error ? { name: error.name, message: error.message } : error)
      return false
    }
    finally {
      restoringPipeline = false
    }
  }

  async function handleDeviceChange() {
    const hasInputDevice = await hasAudioInputDevice()
    if (!isRecording.value) {
      // 空闲态：仅刷新「是否检测到麦克风」指示。
      setMicrophoneDetected(hasInputDevice)
      return
    }
    const action = decideHotplugAction({
      recording: isRecording.value,
      stoppedByUser,
      restoring: restoringPipeline,
      alreadyLost: deviceLostDuringRecording,
      hasInputDevice,
      trackHealthy: isCurrentTrackHealthy(),
    })
    if (action === 'mark-lost')
      markDeviceLost('devicechange')
    else if (action === 'attempt-recover')
      recoveryController?.kick()
  }

  function handleTrackEnded() {
    markDeviceLost('track-ended')
  }

  function handleTrackMuted() {
    markDeviceLost('track-muted')
  }

  function handleTrackUnmuted() {
    if (deviceLostDuringRecording)
      recoveryController?.kick()
  }

  async function openAudioPipeline() {
    releaseAudioPipeline(true)
    mediaStream.value = await navigator.mediaDevices.getUserMedia(AUDIO_CONSTRAINTS)

    setMicrophoneDetected(true)
    appStore.microphonePermissionGranted = true
    appStore.persist()

    audioCtx.value = new AudioContext()
    await audioCtx.value.resume().catch(() => undefined)
    source = audioCtx.value.createMediaStreamSource(mediaStream.value)
    processor = audioCtx.value.createScriptProcessor(4096, 1, 1)

    mediaStream.value.getTracks().forEach((track) => {
      track.addEventListener?.('ended', handleTrackEnded)
      track.addEventListener?.('mute', handleTrackMuted)
      track.addEventListener?.('unmute', handleTrackUnmuted)
    })

    await debugLog('audio', 'microphone stream initialized', {
      trackCount: mediaStream.value.getTracks().length,
      sampleRate: audioCtx.value.sampleRate,
    })

    const chunkSamples = Math.round(TARGET_SR * (CHUNK_MS / 1000))
    let loggedFirstChunk = false

    processor.onaudioprocess = (e) => {
      if (!isRecording.value || isPaused.value) return
      const input = e.inputBuffer.getChannelData(0)
      const resampled = resampleLinear(new Float32Array(input), audioCtx.value!.sampleRate, TARGET_SR)
      buffer = concatFloat32(buffer, resampled)

      if (onChunkCallback) {
        while (buffer.length >= chunkSamples) {
          const chunk = buffer.slice(0, chunkSamples)
          buffer = buffer.slice(chunkSamples)
          if (!loggedFirstChunk) {
            loggedFirstChunk = true
            void debugLog('audio', 'captured first audio chunk', { chunkSamples })
          }
          onChunkCallback(float32ToInt16(chunk))
        }
      }
    }

    // 重连尝试期间用户可能已停止：避免遗留一个「野」采集管道。
    if (stoppedByUser) {
      releaseAudioPipeline(true)
      const aborted = new Error('recorder stopped during recovery')
      aborted.name = 'AbortError'
      throw aborted
    }

    source.connect(processor)
    processor.connect(audioCtx.value.destination)
    isRecording.value = true
    isPaused.value = false
    addDeviceChangeListener()
  }

  const cleanup = () => {
    recoveryController?.cancel()
    recoveryController = null
    releaseAudioPipeline(true)
    removeDeviceChangeListener()
    onChunkCallback = null
    onDeviceLostCallback = null
    onDeviceRestoredCallback = null
    buffer = new Float32Array(0)
    isRecording.value = false
    isPaused.value = false
    deviceLostDuringRecording = false
    restoringPipeline = false
  }

  const start = async (onChunk?: (chunk: ArrayBuffer) => void, options?: RecorderStartOptions) => {
    stoppedByUser = false
    onChunkCallback = onChunk ?? null
    onDeviceLostCallback = options?.onDeviceLost ?? null
    onDeviceRestoredCallback = options?.onDeviceRestored ?? null
    buffer = new Float32Array(0)
    deviceLostDuringRecording = false
    restoringPipeline = false

    recoveryController?.cancel()
    recoveryController = createRecoveryController({
      attempt: attemptRecovery,
      isActive: () => isRecording.value && !stoppedByUser && deviceLostDuringRecording,
      onRecovered: () => {
        deviceLostDuringRecording = false
        onDeviceRestoredCallback?.()
        void debugLog('audio', 'microphone stream restored after device change')
      },
    })

    try {
      await debugLog('audio', 'requesting microphone stream')
      await openAudioPipeline()
    }
    catch (error) {
      setMicrophoneDetected(!isRecorderDeviceNotFound(error))
      appStore.microphonePermissionGranted = false
      appStore.persist()
      cleanup()
      void debugLog('audio.error', 'failed to initialize recorder', error instanceof Error ? { name: error.name, message: error.message } : error)
      throw mapRecorderError(error)
    }
  }

  const stop = () => {
    stoppedByUser = true
    if (buffer.length > 0 && onChunkCallback) {
      onChunkCallback(float32ToInt16(buffer.slice()))
      buffer = new Float32Array(0)
    }
    void debugLog('audio', 'stopping recorder')
    cleanup()
  }

  const pause = () => { isPaused.value = true }
  const resume = () => { isPaused.value = false }

  const requestPermission = async () => {
    try {
      await debugLog('audio', 'checking microphone permission')
      const stream = await navigator.mediaDevices.getUserMedia(AUDIO_CONSTRAINTS)
      stream.getTracks().forEach(track => track.stop())
      setMicrophoneDetected(true)
      appStore.microphonePermissionGranted = true
      appStore.persist()
      await debugLog('audio', 'microphone permission granted')
      return true
    }
    catch (error) {
      setMicrophoneDetected(!isRecorderDeviceNotFound(error))
      appStore.microphonePermissionGranted = false
      appStore.persist()
      void debugLog('audio.error', 'microphone permission request failed', error instanceof Error ? { name: error.name, message: error.message } : error)
      throw mapRecorderError(error)
    }
  }

  const refreshDeviceAvailability = async () => {
    const detected = await hasAudioInputDevice()
    setMicrophoneDetected(detected)
    return detected
  }

  return { isRecording, isPaused, start, stop, pause, resume, requestPermission, refreshDeviceAvailability }
}

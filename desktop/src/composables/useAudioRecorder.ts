import { ref, shallowRef } from 'vue'
import { useAppStore } from '@/stores/app'
import { debugLog } from '@/utils/debug'

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
  let restartingAfterDeviceChange = false
  let listeningForDeviceChanges = false

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
      if (stopTracks)
        track.stop()
    })
    processor = null
    source = null
    audioCtx.value = null
    mediaStream.value = null
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

  async function handleDeviceChange() {
    const detected = await hasAudioInputDevice()
    setMicrophoneDetected(detected)
    if (!detected) {
      if (isRecording.value) {
        deviceLostDuringRecording = true
        isPaused.value = true
        onDeviceLostCallback?.()
      }
      return
    }

    if (!deviceLostDuringRecording || !isRecording.value || restartingAfterDeviceChange || !onChunkCallback)
      return

    restartingAfterDeviceChange = true
    try {
      await openAudioPipeline()
      deviceLostDuringRecording = false
      onDeviceRestoredCallback?.()
      await debugLog('audio', 'microphone stream restored after device change')
    }
    catch (error) {
      setMicrophoneDetected(!isRecorderDeviceNotFound(error))
      void debugLog('audio.error', 'failed to restore microphone stream', error instanceof Error ? { name: error.name, message: error.message } : error)
    }
    finally {
      restartingAfterDeviceChange = false
    }
  }

  function handleTrackEnded() {
    if (stoppedByUser || !isRecording.value)
      return
    deviceLostDuringRecording = true
    isPaused.value = true
    setMicrophoneDetected(false)
    releaseAudioPipeline(false)
    onDeviceLostCallback?.()
    void debugLog('audio.error', 'microphone track ended during recording')
    void handleDeviceChange()
  }

  async function openAudioPipeline() {
    releaseAudioPipeline(false)
    mediaStream.value = await navigator.mediaDevices.getUserMedia(AUDIO_CONSTRAINTS)

    setMicrophoneDetected(true)
    appStore.microphonePermissionGranted = true
    appStore.persist()

    audioCtx.value = new AudioContext()
    await audioCtx.value.resume().catch(() => undefined)
    source = audioCtx.value.createMediaStreamSource(mediaStream.value)
    processor = audioCtx.value.createScriptProcessor(4096, 1, 1)

    mediaStream.value.getTracks().forEach(track => track.addEventListener?.('ended', handleTrackEnded))

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

    source.connect(processor)
    processor.connect(audioCtx.value.destination)
    isRecording.value = true
    isPaused.value = false
    addDeviceChangeListener()
  }

  const cleanup = () => {
    releaseAudioPipeline(true)
    removeDeviceChangeListener()
    onChunkCallback = null
    onDeviceLostCallback = null
    onDeviceRestoredCallback = null
    buffer = new Float32Array(0)
    isRecording.value = false
    isPaused.value = false
    deviceLostDuringRecording = false
    restartingAfterDeviceChange = false
  }

  const start = async (onChunk?: (chunk: ArrayBuffer) => void, options?: RecorderStartOptions) => {
    stoppedByUser = false
    onChunkCallback = onChunk ?? null
    onDeviceLostCallback = options?.onDeviceLost ?? null
    onDeviceRestoredCallback = options?.onDeviceRestored ?? null
    buffer = new Float32Array(0)
    deviceLostDuringRecording = false

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

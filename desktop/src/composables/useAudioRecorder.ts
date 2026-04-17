import { ref, shallowRef } from 'vue'
import { useAppStore } from '@/stores/app'
import { debugLog } from '@/utils/debug'

const TARGET_SR = 16000
const CHUNK_MS = 300

type FloatBuffer = Float32Array<ArrayBufferLike>

function mapRecorderError(error: unknown): Error {
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

  const cleanup = () => {
    processor?.disconnect()
    source?.disconnect()
    void audioCtx.value?.close()
    mediaStream.value?.getTracks().forEach(t => t.stop())
    processor = null
    source = null
    audioCtx.value = null
    mediaStream.value = null
    onChunkCallback = null
    buffer = new Float32Array(0)
    isRecording.value = false
    isPaused.value = false
  }

  const start = async (onChunk?: (chunk: ArrayBuffer) => void) => {
    onChunkCallback = onChunk ?? null
    buffer = new Float32Array(0)
    let loggedFirstChunk = false

    try {
      await debugLog('audio', 'requesting microphone stream')
      mediaStream.value = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
        video: false,
      })

      appStore.microphonePermissionGranted = true
      appStore.persist()

      audioCtx.value = new AudioContext()
      await audioCtx.value.resume().catch(() => undefined)
      source = audioCtx.value.createMediaStreamSource(mediaStream.value)
      processor = audioCtx.value.createScriptProcessor(4096, 1, 1)

      await debugLog('audio', 'microphone stream initialized', {
        trackCount: mediaStream.value.getTracks().length,
        sampleRate: audioCtx.value.sampleRate,
      })

      const chunkSamples = Math.round(TARGET_SR * (CHUNK_MS / 1000))

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
    }
    catch (error) {
      appStore.microphonePermissionGranted = false
      appStore.persist()
      cleanup()
      void debugLog('audio.error', 'failed to initialize recorder', error instanceof Error ? { name: error.name, message: error.message } : error)
      throw mapRecorderError(error)
    }
  }

  const stop = () => {
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
      const stream = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
        video: false,
      })
      stream.getTracks().forEach(track => track.stop())
      appStore.microphonePermissionGranted = true
      appStore.persist()
      await debugLog('audio', 'microphone permission granted')
      return true
    }
    catch (error) {
      appStore.microphonePermissionGranted = false
      appStore.persist()
      void debugLog('audio.error', 'microphone permission request failed', error instanceof Error ? { name: error.name, message: error.message } : error)
      throw mapRecorderError(error)
    }
  }

  return { isRecording, isPaused, start, stop, pause, resume, requestPermission }
}

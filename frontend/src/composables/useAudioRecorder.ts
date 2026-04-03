import { ref, shallowRef } from 'vue'

const TARGET_SR = 16000
const CHUNK_MS = 300

function mapRecorderError(error: unknown): Error {
  if (error instanceof Error) {
    switch (error.name) {
      case 'NotAllowedError':
      case 'PermissionDeniedError':
        return new Error('浏览器未授予麦克风权限，或当前页面不是 HTTPS / localhost 安全上下文')
      case 'NotFoundError':
      case 'DevicesNotFoundError':
        return new Error('未检测到可用麦克风设备')
      case 'NotReadableError':
      case 'TrackStartError':
        return new Error('麦克风当前被其他应用占用，无法开始录音')
      case 'OverconstrainedError':
      case 'ConstraintNotSatisfiedError':
        return new Error('当前浏览器或设备不支持请求的音频采集参数')
      default:
        return error
    }
  }

  return new Error('初始化录音失败')
}

function resampleLinear(input: Float32Array, srcSr: number, dstSr: number): Float32Array {
  if (srcSr === dstSr)
    return input
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

function concatFloat32(a: Float32Array, b: Float32Array): Float32Array {
  const out = new Float32Array(a.length + b.length)
  out.set(a, 0)
  out.set(b, a.length)
  return out
}

export function useAudioRecorder() {
  const mediaStream = shallowRef<MediaStream | null>(null)
  const audioCtx = shallowRef<AudioContext | null>(null)
  const isRecording = ref(false)
  const isPaused = ref(false)

  let processor: ScriptProcessorNode | null = null
  let source: MediaStreamAudioSourceNode | null = null
  let buffer = new Float32Array(0)
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

    if (!window.isSecureContext)
      throw new Error('当前页面不是安全上下文，浏览器通常只允许在 HTTPS 或 localhost 下使用麦克风')

    if (!navigator.mediaDevices?.getUserMedia)
      throw new Error('当前浏览器不支持麦克风采集接口')

    if (typeof AudioContext === 'undefined')
      throw new Error('当前浏览器不支持 Web Audio API')

    try {
      mediaStream.value = await navigator.mediaDevices.getUserMedia({
        audio: { channelCount: 1, echoCancellation: true, noiseSuppression: true, autoGainControl: true },
        video: false,
      })

      audioCtx.value = new AudioContext()
      source = audioCtx.value.createMediaStreamSource(mediaStream.value)
      processor = audioCtx.value.createScriptProcessor(4096, 1, 1)

      const chunkSamples = Math.round(TARGET_SR * (CHUNK_MS / 1000))

      processor.onaudioprocess = (e) => {
        if (!isRecording.value || isPaused.value)
          return
        const input = e.inputBuffer.getChannelData(0)
        const resampled = resampleLinear(new Float32Array(input), audioCtx.value!.sampleRate, TARGET_SR)
        buffer = concatFloat32(buffer, resampled)

        while (buffer.length >= chunkSamples && onChunkCallback) {
          const chunk = buffer.slice(0, chunkSamples)
          buffer = buffer.slice(chunkSamples)
          onChunkCallback(chunk.buffer)
        }
      }

      source.connect(processor)
      processor.connect(audioCtx.value.destination)

      isRecording.value = true
      isPaused.value = false
    }
    catch (error) {
      cleanup()
      throw mapRecorderError(error)
    }
  }

  const stop = () => {
    // Flush remaining buffer.
    if (buffer.length > 0 && onChunkCallback) {
      onChunkCallback(buffer.slice().buffer)
      buffer = new Float32Array(0)
    }

    cleanup()
  }

  const pause = () => { isPaused.value = true }
  const resume = () => { isPaused.value = false }

  return { isRecording, isPaused, start, stop, pause, resume }
}
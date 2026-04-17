import { computed, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'

export interface RecognitionSettings {
  keepPunctuation: boolean
  minSpeechThreshold: number
  noiseGateMultiplier: number
  endSilenceChunks: number
  minEffectiveSpeechChunks: number
  singleChunkPeakMultiplier: number
}

const DEFAULTS: RecognitionSettings = {
  keepPunctuation: false,
  minSpeechThreshold: 0.018,
  noiseGateMultiplier: 2.8,
  endSilenceChunks: 4,
  minEffectiveSpeechChunks: 2,
  singleChunkPeakMultiplier: 1.45,
}

function clamp(v: number, min: number, max: number) {
  return Math.min(max, Math.max(min, v))
}

function normalize(raw?: Partial<RecognitionSettings>): RecognitionSettings {
  const r = raw || {}
  return {
    keepPunctuation: Boolean(r.keepPunctuation),
    minSpeechThreshold: clamp(Number(r.minSpeechThreshold) || DEFAULTS.minSpeechThreshold, 0.005, 0.08),
    noiseGateMultiplier: clamp(Number(r.noiseGateMultiplier) || DEFAULTS.noiseGateMultiplier, 1.2, 6),
    endSilenceChunks: Math.round(clamp(Number(r.endSilenceChunks) || DEFAULTS.endSilenceChunks, 2, 10)),
    minEffectiveSpeechChunks: Math.round(clamp(Number(r.minEffectiveSpeechChunks) || DEFAULTS.minEffectiveSpeechChunks, 1, 6)),
    singleChunkPeakMultiplier: clamp(Number(r.singleChunkPeakMultiplier) || DEFAULTS.singleChunkPeakMultiplier, 1, 3),
  }
}

export function useSettings() {
  const appStore = useAppStore()
  const settings = ref<RecognitionSettings>(normalize(appStore.recognitionSettings))

  watch(settings, (v) => {
    const n = normalize(v)
    Object.assign(appStore.recognitionSettings, n)
    appStore.persist()
  }, { deep: true })

  const effectiveSettings = computed(() => normalize(settings.value))

  const reset = () => {
    settings.value = { ...DEFAULTS }
  }

  return { settings, effectiveSettings, reset, DEFAULTS }
}

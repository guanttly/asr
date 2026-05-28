import { computed, ref, watch } from 'vue'
import { useAppStore } from '@/stores/app'
import { DEFAULT_RECOGNITION_SETTINGS, normalizeRecognitionSettings } from '@/utils/settingsValidation'

export interface RecognitionSettings {
  keepPunctuation: boolean
  minSpeechThreshold: number
  noiseGateMultiplier: number
  endSilenceChunks: number
  minEffectiveSpeechChunks: number
  singleChunkPeakMultiplier: number
}

const DEFAULTS: RecognitionSettings = DEFAULT_RECOGNITION_SETTINGS

export function useSettings() {
  const appStore = useAppStore()
  const settings = ref<RecognitionSettings>(normalizeRecognitionSettings(appStore.recognitionSettings))

  watch(settings, (v) => {
    const n = normalizeRecognitionSettings(v)
    Object.assign(appStore.recognitionSettings, n)
    appStore.persist()
  }, { deep: true })

  const effectiveSettings = computed(() => normalizeRecognitionSettings(settings.value))

  const reset = () => {
    settings.value = { ...DEFAULTS }
  }

  return { settings, effectiveSettings, reset, DEFAULTS }
}

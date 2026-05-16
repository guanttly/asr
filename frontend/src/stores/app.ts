import type { WorkflowBindingKey, WorkflowBindings } from '@/types/workflow'
import type { HardwareProfilePayload, ProductCapabilitiesPayload, ProductFeaturesPayload, ProductLanguagePayload } from '@/api/appSettings'
import type { ProductFeatureKey } from '@/constants/product'

import { defineStore } from 'pinia'

import { getCurrentUserWorkflowBindings, updateCurrentUserWorkflowBindings } from '@/api/user'
import { getProductFeatures } from '@/api/appSettings'
import { PRODUCT_EDITIONS, PRODUCT_FEATURE_KEYS } from '@/constants/product'
import { WORKFLOW_BINDING_KEYS } from '@/types/workflow'

const LEGACY_WORKFLOW_BINDINGS_STORAGE_KEY = 'asr_app_workflow_bindings'

function defaultProductLanguages(): ProductLanguagePayload[] {
  // 首选项必须是 auto：医学场景常有中英混合（缩写/单位/药名），
  // 锁中文会让英文术语被错听，锁英文会丢中文病历主体。
  return [
    { code: 'auto', label: '自动识别（中英混合）' },
    { code: 'zh-CN', label: '普通话' },
    { code: 'en-US', label: '英文（美）' },
  ]
}

function defaultHardwareRequirements(): Partial<Record<typeof PRODUCT_EDITIONS[keyof typeof PRODUCT_EDITIONS], HardwareProfilePayload>> {
  return {
    [PRODUCT_EDITIONS.STANDARD]: {
      tier: PRODUCT_EDITIONS.STANDARD,
      minimum: { cpu: '8 核', memory: '16 GB', storage: '200 GB SSD', acceleration: 'RTX 3090' },
      recommended: { cpu: '16 核', memory: '32 GB', storage: '500 GB SSD', acceleration: 'A10 / A100' },
    },
    [PRODUCT_EDITIONS.ADVANCED]: {
      tier: PRODUCT_EDITIONS.ADVANCED,
      minimum: { cpu: '16 核', memory: '32 GB', storage: '500 GB SSD', acceleration: 'A10' },
      recommended: { cpu: '16 核及以上', memory: '32 GB 及以上', storage: '500 GB SSD 及以上', acceleration: 'A100' },
    },
  }
}

function defaultProductFeatures(): ProductFeaturesPayload {
  return {
    edition: PRODUCT_EDITIONS.STANDARD,
    capabilities: {
      [PRODUCT_FEATURE_KEYS.REALTIME]: true,
      [PRODUCT_FEATURE_KEYS.BATCH]: true,
      [PRODUCT_FEATURE_KEYS.MEETING]: false,
      [PRODUCT_FEATURE_KEYS.VOICEPRINT]: false,
      [PRODUCT_FEATURE_KEYS.VOICE_CONTROL]: false,
    },
    supported_languages: defaultProductLanguages(),
    hardware_tier: PRODUCT_EDITIONS.STANDARD,
    hardware_requirements: defaultHardwareRequirements(),
  }
}

function normalizeProductFeatures(raw?: Partial<ProductFeaturesPayload> | null): ProductFeaturesPayload {
  const defaults = defaultProductFeatures()
  const capabilities: Partial<ProductCapabilitiesPayload> = raw?.capabilities || {}
  return {
    edition: raw?.edition === PRODUCT_EDITIONS.ADVANCED ? PRODUCT_EDITIONS.ADVANCED : defaults.edition,
    capabilities: {
      [PRODUCT_FEATURE_KEYS.REALTIME]: capabilities[PRODUCT_FEATURE_KEYS.REALTIME] !== false,
      [PRODUCT_FEATURE_KEYS.BATCH]: capabilities[PRODUCT_FEATURE_KEYS.BATCH] !== false,
      [PRODUCT_FEATURE_KEYS.MEETING]: capabilities[PRODUCT_FEATURE_KEYS.MEETING] === true,
      [PRODUCT_FEATURE_KEYS.VOICEPRINT]: capabilities[PRODUCT_FEATURE_KEYS.VOICEPRINT] === true,
      [PRODUCT_FEATURE_KEYS.VOICE_CONTROL]: capabilities[PRODUCT_FEATURE_KEYS.VOICE_CONTROL] === true,
    },
    supported_languages: Array.isArray(raw?.supported_languages) && raw.supported_languages.length > 0
      ? raw.supported_languages.filter(item => typeof item.code === 'string' && typeof item.label === 'string')
      : defaults.supported_languages,
    hardware_tier: raw?.hardware_tier === PRODUCT_EDITIONS.ADVANCED ? PRODUCT_EDITIONS.ADVANCED : defaults.hardware_tier,
    hardware_requirements: raw?.hardware_requirements && Object.keys(raw.hardware_requirements).length > 0
      ? { ...defaults.hardware_requirements, ...raw.hardware_requirements }
      : defaults.hardware_requirements,
  }
}

function defaultWorkflowBindings(): WorkflowBindings {
  return {
    [WORKFLOW_BINDING_KEYS.REALTIME]: null,
    [WORKFLOW_BINDING_KEYS.BATCH]: null,
    [WORKFLOW_BINDING_KEYS.MEETING]: null,
    [WORKFLOW_BINDING_KEYS.VOICE_CONTROL]: null,
  }
}

function normalizeWorkflowBindings(raw?: Partial<Record<WorkflowBindingKey, unknown>> | null): WorkflowBindings {
  const realtime = raw?.[WORKFLOW_BINDING_KEYS.REALTIME]
  const batch = raw?.[WORKFLOW_BINDING_KEYS.BATCH]
  const meeting = raw?.[WORKFLOW_BINDING_KEYS.MEETING]
  const voiceControl = raw?.[WORKFLOW_BINDING_KEYS.VOICE_CONTROL]
  return {
    [WORKFLOW_BINDING_KEYS.REALTIME]: typeof realtime === 'number' ? realtime : null,
    [WORKFLOW_BINDING_KEYS.BATCH]: typeof batch === 'number' ? batch : null,
    [WORKFLOW_BINDING_KEYS.MEETING]: typeof meeting === 'number' ? meeting : null,
    [WORKFLOW_BINDING_KEYS.VOICE_CONTROL]: typeof voiceControl === 'number' ? voiceControl : null,
  }
}

function loadLegacyWorkflowBindings(): WorkflowBindings {
  if (typeof window === 'undefined')
    return defaultWorkflowBindings()

  try {
    const raw = localStorage.getItem(LEGACY_WORKFLOW_BINDINGS_STORAGE_KEY)
    if (!raw)
      return defaultWorkflowBindings()

    return normalizeWorkflowBindings(JSON.parse(raw) as Partial<WorkflowBindings>)
  }
  catch {
    return defaultWorkflowBindings()
  }
}

function clearLegacyWorkflowBindings() {
  if (typeof window === 'undefined')
    return
  localStorage.removeItem(LEGACY_WORKFLOW_BINDINGS_STORAGE_KEY)
}

function hasWorkflowBindingValue(bindings: WorkflowBindings) {
  return Object.values(bindings).some(value => typeof value === 'number')
}

function sanitizeWorkflowBindings(bindings: WorkflowBindings, capabilities: ProductCapabilitiesPayload): WorkflowBindings {
  return {
    [WORKFLOW_BINDING_KEYS.REALTIME]: bindings[WORKFLOW_BINDING_KEYS.REALTIME],
    [WORKFLOW_BINDING_KEYS.BATCH]: bindings[WORKFLOW_BINDING_KEYS.BATCH],
    [WORKFLOW_BINDING_KEYS.MEETING]: capabilities[PRODUCT_FEATURE_KEYS.MEETING] ? bindings[WORKFLOW_BINDING_KEYS.MEETING] : null,
    [WORKFLOW_BINDING_KEYS.VOICE_CONTROL]: capabilities[PRODUCT_FEATURE_KEYS.VOICE_CONTROL] ? bindings[WORKFLOW_BINDING_KEYS.VOICE_CONTROL] : null,
  }
}

export const useAppStore = defineStore('app', {
  state: () => ({
    siderCollapsed: false,
    productEdition: defaultProductFeatures().edition,
    productCapabilities: defaultProductFeatures().capabilities,
    productSupportedLanguages: defaultProductFeatures().supported_languages,
    productHardwareTier: defaultProductFeatures().hardware_tier,
    productHardwareRequirements: defaultProductFeatures().hardware_requirements,
    productFeaturesReady: false,
    workflowBindings: defaultWorkflowBindings(),
    workflowBindingsReady: false,
    workflowBindingsLoading: false,
    workflowBindingsSaving: false,
  }),
  actions: {
    toggleSider() {
      this.siderCollapsed = !this.siderCollapsed
    },
    hasCapability(key: ProductFeatureKey) {
      return Boolean(this.productCapabilities[key])
    },
    applyProductFeatures(payload?: ProductFeaturesPayload | null) {
    const normalized = normalizeProductFeatures(payload)
    this.productEdition = normalized.edition
    this.productCapabilities = normalized.capabilities
    this.productSupportedLanguages = normalized.supported_languages
    this.productHardwareTier = normalized.hardware_tier
    this.productHardwareRequirements = normalized.hardware_requirements
    this.productFeaturesReady = true
    this.workflowBindings = sanitizeWorkflowBindings(this.workflowBindings, normalized.capabilities)
  },
    async bootstrapProductFeatures() {
    if (typeof window === 'undefined' || !localStorage.getItem('asr_token')) {
      this.applyProductFeatures(null)
      return
    }

    try {
      const result = await getProductFeatures()
      this.applyProductFeatures(result.data)
    }
    catch {
      this.applyProductFeatures(null)
    }
  },
    resetWorkflowBindings() {
      this.workflowBindings = sanitizeWorkflowBindings(defaultWorkflowBindings(), this.productCapabilities)
      this.workflowBindingsReady = true
      this.workflowBindingsLoading = false
      this.workflowBindingsSaving = false
      clearLegacyWorkflowBindings()
    },
    async bootstrapWorkflowBindings() {
      const legacyBindings = loadLegacyWorkflowBindings()

      if (typeof window === 'undefined') {
        this.workflowBindings = sanitizeWorkflowBindings(legacyBindings, this.productCapabilities)
        this.workflowBindingsReady = true
        return
      }

      if (!localStorage.getItem('asr_token')) {
        this.workflowBindings = sanitizeWorkflowBindings(defaultWorkflowBindings(), this.productCapabilities)
        this.workflowBindingsReady = true
        this.workflowBindingsLoading = false
        return
      }

      this.workflowBindingsLoading = true
      try {
        const result = await getCurrentUserWorkflowBindings()
        const remoteBindings = normalizeWorkflowBindings(result.data)

        if (!hasWorkflowBindingValue(remoteBindings) && hasWorkflowBindingValue(legacyBindings)) {
          const nextBindings = sanitizeWorkflowBindings(legacyBindings, this.productCapabilities)
          await updateCurrentUserWorkflowBindings(nextBindings)
          this.workflowBindings = nextBindings
          clearLegacyWorkflowBindings()
        }
        else {
          this.workflowBindings = sanitizeWorkflowBindings(remoteBindings, this.productCapabilities)
          if (hasWorkflowBindingValue(remoteBindings))
            clearLegacyWorkflowBindings()
        }
      }
      catch {
        this.workflowBindings = sanitizeWorkflowBindings(
				hasWorkflowBindingValue(legacyBindings) ? legacyBindings : defaultWorkflowBindings(),
				this.productCapabilities,
			)
      }
      finally {
        this.workflowBindingsLoading = false
        this.workflowBindingsReady = true
      }
    },
    async replaceWorkflowBindings(bindings: WorkflowBindings) {
      const previousBindings = this.workflowBindings
      const nextBindings = sanitizeWorkflowBindings(bindings, this.productCapabilities)
      this.workflowBindings = nextBindings
      this.workflowBindingsSaving = true

      try {
        await updateCurrentUserWorkflowBindings(nextBindings)
        clearLegacyWorkflowBindings()
      }
      catch (error) {
        this.workflowBindings = previousBindings
        throw error
      }
      finally {
        this.workflowBindingsSaving = false
      }
    },
    async setWorkflowBinding(key: WorkflowBindingKey, workflowId: number | null) {
      await this.replaceWorkflowBindings({
        ...this.workflowBindings,
        [key]: workflowId,
      })
    },
  },
})

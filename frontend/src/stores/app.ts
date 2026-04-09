import type { WorkflowBindingKey, WorkflowBindings } from '@/types/workflow'

import { defineStore } from 'pinia'

import { getCurrentUserWorkflowBindings, updateCurrentUserWorkflowBindings } from '@/api/user'

const LEGACY_WORKFLOW_BINDINGS_STORAGE_KEY = 'asr_app_workflow_bindings'

function defaultWorkflowBindings(): WorkflowBindings {
  return {
    realtime: null,
    batch: null,
    meeting: null,
  }
}

function normalizeWorkflowBindings(raw?: Partial<Record<WorkflowBindingKey, unknown>> | null): WorkflowBindings {
  return {
    realtime: typeof raw?.realtime === 'number' ? raw.realtime : null,
    batch: typeof raw?.batch === 'number' ? raw.batch : null,
    meeting: typeof raw?.meeting === 'number' ? raw.meeting : null,
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

export const useAppStore = defineStore('app', {
  state: () => ({
    siderCollapsed: false,
    workflowBindings: defaultWorkflowBindings(),
    workflowBindingsReady: false,
    workflowBindingsLoading: false,
    workflowBindingsSaving: false,
  }),
  actions: {
    toggleSider() {
      this.siderCollapsed = !this.siderCollapsed
    },
    resetWorkflowBindings() {
      this.workflowBindings = defaultWorkflowBindings()
      this.workflowBindingsReady = true
      this.workflowBindingsLoading = false
      this.workflowBindingsSaving = false
      clearLegacyWorkflowBindings()
    },
    async bootstrapWorkflowBindings() {
      const legacyBindings = loadLegacyWorkflowBindings()

      if (typeof window === 'undefined') {
        this.workflowBindings = legacyBindings
        this.workflowBindingsReady = true
        return
      }

      if (!localStorage.getItem('asr_token')) {
        this.workflowBindings = defaultWorkflowBindings()
        this.workflowBindingsReady = true
        this.workflowBindingsLoading = false
        return
      }

      this.workflowBindingsLoading = true
      try {
        const result = await getCurrentUserWorkflowBindings()
        const remoteBindings = normalizeWorkflowBindings(result.data)

        if (!hasWorkflowBindingValue(remoteBindings) && hasWorkflowBindingValue(legacyBindings)) {
          await updateCurrentUserWorkflowBindings(legacyBindings)
          this.workflowBindings = legacyBindings
          clearLegacyWorkflowBindings()
        }
        else {
          this.workflowBindings = remoteBindings
          if (hasWorkflowBindingValue(remoteBindings))
            clearLegacyWorkflowBindings()
        }
      }
      catch {
        this.workflowBindings = hasWorkflowBindingValue(legacyBindings) ? legacyBindings : defaultWorkflowBindings()
      }
      finally {
        this.workflowBindingsLoading = false
        this.workflowBindingsReady = true
      }
    },
    async replaceWorkflowBindings(bindings: WorkflowBindings) {
      const previousBindings = this.workflowBindings
      this.workflowBindings = bindings
      this.workflowBindingsSaving = true

      try {
        await updateCurrentUserWorkflowBindings(bindings)
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

import type { WorkflowBindingKey, WorkflowPreviewItem } from '@/types/workflow'

import { computed } from 'vue'

import { useAppStore } from '@/stores/app'

export interface WorkflowBindingMessages {
  emptyLabel?: string
  unsetMessage: string
  missingMessage: (workflowId: number) => string
  readyMessage: (workflow: WorkflowPreviewItem) => string
}

export function useWorkflowBindingStatus(
  bindingKey: WorkflowBindingKey,
  catalog: {
    findWorkflow: (workflowId?: number | null) => WorkflowPreviewItem | null
    hasWorkflow: (workflowId?: number | null) => boolean
    labelForWorkflow: (workflowId?: number | null, emptyLabel?: string) => string
  },
  messages: WorkflowBindingMessages,
) {
  const appStore = useAppStore()

  const configuredWorkflowId = computed(() => appStore.workflowBindings[bindingKey])
  const configuredWorkflow = computed(() => catalog.findWorkflow(configuredWorkflowId.value))
  const configuredWorkflowMissing = computed(() => {
    const workflowId = configuredWorkflowId.value
    if (!workflowId)
      return false
    return !catalog.hasWorkflow(workflowId)
  })
  const configuredWorkflowLabel = computed(() => catalog.labelForWorkflow(configuredWorkflowId.value, messages.emptyLabel || '未配置默认工作流'))
  const configuredWorkflowNotice = computed(() => {
    if (!configuredWorkflowId.value)
      return messages.unsetMessage
    if (configuredWorkflowMissing.value)
      return messages.missingMessage(configuredWorkflowId.value)
    if (configuredWorkflow.value)
      return messages.readyMessage(configuredWorkflow.value)
    return messages.unsetMessage
  })

  function setConfiguredWorkflow(workflowId: number | null) {
    return appStore.setWorkflowBinding(bindingKey, workflowId)
  }

  return {
    configuredWorkflowId,
    configuredWorkflow,
    configuredWorkflowLabel,
    configuredWorkflowMissing,
    configuredWorkflowNotice,
    setConfiguredWorkflow,
  }
}

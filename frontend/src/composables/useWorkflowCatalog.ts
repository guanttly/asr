import type { MaybeRefOrGetter } from 'vue'

import type { ActiveWorkflowType, WorkflowCatalogItem, WorkflowPreviewItem } from '@/types/workflow'

import { computed, ref, toValue } from 'vue'

import { getWorkflows } from '@/api/workflow'

function toWorkflowPreview(item: WorkflowCatalogItem): WorkflowPreviewItem {
  return {
    id: item.id,
    label: item.name,
    description: item.description,
    owner_type: item.owner_type,
    workflow_type: item.workflow_type,
    source_kind: item.source_kind,
    target_kind: item.target_kind,
    is_legacy: item.is_legacy,
    validation_message: item.validation_message,
    nodes: item.nodes || [],
  }
}

export function useWorkflowCatalog(workflowTypeSource: MaybeRefOrGetter<ActiveWorkflowType>, limit = 200) {
  const loading = ref(false)
  const workflows = ref<WorkflowCatalogItem[]>([])

  const workflowOptions = computed(() => workflows.value.map(item => ({
    label: item.name,
    value: item.id,
  })))

  function findWorkflow(workflowId?: number | null) {
    if (!workflowId)
      return null

    const item = workflows.value.find(entry => entry.id === workflowId)
    return item ? toWorkflowPreview(item) : null
  }

  function hasWorkflow(workflowId?: number | null) {
    if (!workflowId)
      return false
    return workflows.value.some(item => item.id === workflowId)
  }

  function labelForWorkflow(workflowId?: number | null, emptyLabel = '-') {
    if (!workflowId)
      return emptyLabel
    return workflows.value.find(item => item.id === workflowId)?.name || `工作流 #${workflowId}`
  }

  async function loadWorkflows() {
    loading.value = true
    try {
      const workflowType = toValue(workflowTypeSource)
      const result = await getWorkflows({
        offset: 0,
        limit,
        workflow_type: workflowType,
        include_legacy: false,
      })
      workflows.value = result.data.items || []
      return workflows.value
    }
    finally {
      loading.value = false
    }
  }

  return {
    loading,
    workflows,
    workflowOptions,
    findWorkflow,
    hasWorkflow,
    labelForWorkflow,
    loadWorkflows,
  }
}

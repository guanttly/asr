<script setup lang="ts">
import type { AxiosError } from 'axios'
import type { WorkflowBindingKey } from '@/types/workflow'

import { useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import WorkflowSelectionPreview from '@/components/WorkflowSelectionPreview.vue'
import { useWorkflowBindingStatus } from '@/composables/useWorkflowBindingStatus'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'
import { useAppStore } from '@/stores/app'

interface SectionMeta {
  key: WorkflowBindingKey
  title: string
  route: string
  description: string
  emptyTitle: string
  emptyDescription: string
}

const router = useRouter()
const message = useMessage()
const appStore = useAppStore()
const loading = ref(false)
const realtimeCatalog = useWorkflowCatalog('realtime_transcription')
const batchCatalog = useWorkflowCatalog('batch_transcription')
const meetingCatalog = useWorkflowCatalog('meeting')

const realtimeBinding = useWorkflowBindingStatus('realtime', realtimeCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '实时语音识别当前未设置默认工作流，应用仍可使用，但不会自动触发对应后处理链路。',
  missingMessage: workflowId => `当前绑定的工作流 #${workflowId} 已不在可用列表中，通常表示它被下线或仍是 legacy 版本。请重新选择。`,
  readyMessage: workflow => `实时语音识别当前默认使用「${workflow.label}」，对应应用会自动复用这条配置。`,
})
const batchBinding = useWorkflowBindingStatus('batch', batchCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '批量转写当前未设置默认工作流，应用仍可使用，但不会自动触发对应后处理链路。',
  missingMessage: workflowId => `当前绑定的工作流 #${workflowId} 已不在可用列表中，通常表示它被下线或仍是 legacy 版本。请重新选择。`,
  readyMessage: workflow => `批量转写当前默认使用「${workflow.label}」，对应应用会自动复用这条配置。`,
})
const meetingBinding = useWorkflowBindingStatus('meeting', meetingCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '会议纪要当前未设置默认工作流，应用仍可使用，但不会自动触发对应后处理链路。',
  missingMessage: workflowId => `当前绑定的工作流 #${workflowId} 已不在可用列表中，通常表示它被下线或仍是 legacy 版本。请重新选择。`,
  readyMessage: workflow => `会议纪要当前默认使用「${workflow.label}」，对应应用会自动复用这条配置。`,
})

const sections: SectionMeta[] = [
  {
    key: 'realtime',
    title: '实时语音识别',
    route: '/realtime',
    description: '录音结束后自动套用这里配置的默认工作流，不再在实时界面单独选择。',
    emptyTitle: '未配置实时应用工作流',
    emptyDescription: '设置后，实时录音保存任务时会自动触发对应工作流。',
  },
  {
    key: 'batch',
    title: '批量转写',
    route: '/transcription',
    description: '上传文件或提交 URL 后，会统一使用这里配置的默认后处理工作流。',
    emptyTitle: '未配置批量转写工作流',
    emptyDescription: '设置后，批量任务创建时会自动携带对应工作流。',
  },
  {
    key: 'meeting',
    title: '会议纪要',
    route: '/meetings',
    description: '会议创建与摘要重新生成都会优先使用这里配置的默认会议工作流。',
    emptyTitle: '未配置会议纪要工作流',
    emptyDescription: '设置后，会议相关页面不再单独暴露工作流选择器。',
  },
]

const configuredCount = computed(() => Object.values(appStore.workflowBindings).filter(value => typeof value === 'number').length)
const bindingSaving = computed(() => appStore.workflowBindingsSaving)

function extractErrorMessage(error: unknown, fallback: string) {
  const responseMessage = (error as AxiosError<{ message?: string }>)?.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim())
    return responseMessage
  return fallback
}

function workflowOptionsFor(key: WorkflowBindingKey) {
  switch (key) {
    case 'realtime':
      return realtimeCatalog.workflowOptions.value
    case 'batch':
      return batchCatalog.workflowOptions.value
    default:
      return meetingCatalog.workflowOptions.value
  }
}

function selectedWorkflowFor(key: WorkflowBindingKey) {
  switch (key) {
    case 'realtime':
      return realtimeBinding.configuredWorkflow.value
    case 'batch':
      return batchBinding.configuredWorkflow.value
    default:
      return meetingBinding.configuredWorkflow.value
  }
}

function bindingMissing(key: WorkflowBindingKey) {
  switch (key) {
    case 'realtime':
      return realtimeBinding.configuredWorkflowMissing.value
    case 'batch':
      return batchBinding.configuredWorkflowMissing.value
    default:
      return meetingBinding.configuredWorkflowMissing.value
  }
}

function bindingNotice(section: SectionMeta) {
  switch (section.key) {
    case 'realtime':
      return realtimeBinding.configuredWorkflowNotice.value
    case 'batch':
      return batchBinding.configuredWorkflowNotice.value
    default:
      return meetingBinding.configuredWorkflowNotice.value
  }
}

async function handleBindingChange(key: WorkflowBindingKey, value: number | null) {
  const nextValue = typeof value === 'number' ? value : null
  try {
    switch (key) {
      case 'realtime':
        await realtimeBinding.setConfiguredWorkflow(nextValue)
        break
      case 'batch':
        await batchBinding.setConfiguredWorkflow(nextValue)
        break
      default:
        await meetingBinding.setConfiguredWorkflow(nextValue)
        break
    }
  }
  catch (error) {
    message.error(extractErrorMessage(error, '应用默认工作流保存失败'))
  }
}

async function loadWorkflowBuckets() {
  loading.value = true
  try {
    const results = await Promise.allSettled([
      realtimeCatalog.loadWorkflows(),
      batchCatalog.loadWorkflows(),
      meetingCatalog.loadWorkflows(),
    ])

    if (results.some(result => result.status === 'rejected'))
      message.warning('部分工作流列表加载失败，请稍后刷新重试')
  }
  catch {
    message.error('应用配置加载失败')
  }
  finally {
    loading.value = false
  }
}

onMounted(loadWorkflowBuckets)
</script>

<template>
  <div class="grid gap-5">
    <NCard class="card-main">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <div class="text-sm font-700 text-ink">
            应用配置
          </div>
          <div class="mt-1 text-sm leading-6 text-slate">
            这里统一决定每个应用默认绑定哪条工作流。配置会保存到当前账号，应用页只负责执行，不再在页面内部混入工作流配置。
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3 sm:flex sm:items-center">
          <div class="subtle-panel m-0 min-w-28">
            <div class="text-xs text-slate/70">
              已配置应用
            </div>
            <div class="mt-1.5 text-lg font-700 text-ink">
              {{ configuredCount }}/3
            </div>
          </div>
          <NButton quaternary @click="router.push('/workflows')">
            管理工作流
          </NButton>
        </div>
      </div>
    </NCard>

    <div class="grid gap-5 xl:grid-cols-3">
      <NCard v-for="section in sections" :key="section.key" class="card-main" :loading="loading || appStore.workflowBindingsLoading">
        <div class="flex items-start justify-between gap-3">
          <div>
            <div class="text-sm font-700 text-ink">
              {{ section.title }}
            </div>
            <div class="mt-1 text-xs leading-6 text-slate">
              {{ section.description }}
            </div>
          </div>
          <NButton text size="small" @click="router.push(section.route)">
            打开应用
          </NButton>
        </div>

        <div class="mt-4 grid gap-3">
          <NSelect
            :value="appStore.workflowBindings[section.key]"
            clearable
            filterable
            :loading="bindingSaving"
            :options="workflowOptionsFor(section.key)"
            placeholder="选择默认工作流"
            :disabled="loading || appStore.workflowBindingsLoading || bindingSaving"
            @update:value="(value) => void handleBindingChange(section.key, value as number | null)"
          />

          <div class="rounded-2 border px-3 py-2 text-xs leading-6" :class="bindingMissing(section.key) ? 'border-amber-200 bg-amber-50 text-amber-700' : 'border-transparent bg-mist/70 text-slate'">
            {{ bindingNotice(section) }}
          </div>

          <WorkflowSelectionPreview
            :workflow="selectedWorkflowFor(section.key)"
            :loading="loading"
            :empty-title="section.emptyTitle"
            :empty-description="section.emptyDescription"
          />
        </div>
      </NCard>
    </div>
  </div>
</template>

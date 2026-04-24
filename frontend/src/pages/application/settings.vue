<script setup lang="ts">
import type { AxiosError } from 'axios'
import type { VoiceControlPayload } from '@/api/appSettings'
import type { ProductFeatureKey } from '@/constants/product'

import type { WorkflowBindingKey } from '@/types/workflow'
import { useMessage } from 'naive-ui'
import { computed, onMounted, reactive, ref } from 'vue'

import { useRouter } from 'vue-router'
import { getVoiceControl, updateVoiceControl } from '@/api/appSettings'
import WorkflowSelectionPreview from '@/components/WorkflowSelectionPreview.vue'
import { useWorkflowBindingStatus } from '@/composables/useWorkflowBindingStatus'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'
import { PRODUCT_FEATURE_KEYS } from '@/constants/product'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'
import { WORKFLOW_BINDING_KEYS, WORKFLOW_TYPES } from '@/types/workflow'

interface SectionMeta {
  key: WorkflowBindingKey
  requiredFeature?: ProductFeatureKey
  title: string
  route: string
  actionLabel?: string
  description: string
  emptyTitle: string
  emptyDescription: string
}

const router = useRouter()
const message = useMessage()
const appStore = useAppStore()
const loading = ref(false)
const realtimeCatalog = useWorkflowCatalog(WORKFLOW_TYPES.REALTIME)
const batchCatalog = useWorkflowCatalog(WORKFLOW_TYPES.BATCH)
const meetingCatalog = useWorkflowCatalog(WORKFLOW_TYPES.MEETING)
const voiceCatalog = useWorkflowCatalog(WORKFLOW_TYPES.VOICE_CONTROL)

const realtimeBinding = useWorkflowBindingStatus(WORKFLOW_BINDING_KEYS.REALTIME, realtimeCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '实时语音识别当前未设置默认工作流，应用仍可使用，但不会自动触发对应后处理链路。',
  missingMessage: workflowId => `当前绑定的工作流 #${workflowId} 已不在可用列表中，通常表示它被下线或仍是 legacy 版本。请重新选择。`,
  readyMessage: workflow => `实时语音识别当前默认使用「${workflow.label}」，对应应用会自动复用这条配置。`,
})
const batchBinding = useWorkflowBindingStatus(WORKFLOW_BINDING_KEYS.BATCH, batchCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '批量转写当前未设置默认工作流，应用仍可使用，但不会自动触发对应后处理链路。',
  missingMessage: workflowId => `当前绑定的工作流 #${workflowId} 已不在可用列表中，通常表示它被下线或仍是 legacy 版本。请重新选择。`,
  readyMessage: workflow => `批量转写当前默认使用「${workflow.label}」，对应应用会自动复用这条配置。`,
})
const meetingBinding = useWorkflowBindingStatus(WORKFLOW_BINDING_KEYS.MEETING, meetingCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '会议纪要当前未设置默认工作流，应用仍可使用，但不会自动触发对应后处理链路。',
  missingMessage: workflowId => `当前绑定的工作流 #${workflowId} 已不在可用列表中，通常表示它被下线或仍是 legacy 版本。请重新选择。`,
  readyMessage: workflow => `会议纪要当前默认使用「${workflow.label}」，对应应用会自动复用这条配置。`,
})
const voiceBinding = useWorkflowBindingStatus(WORKFLOW_BINDING_KEYS.VOICE_CONTROL, voiceCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '终端语音控制当前未设置默认工作流，命中触发词后将无法进入统一的 workflow 指令识别链路。',
  missingMessage: workflowId => `当前绑定的语音控制工作流 #${workflowId} 已不在可用列表中，请重新选择。`,
  readyMessage: workflow => `终端语音控制当前默认使用「${workflow.label}」，voice_wake 与 voice_intent 节点会共同完成唤醒和命令识别。`,
})

const sections: SectionMeta[] = [
  {
    key: WORKFLOW_BINDING_KEYS.REALTIME,
    title: '实时语音识别',
    route: '/realtime',
    description: '录音结束后自动套用这里配置的默认工作流，不再在实时界面单独选择。',
    emptyTitle: '未配置实时应用工作流',
    emptyDescription: '设置后，实时录音保存任务时会自动触发对应工作流。',
  },
  {
    key: WORKFLOW_BINDING_KEYS.BATCH,
    title: '批量转写',
    route: '/transcription',
    description: '上传文件或提交 URL 后，会统一使用这里配置的默认后处理工作流。',
    emptyTitle: '未配置批量转写工作流',
    emptyDescription: '设置后，批量任务创建时会自动携带对应工作流。',
  },
  {
    key: WORKFLOW_BINDING_KEYS.MEETING,
    requiredFeature: PRODUCT_FEATURE_KEYS.MEETING,
    title: '会议纪要',
    route: '/meetings',
    description: '会议创建与摘要重新生成都会优先使用这里配置的默认会议工作流。',
    emptyTitle: '未配置会议纪要工作流',
    emptyDescription: '设置后，会议相关页面不再单独暴露工作流选择器。',
  },
  {
    key: WORKFLOW_BINDING_KEYS.VOICE_CONTROL,
    requiredFeature: PRODUCT_FEATURE_KEYS.VOICE_CONTROL,
    title: '终端语音控制',
    route: '/workflows',
    actionLabel: '管理工作流',
    description: '桌面端每段转写都会执行这里绑定的语音控制工作流，由 voice_wake 节点判断唤醒，再由 voice_intent 节点输出结构化控制结果。',
    emptyTitle: '未配置语音控制工作流',
    emptyDescription: '设置后，终端语音控制将统一走 workflow 链路，不再使用额外分类接口。',
  },
]

const availableSections = computed(() => sections.filter(section => !section.requiredFeature || appStore.hasCapability(section.requiredFeature)))

const configuredCount = computed(() => availableSections.value.filter(section => typeof appStore.workflowBindings[section.key] === 'number').length)
const bindingSaving = computed(() => appStore.workflowBindingsSaving)

function extractErrorMessage(error: unknown, fallback: string) {
  const responseMessage = (error as AxiosError<{ message?: string }>)?.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim())
    return responseMessage
  return fallback
}

function workflowOptionsFor(key: WorkflowBindingKey) {
  switch (key) {
    case WORKFLOW_BINDING_KEYS.REALTIME:
      return realtimeCatalog.workflowOptions.value
    case WORKFLOW_BINDING_KEYS.BATCH:
      return batchCatalog.workflowOptions.value
    case WORKFLOW_BINDING_KEYS.VOICE_CONTROL:
      return voiceCatalog.workflowOptions.value
    default:
      return meetingCatalog.workflowOptions.value
  }
}

function selectedWorkflowFor(key: WorkflowBindingKey) {
  switch (key) {
    case WORKFLOW_BINDING_KEYS.REALTIME:
      return realtimeBinding.configuredWorkflow.value
    case WORKFLOW_BINDING_KEYS.BATCH:
      return batchBinding.configuredWorkflow.value
    case WORKFLOW_BINDING_KEYS.VOICE_CONTROL:
      return voiceBinding.configuredWorkflow.value
    default:
      return meetingBinding.configuredWorkflow.value
  }
}

function bindingMissing(key: WorkflowBindingKey) {
  switch (key) {
    case WORKFLOW_BINDING_KEYS.REALTIME:
      return realtimeBinding.configuredWorkflowMissing.value
    case WORKFLOW_BINDING_KEYS.BATCH:
      return batchBinding.configuredWorkflowMissing.value
    case WORKFLOW_BINDING_KEYS.VOICE_CONTROL:
      return voiceBinding.configuredWorkflowMissing.value
    default:
      return meetingBinding.configuredWorkflowMissing.value
  }
}

function bindingNotice(section: SectionMeta) {
  switch (section.key) {
    case WORKFLOW_BINDING_KEYS.REALTIME:
      return realtimeBinding.configuredWorkflowNotice.value
    case WORKFLOW_BINDING_KEYS.BATCH:
      return batchBinding.configuredWorkflowNotice.value
    case WORKFLOW_BINDING_KEYS.VOICE_CONTROL:
      return voiceBinding.configuredWorkflowNotice.value
    default:
      return meetingBinding.configuredWorkflowNotice.value
  }
}

async function handleBindingChange(key: WorkflowBindingKey, value: number | null) {
  const nextValue = typeof value === 'number' ? value : null
  try {
    switch (key) {
      case WORKFLOW_BINDING_KEYS.REALTIME:
        await realtimeBinding.setConfiguredWorkflow(nextValue)
        break
      case WORKFLOW_BINDING_KEYS.BATCH:
        await batchBinding.setConfiguredWorkflow(nextValue)
        break
      case WORKFLOW_BINDING_KEYS.VOICE_CONTROL:
        await voiceBinding.setConfiguredWorkflow(nextValue)
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
    const tasks: Array<Promise<unknown>> = [
		realtimeCatalog.loadWorkflows(),
		batchCatalog.loadWorkflows(),
	]
    if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.MEETING))
		tasks.push(meetingCatalog.loadWorkflows())
    if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICE_CONTROL))
		tasks.push(voiceCatalog.loadWorkflows())
    const results = await Promise.allSettled(tasks)

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

// ---- Voice control config (admin) ----
const userStore = useUserStore()
const isAdmin = computed(() => userStore.profile?.role === 'admin')
const voiceLoading = ref(false)
const voiceSaving = ref(false)
const voiceForm = reactive<VoiceControlPayload>({
  command_timeout_ms: 10000,
  enabled: true,
})
async function loadVoiceControl() {
  voiceLoading.value = true
  try {
    const result = await getVoiceControl() as unknown as { data?: VoiceControlPayload }
    const data = result?.data
    if (data) {
      voiceForm.command_timeout_ms = data.command_timeout_ms || 10000
      voiceForm.enabled = data.enabled !== false
    }
  }
  catch (error) {
    message.error(extractErrorMessage(error, '加载语音控制配置失败'))
  }
  finally {
    voiceLoading.value = false
  }
}
async function saveVoiceControl() {
  voiceSaving.value = true
  try {
    await updateVoiceControl({
      command_timeout_ms: Math.max(2000, Math.min(60000, Math.round(voiceForm.command_timeout_ms))),
      enabled: voiceForm.enabled,
    })
    message.success('语音控制配置已保存')
    await loadVoiceControl()
  }
  catch (error) {
    message.error(extractErrorMessage(error, '保存语音控制配置失败'))
  }
  finally {
    voiceSaving.value = false
  }
}
onMounted(() => {
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICE_CONTROL))
    void loadVoiceControl()
})
</script>

<template>
  <div class="h-full min-h-0 overflow-y-auto pr-1">
    <div class="grid gap-5 pb-2">
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
                {{ configuredCount }}/{{ availableSections.length }}
              </div>
            </div>
            <NButton quaternary @click="router.push('/workflows')">
              管理工作流
            </NButton>
          </div>
        </div>
      </NCard>

      <div class="grid gap-5 xl:grid-cols-2 2xl:grid-cols-4">
        <NCard v-for="section in availableSections" :key="section.key" class="card-main" :loading="loading || appStore.workflowBindingsLoading">
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
              {{ section.actionLabel || '打开应用' }}
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

      <NCard v-if="appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICE_CONTROL)" class="card-main" :loading="voiceLoading">
        <div class="flex items-start justify-between gap-4">
          <div>
            <div class="text-sm font-700 text-ink">
              语音控制运行参数
            </div>
            <div class="mt-1 text-xs leading-6 text-slate">
              这里维护的是桌面端语音控制的全局运行参数，不是另一条工作流。上方卡片负责绑定默认语音控制工作流；这里仅保留等待时长和启停状态。唤醒词统一由 voice_wake 节点维护。
            </div>
          </div>
          <NTag :type="voiceForm.enabled ? 'success' : 'default'" round size="small">
            {{ voiceForm.enabled ? '已启用' : '已停用' }}
          </NTag>
        </div>
        <div class="mt-4 grid gap-4 lg:grid-cols-2">
          <div>
            <div class="text-xs text-slate/80 mb-1">
              等待指令时长（毫秒）
            </div>
            <NInputNumber
              v-model:value="voiceForm.command_timeout_ms"
              :min="2000"
              :max="60000"
              :step="1000"
              :disabled="!isAdmin"
              class="w-full"
            />
          </div>
          <div>
            <div class="text-xs text-slate/80 mb-1">
              启用语音控制
            </div>
            <NSwitch v-model:value="voiceForm.enabled" :disabled="!isAdmin" />
          </div>
        </div>
        <div class="mt-4 flex justify-end gap-2">
          <NButton :disabled="voiceLoading" @click="loadVoiceControl">
            重新加载
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="voiceSaving" :disabled="!isAdmin" @click="saveVoiceControl">
            保存
          </NButton>
        </div>
        <div v-if="!isAdmin" class="mt-2 text-xs text-amber-600">
          当前账号没有修改权限，仅管理员可调整等待时长与启停状态。
        </div>
      </NCard>
    </div>
  </div>
</template>

<script setup lang="ts">
import MarkdownIt from 'markdown-it'
import { useMessage } from 'naive-ui'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { deleteMeeting, getMeetingDetail, regenerateMeetingSummary } from '@/api/meeting'
import WorkflowSelectionPreview from '@/components/WorkflowSelectionPreview.vue'
import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'
import { useWorkflowBindingStatus } from '@/composables/useWorkflowBindingStatus'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'

interface TranscriptItem {
  speaker_label: string
  text: string
  start_time: number
  end_time: number
}

interface SummaryItem {
  content: string
  model_version: string
  created_at?: string
}

interface MeetingDetail {
  id: number
  title: string
  status: string
  duration: number
  workflow_id?: number | null
  sync_fail_count?: number
  last_sync_error?: string
  last_sync_at?: string
  next_sync_at?: string
  created_at?: string
  updated_at?: string
  transcripts: TranscriptItem[]
  summary?: SummaryItem | null
}

const route = useRoute()
const router = useRouter()
const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()
const { subscribe: subscribeBusinessTopic } = useBusinessSocket()
const summaryMarkdown = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})
const meetingWorkflowCatalog = useWorkflowCatalog('meeting')
const {
  configuredWorkflowId,
  configuredWorkflow,
  configuredWorkflowMissing,
  configuredWorkflowNotice,
} = useWorkflowBindingStatus('meeting', meetingWorkflowCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '当前未设置会议默认工作流，请前往应用配置页统一配置。',
  missingMessage: workflowId => `应用配置中的会议工作流 #${workflowId} 当前不可用，请前往应用配置页重新选择。`,
  readyMessage: () => '会议摘要生成会优先使用应用配置页中绑定的默认工作流。',
})
const loading = ref(false)
const summaryLoading = ref(false)
const deleting = ref(false)
const detail = ref<MeetingDetail | null>(null)
const nowTimestamp = ref(Date.now())
let clockTimer: number | null = null
let stopBusinessSubscription: (() => void) | null = null

const transcript = computed(() => detail.value?.transcripts || [])
const renderedSummaryHtml = computed(() => {
  const content = detail.value?.summary?.content?.trim()
  if (!content)
    return ''
  return summaryMarkdown.render(content)
})
const boundWorkflowMissing = computed(() => {
  const workflowId = detail.value?.workflow_id
  if (!workflowId)
    return false
  return !meetingWorkflowCatalog.hasWorkflow(workflowId)
})
const effectiveWorkflowId = computed(() => configuredWorkflowId.value || detail.value?.workflow_id || null)
const summaryWorkflowNotice = computed(() => {
  if (configuredWorkflowId.value && configuredWorkflowMissing.value)
    return configuredWorkflowNotice.value
  if (!configuredWorkflowId.value && boundWorkflowMissing.value && detail.value?.workflow_id)
    return `当前会议绑定的工作流 #${detail.value.workflow_id} 不在可用列表中，通常表示它仍是待升级的 legacy 工作流。请先在应用配置页重新指定会议工作流。`
  if (!configuredWorkflowId.value && detail.value?.workflow_id)
    return '当前未设置会议默认工作流，本页会继续沿用这条会议已绑定的工作流生成摘要。'
  return configuredWorkflowNotice.value
})
const selectedWorkflow = computed(() => {
  return configuredWorkflow.value || meetingWorkflowCatalog.findWorkflow(effectiveWorkflowId.value)
})

const detailStatusMeta = computed(() => getMeetingStatusMeta(detail.value?.status))
const detailElapsedText = computed(() => formatElapsedDuration(detail.value))
const canDeleteMeeting = computed(() => ['uploaded', 'completed', 'failed'].includes(detail.value?.status || ''))
const detailFailureReason = computed(() => detail.value?.last_sync_error?.trim() || '')
const detailFailureHint = computed(() => {
  if (detail.value?.status !== 'failed')
    return ''
  if (detailFailureReason.value)
    return '请根据上面的错误信息检查上游转写任务或当前会议工作流配置后再重试。'
  return '当前会议处理已失败，但系统没有返回更具体的错误文本。建议先检查对应 ASR 任务和会议工作流执行记录。'
})

function formatTime(value: number) {
  const minute = Math.floor(value / 60).toString().padStart(2, '0')
  const second = Math.floor(value % 60).toString().padStart(2, '0')
  return `${minute}:${second}`
}

function getMeetingStatusMeta(status?: string) {
  const map: Record<string, { label: string, toneClass: string, pillClass: string, description: string }> = {
    uploaded: {
      label: '待转写',
      toneClass: 'text-amber-700',
      pillClass: 'bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-200',
      description: '音频已上传，等待开始转写。',
    },
    pending: {
      label: '待处理',
      toneClass: 'text-amber-700',
      pillClass: 'bg-amber-50 text-amber-700 ring-1 ring-inset ring-amber-200',
      description: '任务已创建，尚未进入实际处理。',
    },
    processing: {
      label: '转写中',
      toneClass: 'text-sky-700',
      pillClass: 'bg-sky-50 text-sky-700 ring-1 ring-inset ring-sky-200',
      description: '会议正在进行转写或后续处理。',
    },
    completed: {
      label: '已完成',
      toneClass: 'text-emerald-700',
      pillClass: 'bg-emerald-50 text-emerald-700 ring-1 ring-inset ring-emerald-200',
      description: '逐字稿和可用摘要内容已生成完成。',
    },
    failed: {
      label: '失败',
      toneClass: 'text-rose-700',
      pillClass: 'bg-rose-50 text-rose-700 ring-1 ring-inset ring-rose-200',
      description: '本次会议处理已中断，请查看下方失败原因。',
    },
  }
  return map[status || ''] || {
    label: status || '-',
    toneClass: 'text-slate-700',
    pillClass: 'bg-slate-100 text-slate-700 ring-1 ring-inset ring-slate-200',
    description: '当前状态暂未归类。',
  }
}

function formatAudioDuration(value?: number) {
  if (!value || value <= 0)
    return '-'

  const totalSeconds = Math.max(0, Math.round(value))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60

  if (hours > 0)
    return `${hours}时${minutes}分${seconds}秒`
  if (minutes > 0)
    return `${minutes}分${seconds}秒`
  return `${seconds}秒`
}

function formatDateTime(value?: string) {
  if (!value)
    return '-'

  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value

  return date.toLocaleString('zh-CN', {
    hour12: false,
  })
}

function parseDateValue(value?: string) {
  if (!value)
    return 0
  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) ? 0 : timestamp
}

function formatElapsedDuration(meeting?: MeetingDetail | null) {
  if (!meeting)
    return '-'
  const start = parseDateValue(meeting.created_at)
  if (!start)
    return '-'
  const end = meeting.status === 'processing'
    ? nowTimestamp.value
    : parseDateValue(meeting.updated_at) || nowTimestamp.value
  const totalSeconds = Math.max(0, Math.floor((end - start) / 1000))
  const hours = Math.floor(totalSeconds / 3600)
  const minutes = Math.floor((totalSeconds % 3600) / 60)
  const seconds = totalSeconds % 60
  if (hours > 0)
    return `${hours}时${minutes}分${seconds}秒`
  if (minutes > 0)
    return `${minutes}分${seconds}秒`
  return `${seconds}秒`
}

async function loadWorkflows() {
  try {
    await meetingWorkflowCatalog.loadWorkflows()
  }
  catch {
    message.error('工作流列表加载失败')
  }
}

async function loadDetail() {
  loading.value = true
  try {
    const result = await getMeetingDetail(String(route.params.id))
    detail.value = result.data
  }
  catch {
    message.error('会议详情加载失败')
  }
  finally {
    loading.value = false
  }
}

async function handleGenerateSummary() {
  if (!detail.value)
    return
  const workflowId = effectiveWorkflowId.value
  if (!workflowId) {
    message.warning('请先前往应用配置页设置会议摘要工作流')
    return
  }
  if (configuredWorkflowId.value && configuredWorkflowMissing.value) {
    message.warning('应用配置中的会议工作流当前不可用，请先调整配置')
    return
  }
  if (!configuredWorkflowId.value && boundWorkflowMissing.value) {
    message.warning('当前会议绑定的工作流待升级，请先到应用配置页重新选择')
    return
  }

  summaryLoading.value = true
  try {
    const result = await regenerateMeetingSummary(detail.value.id, {
      workflow_id: workflowId,
    })
    detail.value = result.data
    message.success(result.data.summary ? '会议摘要已更新' : '工作流执行完成，但未生成会议摘要')
  }
  catch {
    message.error('会议摘要生成失败')
  }
  finally {
    summaryLoading.value = false
  }
}

async function handleDeleteMeeting() {
  if (!detail.value || !canDeleteMeeting.value || deleting.value)
    return

  const confirmed = await confirmDelete({
    entityType: '会议记录',
    entityName: detail.value.title || `#${detail.value.id}`,
    description: '删除后，这条会议记录、逐字稿和摘要会一并移除。',
  })
  if (!confirmed)
    return

  deleting.value = true
  try {
    await deleteMeeting(detail.value.id)
    message.success('会议记录已删除')
    router.push('/meetings')
  }
  catch {
    message.error('会议记录删除失败')
  }
  finally {
    deleting.value = false
  }
}

onMounted(async () => {
  await Promise.all([loadDetail(), loadWorkflows()])
  clockTimer = window.setInterval(() => {
    nowTimestamp.value = Date.now()
  }, 1000)

  stopBusinessSubscription = subscribeBusinessTopic(['meeting.updated'], (event) => {
    const payload = event.payload as { meeting?: MeetingDetail } | undefined
    if (!payload?.meeting?.id || String(payload.meeting.id) !== String(route.params.id))
      return
    void loadDetail()
  })
})

onBeforeUnmount(() => {
  stopBusinessSubscription?.()
  stopBusinessSubscription = null
  if (clockTimer != null)
    window.clearInterval(clockTimer)
})
</script>

<template>
  <div class="grid gap-5">
    <div class="flex items-center justify-end gap-2">
      <NButton quaternary @click="router.push('/meetings')">
        返回列表
      </NButton>
      <NButton v-if="canDeleteMeeting" type="error" secondary :loading="deleting" @click="handleDeleteMeeting">
        删除会议
      </NButton>
    </div>

    <div class="grid grid-cols-2 gap-3 md:grid-cols-5">
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">
          会议 ID
        </div>
        <div class="mt-1.5 text-sm font-700 text-ink">
          #{{ route.params.id }}
        </div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">
          状态
        </div>
        <div class="mt-2 inline-flex items-center rounded-full px-3 py-1 text-sm font-700" :class="detailStatusMeta.pillClass">
          {{ detailStatusMeta.label }}
        </div>
        <div class="mt-2 text-xs leading-6" :class="detailStatusMeta.toneClass">
          {{ detailStatusMeta.description }}
        </div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">
          音频时长
        </div>
        <div class="mt-1.5 text-sm font-700 text-ink">
          {{ formatAudioDuration(detail?.duration) }}
        </div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">
          处理耗时
        </div>
        <div class="mt-1.5 text-sm font-700 text-ink">
          {{ detailElapsedText }}
        </div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">
          片段数
        </div>
        <div class="mt-1.5 text-sm font-700 text-ink">
          {{ transcript.length }}
        </div>
      </div>
    </div>

    <NCard class="card-main" :loading="loading">
      <div v-if="detail?.status === 'failed'" class="mb-4 rounded-2.5 border border-rose-200 bg-rose-50 px-4 py-3.5 text-rose-800 shadow-[0_10px_30px_rgba(190,24,93,0.08)]">
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div>
            <div class="text-sm font-700">
              会议处理失败
            </div>
            <div class="mt-1 text-sm leading-6 text-rose-700">
              {{ detailFailureHint }}
            </div>
          </div>
          <div class="rounded-full bg-white/80 px-3 py-1 text-xs font-700 text-rose-700 ring-1 ring-inset ring-rose-200">
            失败 {{ detail?.sync_fail_count || 0 }} 次
          </div>
        </div>
        <div class="mt-3 rounded-2 bg-white/85 px-3 py-3 text-sm leading-6 text-rose-900 ring-1 ring-inset ring-rose-100">
          {{ detailFailureReason || '当前没有返回明确的失败原因。' }}
        </div>
        <div class="mt-3 flex flex-wrap gap-x-5 gap-y-1 text-xs text-rose-700">
          <span>上次失败时间：{{ formatDateTime(detail?.last_sync_at) }}</span>
          <span>下次重试时间：{{ formatDateTime(detail?.next_sync_at) }}</span>
        </div>
      </div>

      <div class="mb-4 grid gap-4 rounded-2.5 bg-[#fbfdff] p-4 lg:grid-cols-[minmax(0,1fr)_auto]">
        <div class="grid gap-3">
          <div>
            <div class="text-sm font-600 text-ink">
              会议摘要工作流
            </div>
            <div class="mt-1 text-xs text-slate">
              会议详情页只负责执行摘要生成，具体绑定关系统一在应用配置页维护。
            </div>
          </div>
          <div class="rounded-2 border px-3 py-2 text-xs leading-6" :class="configuredWorkflowMissing || boundWorkflowMissing ? 'border-amber-200 bg-amber-50 text-amber-700' : 'border-transparent bg-mist/70 text-slate'">
            {{ summaryWorkflowNotice }}
          </div>
        </div>
        <div class="flex items-start justify-end gap-2">
          <NButton quaternary @click="router.push('/workflows/application-settings')">
            应用配置
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="summaryLoading" :disabled="transcript.length === 0" @click="handleGenerateSummary">
            {{ detail?.summary ? '重新生成摘要' : '生成摘要' }}
          </NButton>
        </div>
      </div>

      <WorkflowSelectionPreview
        :workflow="selectedWorkflow"
        :loading="meetingWorkflowCatalog.loading.value"
        empty-title="未配置会议默认工作流"
        empty-description="前往应用配置页设置后，这里会展示会议应用统一使用的摘要生成链路。"
      />

      <NTabs type="line" animated>
        <NTabPane name="transcript" tab="逐字稿">
          <div class="grid gap-4">
            <div v-for="(item, index) in transcript" :key="`${index}-${item.start_time}`" class="subtle-panel">
              <div class="flex items-center justify-between">
                <div class="font-600 text-ink">
                  {{ item.speaker_label }}
                </div>
                <div class="text-xs text-slate/70">
                  {{ formatTime(item.start_time) }} - {{ formatTime(item.end_time) }}
                </div>
              </div>
              <div class="mt-3 text-sm leading-6 text-ink">
                {{ item.text }}
              </div>
            </div>

            <NEmpty v-if="!loading && transcript.length === 0" description="当前会议还没有逐字稿内容" class="empty-shell" />
          </div>
        </NTabPane>
        <NTabPane name="summary" tab="会议摘要">
          <div class="subtle-panel">
            <div class="font-600 text-ink">
              核心内容
            </div>
            <div
              v-if="renderedSummaryHtml"
              class="summary-markdown mt-3 leading-7 text-slate"
              v-html="renderedSummaryHtml"
            />
            <p v-else class="mt-3 whitespace-pre-line leading-7 text-slate">
              当前会议还没有生成摘要。
            </p>
            <div v-if="detail?.summary" class="mt-4 text-xs text-slate">
              模型版本：{{ detail.summary.model_version }}
            </div>
          </div>
        </NTabPane>
      </NTabs>
    </NCard>
  </div>
</template>

<style scoped>
.summary-markdown :deep(h1),
.summary-markdown :deep(h2),
.summary-markdown :deep(h3),
.summary-markdown :deep(h4) {
  margin: 1.1em 0 0.55em;
  color: #16202c;
  font-weight: 700;
  line-height: 1.4;
}

.summary-markdown :deep(h1) {
  font-size: 1.2rem;
}

.summary-markdown :deep(h2) {
  font-size: 1.08rem;
}

.summary-markdown :deep(h3),
.summary-markdown :deep(h4) {
  font-size: 1rem;
}

.summary-markdown :deep(p),
.summary-markdown :deep(ul),
.summary-markdown :deep(ol),
.summary-markdown :deep(blockquote),
.summary-markdown :deep(pre) {
  margin: 0.7em 0;
}

.summary-markdown :deep(ul),
.summary-markdown :deep(ol) {
  padding-left: 1.35rem;
}

.summary-markdown :deep(li) {
  margin: 0.28em 0;
}

.summary-markdown :deep(blockquote) {
  border-left: 3px solid rgba(15, 118, 110, 0.24);
  background: rgba(15, 118, 110, 0.06);
  padding: 0.8rem 1rem;
  border-radius: 0.85rem;
}

.summary-markdown :deep(code) {
  border-radius: 0.4rem;
  background: rgba(148, 163, 184, 0.15);
  padding: 0.12rem 0.35rem;
  font-size: 0.92em;
}

.summary-markdown :deep(pre) {
  overflow-x: auto;
  border-radius: 0.9rem;
  background: #f8fafc;
  padding: 0.9rem 1rem;
}

.summary-markdown :deep(pre code) {
  background: transparent;
  padding: 0;
}

.summary-markdown :deep(a) {
  color: #0f766e;
  text-decoration: underline;
  text-underline-offset: 2px;
}

.summary-markdown :deep(hr) {
  margin: 1.1rem 0;
  border: 0;
  border-top: 1px solid rgba(148, 163, 184, 0.28);
}
</style>

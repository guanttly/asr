<script setup lang="ts">
import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { clearDashboardRetryHistory, deleteDashboardRetryHistoryItem, getDashboardOverview, retryDashboardPostProcessTasks, syncDashboardTask } from '@/api/dashboard'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'
import { PRODUCT_FEATURE_KEYS } from '@/constants/product'
import { useAppStore } from '@/stores/app'

interface SyncAlertItem {
  task_id: number
  external_task_id: string
  meeting_id?: number
  alert_reason: string
  status: string
  post_process_status: string
  post_process_error?: string
  sync_fail_count: number
  last_sync_error?: string
  last_sync_at?: string
  next_sync_at?: string
  updated_at: string
}

interface RetryPostProcessItem {
  task_id: number
  external_task_id: string
  meeting_id?: number
  outcome: string
  post_process_status: string
  error_message?: string
}

interface RetryPostProcessResult {
  limit: number
  requested_task_count: number
  scanned: number
  updated: number
  failed: number
  created_at?: string
  items: RetryPostProcessItem[]
}

interface DashboardOverview {
  pending_count: number
  processing_count: number
  completed_count: number
  failed_count: number
  post_process_pending_count: number
  post_process_processing_count: number
  post_process_completed_count: number
  post_process_failed_count: number
  repeated_failure_count: number
  latest_sync_at?: string
  latest_retry_result?: RetryPostProcessResult
  retry_history?: RetryPostProcessResult[]
  alerts: SyncAlertItem[]
}

const message = useMessage()
const route = useRoute()
const router = useRouter()
const appStore = useAppStore()
const confirmDelete = useDeleteConfirmDialog()
const loading = ref(false)
const bulkRetrying = ref(false)
const clearingRetryHistory = ref(false)
const deletingRetryHistoryItem = ref(false)
const deletingRetryHistoryKey = ref('')
const syncingIds = ref<number[]>([])
const overview = ref<DashboardOverview | null>(null)
const alertReasonFilter = ref<'all' | 'repeated_sync_failure' | 'post_process_failed'>('all')
const retryableOnly = ref(false)
const lastRetryResult = ref<RetryPostProcessResult | null>(null)
const retryHistory = ref<RetryPostProcessResult[]>([])
const retryLimit = ref(20)
const retryHistoryFilter = ref<'all' | 'recovered' | 'failed'>('all')
const retryResultFilter = ref<'all' | 'recovered' | 'failed'>('all')
const showAllRetryResults = ref(false)
const selectedRetryHistoryKey = ref('')

const retryHistoryFilterValues = ['all', 'recovered', 'failed'] as const
const retryResultFilterValues = ['all', 'recovered', 'failed'] as const

const metrics = computed(() => {
  const data = overview.value
  return [
    { label: '批量待处理', value: String(data?.pending_count ?? 0), hint: '等待进入同步轮询' },
    { label: '批量处理中', value: String(data?.processing_count ?? 0), hint: '正在等待上游完成' },
    { label: 'ASR 已完成', value: String(data?.completed_count ?? 0), hint: '上游识别已经结束' },
    { label: '回流完成', value: String(data?.post_process_completed_count ?? 0), hint: '会议与摘要已落库' },
    { label: '后处理中', value: String((data?.post_process_pending_count ?? 0) + (data?.post_process_processing_count ?? 0)), hint: '等待或正在生成会议数据' },
    { label: '后处理失败', value: String(data?.post_process_failed_count ?? 0), hint: '需要排查下游处理错误' },
    { label: '重试告警', value: String(data?.repeated_failure_count ?? 0), hint: '连续失败超过阈值' },
  ]
})

const alertReasonOptions = [
  { label: '全部告警', value: 'all' },
  { label: '同步重试过多', value: 'repeated_sync_failure' },
  { label: '后处理失败', value: 'post_process_failed' },
]

const retryResultFilterOptions = [
  { label: '全部结果', value: 'all' },
  { label: '只看恢复项', value: 'recovered' },
  { label: '只看失败项', value: 'failed' },
]

const retryHistoryFilterOptions = [
  { label: '全部批次', value: 'all' },
  { label: '有恢复批次', value: 'recovered' },
  { label: '仍失败批次', value: 'failed' },
]

const filteredRetryHistory = computed(() => {
  const items = [...retryHistory.value].sort((left, right) => getRetryHistoryTimestamp(right) - getRetryHistoryTimestamp(left))
  switch (retryHistoryFilter.value) {
    case 'recovered':
      return items.filter(item => item.updated > 0)
    case 'failed':
      return items.filter(item => item.failed > 0)
    default:
      return items
  }
})

const retryHistoryOptions = computed(() => {
  return filteredRetryHistory.value.map(item => ({
    label: `${formatDateTime(item.created_at)} / 恢复 ${item.updated} / 失败 ${item.failed}`,
    value: getRetryHistoryKey(item),
  }))
})

const filteredAlerts = computed(() => {
  let items = overview.value?.alerts || []
  if (alertReasonFilter.value !== 'all')
    items = items.filter(item => item.alert_reason === alertReasonFilter.value)
  if (retryableOnly.value)
    items = items.filter(item => item.alert_reason === 'post_process_failed')
  return items
})

const retryableAlertIds = computed(() => {
  return filteredAlerts.value
    .filter(item => item.alert_reason === 'post_process_failed')
    .map(item => item.task_id)
})

const failedPostProcessAlertCount = computed(() => {
  return retryableAlertIds.value.length
})

const sortedRetryResultItems = computed(() => {
  return [...(lastRetryResult.value?.items || [])].sort((left, right) => {
    const priorityDiff = getRetryResultPriority(left) - getRetryResultPriority(right)
    if (priorityDiff !== 0)
      return priorityDiff
    return left.task_id - right.task_id
  })
})

const filteredRetryResultItems = computed(() => {
  const items = sortedRetryResultItems.value
  switch (retryResultFilter.value) {
    case 'recovered':
      return items.filter(item => item.outcome === 'completed')
    case 'failed':
      return items.filter(item => item.outcome === 'failed' || item.outcome === 'skipped' || item.post_process_status === 'failed')
    default:
      return items
  }
})

const visibleRetryResultItems = computed(() => {
  if (showAllRetryResults.value)
    return filteredRetryResultItems.value
  return filteredRetryResultItems.value.slice(0, 6)
})

const hiddenRetryResultCount = computed(() => {
  return Math.max(filteredRetryResultItems.value.length - visibleRetryResultItems.value.length, 0)
})

function formatDateTime(value?: string) {
  if (!value)
    return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function renderStatusTag(value?: string) {
  const map: Record<string, { type: 'default' | 'info' | 'success' | 'error' | 'warning', text: string }> = {
    pending: { type: 'default', text: '待处理' },
    processing: { type: 'info', text: '处理中' },
    completed: { type: 'success', text: '已完成' },
    failed: { type: 'error', text: '失败' },
  }
  const item = value ? map[value] : null
  if (!item)
    return value || '-'
  return h(NTag, { size: 'small', round: true, bordered: false, type: item.type }, { default: () => item.text })
}

function renderReasonTag(value?: string) {
  const map: Record<string, { type: 'warning' | 'error', text: string }> = {
    repeated_sync_failure: { type: 'warning', text: '重试过多' },
    post_process_failed: { type: 'error', text: '后处理失败' },
  }
  const item = value ? map[value] : null
  if (!item)
    return value || '-'
  return h(NTag, { size: 'small', round: true, bordered: false, type: item.type }, { default: () => item.text })
}

function renderOutcomeTag(value?: string) {
  const map: Record<string, { type: 'default' | 'info' | 'success' | 'error' | 'warning', text: string }> = {
    completed: { type: 'success', text: '已恢复' },
    updated: { type: 'info', text: '已更新' },
    unchanged: { type: 'default', text: '无变化' },
    skipped: { type: 'warning', text: '已跳过' },
    failed: { type: 'error', text: '仍失败' },
  }
  const item = value ? map[value] : null
  if (!item)
    return value || '-'
  return h(NTag, { size: 'small', round: true, bordered: false, type: item.type }, { default: () => item.text })
}

function getRetryHistoryKey(item: RetryPostProcessResult) {
  return item.created_at || `${item.limit}-${item.scanned}-${item.updated}-${item.failed}`
}

function getSingleQueryValue(value?: string | string[]) {
  if (Array.isArray(value))
    return value[0] || ''
  return value || ''
}

function isRetryHistoryFilterValue(value: string): value is typeof retryHistoryFilterValues[number] {
  return retryHistoryFilterValues.includes(value as typeof retryHistoryFilterValues[number])
}

function isRetryResultFilterValue(value: string): value is typeof retryResultFilterValues[number] {
  return retryResultFilterValues.includes(value as typeof retryResultFilterValues[number])
}

function getRetryHistoryTimestamp(item: RetryPostProcessResult) {
  if (!item.created_at)
    return 0
  const timestamp = new Date(item.created_at).getTime()
  return Number.isNaN(timestamp) ? 0 : timestamp
}

function getRetryResultPriority(item: RetryPostProcessItem) {
  if (item.outcome === 'failed' || item.outcome === 'skipped' || item.post_process_status === 'failed')
    return 0
  if (item.error_message)
    return 1
  if (item.outcome === 'unchanged')
    return 2
  if (item.outcome === 'updated')
    return 3
  if (item.outcome === 'completed' || item.post_process_status === 'completed')
    return 4
  return 5
}

function syncSelectedRetryHistory(preferredKey = selectedRetryHistoryKey.value) {
  const selected = filteredRetryHistory.value.find(item => getRetryHistoryKey(item) === preferredKey)
  const current = selected || filteredRetryHistory.value[0] || null
  selectedRetryHistoryKey.value = current ? getRetryHistoryKey(current) : ''
  lastRetryResult.value = current
}

function applyDashboardQueryState() {
  const historyFilter = getSingleQueryValue(route.query.historyFilter as string | string[] | undefined)
  const resultFilter = getSingleQueryValue(route.query.resultFilter as string | string[] | undefined)
  const historyKey = getSingleQueryValue(route.query.historyKey as string | string[] | undefined)

  if (isRetryHistoryFilterValue(historyFilter))
    retryHistoryFilter.value = historyFilter
  if (isRetryResultFilterValue(resultFilter))
    retryResultFilter.value = resultFilter
  selectedRetryHistoryKey.value = historyKey
}

function syncDashboardQueryState() {
  const nextQuery = { ...route.query }

  if (retryHistoryFilter.value === 'all')
    delete nextQuery.historyFilter
  else
    nextQuery.historyFilter = retryHistoryFilter.value

  if (retryResultFilter.value === 'all')
    delete nextQuery.resultFilter
  else
    nextQuery.resultFilter = retryResultFilter.value

  if (!selectedRetryHistoryKey.value)
    delete nextQuery.historyKey
  else
    nextQuery.historyKey = selectedRetryHistoryKey.value

  const currentHistoryFilter = getSingleQueryValue(route.query.historyFilter as string | string[] | undefined)
  const currentResultFilter = getSingleQueryValue(route.query.resultFilter as string | string[] | undefined)
  const currentHistoryKey = getSingleQueryValue(route.query.historyKey as string | string[] | undefined)

  if (
    currentHistoryFilter === getSingleQueryValue(nextQuery.historyFilter as string | string[] | undefined)
    && currentResultFilter === getSingleQueryValue(nextQuery.resultFilter as string | string[] | undefined)
    && currentHistoryKey === getSingleQueryValue(nextQuery.historyKey as string | string[] | undefined)
  ) {
    return
  }

  void router.replace({ query: nextQuery })
}

async function handleSyncTask(taskId: number) {
  if (syncingIds.value.includes(taskId))
    return

  syncingIds.value = [...syncingIds.value, taskId]
  try {
    await syncDashboardTask(taskId)
    message.success('任务已触发同步')
    await loadOverview()
  }
  catch {
    message.error('任务同步失败')
  }
  finally {
    syncingIds.value = syncingIds.value.filter(id => id !== taskId)
  }
}

async function handleRetryFailedPostProcess() {
  if (bulkRetrying.value)
    return

  bulkRetrying.value = true
  try {
    const limit = Math.min(Math.max(Number(retryLimit.value) || 20, 1), 100)
    retryLimit.value = limit
    const result = await retryDashboardPostProcessTasks(limit, retryableAlertIds.value)
    const data = result.data as RetryPostProcessResult
    selectedRetryHistoryKey.value = getRetryHistoryKey(data)
    retryResultFilter.value = 'all'
    showAllRetryResults.value = false
    message.success(`按当前筛选重试，扫描 ${data.scanned} 个任务，恢复 ${data.updated} 个，失败 ${data.failed} 个`)
    await loadOverview()
  }
  catch {
    message.error('批量重试后处理失败任务失败')
  }
  finally {
    bulkRetrying.value = false
  }
}

async function handleClearRetryHistory() {
  if (clearingRetryHistory.value)
    return

  const confirmed = await confirmDelete({
    entityType: '重试历史',
    entityName: '全部批次记录',
    description: '清空后，看板中的重试历史批次会全部移除，当前结果筛选也会被重置。',
  })
  if (!confirmed)
    return

  clearingRetryHistory.value = true
  try {
    await clearDashboardRetryHistory()
    selectedRetryHistoryKey.value = ''
    retryHistory.value = []
    lastRetryResult.value = null
    showAllRetryResults.value = false
    message.success('重试历史已清空')
    await loadOverview()
  }
  catch {
    message.error('清空重试历史失败')
  }
  finally {
    clearingRetryHistory.value = false
  }
}

async function handleDeleteCurrentRetryHistory() {
  await handleDeleteRetryHistoryItem(lastRetryResult.value?.created_at)
}

async function handleDeleteRetryHistoryItem(createdAt?: string) {
  if (!createdAt || deletingRetryHistoryItem.value)
    return

  const confirmed = await confirmDelete({
    entityType: '重试记录',
    entityName: formatDateTime(createdAt),
    description: '删除后，这一批次的重试结果将从历史列表中移除，但不会影响已经完成的任务状态。',
  })
  if (!confirmed)
    return

  deletingRetryHistoryItem.value = true
  deletingRetryHistoryKey.value = createdAt
  try {
    await deleteDashboardRetryHistoryItem(createdAt)
    if (selectedRetryHistoryKey.value === createdAt)
      selectedRetryHistoryKey.value = ''
    showAllRetryResults.value = false
    message.success('历史记录已删除')
    await loadOverview()
  }
  catch {
    message.error('删除当前历史记录失败')
  }
  finally {
    deletingRetryHistoryItem.value = false
    deletingRetryHistoryKey.value = ''
  }
}

function handleRetryHistoryChange(value: string) {
  selectedRetryHistoryKey.value = value
  syncSelectedRetryHistory(value)
  showAllRetryResults.value = false
}

function handleSelectRetryHistory(item: RetryPostProcessResult) {
  const key = getRetryHistoryKey(item)
  selectedRetryHistoryKey.value = key
  syncSelectedRetryHistory(key)
  showAllRetryResults.value = false
}

watch(retryHistoryFilter, () => {
  syncSelectedRetryHistory()
  showAllRetryResults.value = false
})

watch(() => route.query, () => {
  applyDashboardQueryState()
}, { immediate: true })

watch([retryHistoryFilter, retryResultFilter, selectedRetryHistoryKey], () => {
  syncDashboardQueryState()
})

function goToTask(taskId: number) {
  void router.push({
    name: 'transcription',
    query: { taskId: String(taskId) },
  })
}

function goToMeeting(meetingId?: number) {
  if (!meetingId)
    return
  void router.push({
    name: 'meeting-detail',
    params: { id: String(meetingId) },
  })
}

const alertColumns = [
  { title: 'ID', key: 'task_id', width: 64 },
  {
    title: '告警',
    key: 'alert_reason',
    width: 108,
    render: (row: SyncAlertItem) => renderReasonTag(row.alert_reason),
  },
  {
    title: '状态',
    key: 'status',
    width: 88,
    render: (row: SyncAlertItem) => renderStatusTag(row.status),
  },
  {
    title: '后处理',
    key: 'post_process_status',
    width: 88,
    render: (row: SyncAlertItem) => renderStatusTag(row.post_process_status),
  },
  {
    title: '重试',
    key: 'sync_fail_count',
    width: 72,
    render: (row: SyncAlertItem) => row.sync_fail_count > 0
      ? h(NTag, { size: 'small', round: true, bordered: false, type: 'warning' }, { default: () => `${row.sync_fail_count} 次` })
      : '-',
  },
  {
    title: '最近错误',
    key: 'last_sync_error',
    ellipsis: { tooltip: true },
    render: (row: SyncAlertItem) => row.last_sync_error || row.post_process_error || row.external_task_id || '-',
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 164,
    render: (row: SyncAlertItem) => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 168,
    render: (row: SyncAlertItem) => h('div', { class: 'flex items-center gap-1' }, [
      h(NButton, {
        text: true,
        type: 'primary',
        size: 'small',
        loading: syncingIds.value.includes(row.task_id),
        onClick: () => handleSyncTask(row.task_id),
      }, { default: () => '同步' }),
      h(NButton, {
        text: true,
        size: 'small',
        onClick: () => goToTask(row.task_id),
      }, { default: () => '任务' }),
      row.meeting_id && appStore.hasCapability(PRODUCT_FEATURE_KEYS.MEETING)
        ? h(NButton, {
            text: true,
            size: 'small',
            onClick: () => goToMeeting(row.meeting_id),
          }, { default: () => '会议' })
        : null,
    ]),
  },
]

const retryResultColumns = [
  { title: 'ID', key: 'task_id', width: 64 },
  {
    title: '结果',
    key: 'outcome',
    width: 88,
    render: (row: RetryPostProcessItem) => renderOutcomeTag(row.outcome),
  },
  {
    title: '后处理',
    key: 'post_process_status',
    width: 96,
    render: (row: RetryPostProcessItem) => renderStatusTag(row.post_process_status),
  },
  {
    title: '错误信息',
    key: 'error_message',
    ellipsis: { tooltip: true },
    render: (row: RetryPostProcessItem) => row.error_message || row.external_task_id || '无错误',
  },
  {
    title: '操作',
    key: 'actions',
    width: 120,
    render: (row: RetryPostProcessItem) => h('div', { class: 'flex items-center gap-1' }, [
      h(NButton, {
        text: true,
        type: 'primary',
        size: 'small',
        onClick: () => goToTask(row.task_id),
      }, { default: () => '任务' }),
      row.meeting_id && appStore.hasCapability(PRODUCT_FEATURE_KEYS.MEETING)
        ? h(NButton, {
            text: true,
            size: 'small',
            onClick: () => goToMeeting(row.meeting_id),
          }, { default: () => '会议' })
        : null,
    ]),
  },
]

const retryHistoryColumns = [
  {
    title: '时间',
    key: 'created_at',
    width: 164,
    render: (row: RetryPostProcessResult) => formatDateTime(row.created_at),
  },
  {
    title: '扫描',
    key: 'scanned',
    width: 72,
    render: (row: RetryPostProcessResult) => row.scanned,
  },
  {
    title: '恢复',
    key: 'updated',
    width: 72,
    render: (row: RetryPostProcessResult) => row.updated,
  },
  {
    title: '失败',
    key: 'failed',
    width: 72,
    render: (row: RetryPostProcessResult) => row.failed,
  },
  {
    title: '操作',
    key: 'actions',
    width: 132,
    render: (row: RetryPostProcessResult) => h('div', { class: 'flex items-center gap-1' }, [
      h(NButton, {
        text: true,
        size: 'small',
        type: getRetryHistoryKey(row) === selectedRetryHistoryKey.value ? 'primary' : 'default',
        onClick: () => handleSelectRetryHistory(row),
      }, { default: () => '查看' }),
      h(NButton, {
        text: true,
        size: 'small',
        type: 'error',
        disabled: !row.created_at,
        loading: deletingRetryHistoryItem.value && deletingRetryHistoryKey.value === row.created_at,
        onClick: () => handleDeleteRetryHistoryItem(row.created_at),
      }, { default: () => '删除' }),
    ]),
  },
]

async function loadOverview() {
  loading.value = true
  try {
    const result = await getDashboardOverview()
    overview.value = result.data
    const history: RetryPostProcessResult[] = result.data.retry_history?.length
      ? result.data.retry_history
      : (result.data.latest_retry_result ? [result.data.latest_retry_result] : [])
    retryHistory.value = history
    syncSelectedRetryHistory(selectedRetryHistoryKey.value)
  }
  catch {
    message.error('看板概览加载失败')
  }
  finally {
    loading.value = false
  }
}

onMounted(loadOverview)
</script>

<template>
  <div class="flex-1 flex flex-col min-w-0 gap-5 min-h-0">
    <section class="grid min-w-0 grid-cols-2 gap-3 md:grid-cols-4 2xl:grid-cols-7 shrink-0">
      <NCard v-for="metric in metrics" :key="metric.label" class="card-main min-w-0">
        <div class="text-xs text-slate/70">
          {{ metric.label }}
        </div>
        <div class="mt-1.5 font-display text-2xl font-700 text-ink">
          {{ metric.value }}
        </div>
        <div class="mt-0.5 text-xs text-slate/50">
          {{ metric.hint }}
        </div>
      </NCard>
    </section>

    <section class="flex-1 min-h-0 grid min-w-0 grid-cols-1 gap-5 2xl:grid-cols-[minmax(0,0.92fr)_minmax(0,1.28fr)]">
      <NCard class="card-main min-w-0 flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <span class="text-sm font-600">系统态势</span>
        </template>
        <div class="grid min-w-0 gap-3 md:grid-cols-2">
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              ASR 批量同步
            </div>
            <div class="mt-2 flex flex-wrap items-center justify-between gap-2">
              <span class="min-w-0 text-sm font-600 text-ink">最近同步 {{ formatDateTime(overview?.latest_sync_at) }}</span>
              <NTag :type="overview?.repeated_failure_count ? 'warning' : 'success'" round size="small">
                {{ overview?.repeated_failure_count ? '需关注' : '稳定' }}
              </NTag>
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              回流完成率
            </div>
            <div class="mt-2 flex flex-wrap items-center justify-between gap-2">
              <span class="text-sm font-600 text-ink">{{ overview?.post_process_completed_count ?? 0 }} / {{ overview?.completed_count ?? 0 }}</span>
              <NTag :type="(overview?.post_process_failed_count ?? 0) > 0 ? 'warning' : 'success'" round size="small">
                {{ (overview?.post_process_failed_count ?? 0) > 0 ? '阻塞' : '顺畅' }}
              </NTag>
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              后处理中任务
            </div>
            <div class="mt-2 flex flex-wrap items-center justify-between gap-2">
              <span class="text-sm font-600 text-ink">{{ (overview?.post_process_pending_count ?? 0) + (overview?.post_process_processing_count ?? 0) }}</span>
              <NTag type="info" round size="small">
                生成中
              </NTag>
            </div>
          </div>
          <div class="rounded-2.5 bg-mist/60 p-3.5">
            <div class="text-xs text-slate/70">
              后处理失败
            </div>
            <div class="mt-2 flex flex-wrap items-center justify-between gap-2">
              <span class="text-sm font-600 text-ink">{{ overview?.post_process_failed_count ?? 0 }}</span>
              <NTag :type="(overview?.post_process_failed_count ?? 0) > 0 ? 'warning' : 'success'" round size="small">
                {{ (overview?.post_process_failed_count ?? 0) > 0 ? '需介入' : '正常' }}
              </NTag>
            </div>
          </div>
        </div>
      </NCard>

      <NCard class="card-main min-w-0 flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span class="text-sm font-600">风险告警</span>
            <div class="flex flex-wrap items-center gap-2 md:justify-end">
              <NSelect v-model:value="alertReasonFilter" :options="alertReasonOptions" size="small" class="w-full sm:w-36" />
              <NCheckbox v-model:checked="retryableOnly">
                只看可重试
              </NCheckbox>
              <NInputNumber v-model:value="retryLimit" :min="1" :max="100" :step="5" size="small" class="w-full sm:w-28" placeholder="数量" />
              <NButton quaternary size="small" :disabled="failedPostProcessAlertCount === 0" :loading="bulkRetrying" @click="handleRetryFailedPostProcess">
                批量重试
              </NButton>
              <NButton quaternary size="small" :disabled="retryHistory.length === 0" :loading="clearingRetryHistory" @click="handleClearRetryHistory">
                清空历史
              </NButton>
              <NButton quaternary size="small" :loading="loading" @click="loadOverview">
                刷新
              </NButton>
            </div>
          </div>
        </template>
        <div class="mb-4 text-sm leading-6 text-slate">
          按筛选结果快速查看异常任务，并直接重试后处理、跳转任务或会议详情。
        </div>

        <div v-if="lastRetryResult" class="mb-4 min-w-0 rounded-2.5 bg-mist/60 p-4 text-sm text-slate">
          <div class="grid gap-2 text-ink sm:grid-cols-2 xl:grid-cols-4">
            <div class="rounded-2 bg-white/70 px-3 py-2">
              <div class="text-[11px] text-slate/70">
                执行时间
              </div>
              <div class="mt-1 text-xs font-600">
                {{ formatDateTime(lastRetryResult.created_at) }}
              </div>
            </div>
            <div class="rounded-2 bg-white/70 px-3 py-2">
              <div class="text-[11px] text-slate/70">
                扫描 / 上限
              </div>
              <div class="mt-1 text-xs font-600">
                {{ lastRetryResult.scanned }} / {{ lastRetryResult.limit || 0 }}
              </div>
            </div>
            <div class="rounded-2 bg-white/70 px-3 py-2">
              <div class="text-[11px] text-slate/70">
                恢复 / 失败
              </div>
              <div class="mt-1 text-xs font-600">
                {{ lastRetryResult.updated }} / {{ lastRetryResult.failed }}
              </div>
            </div>
            <div class="rounded-2 bg-white/70 px-3 py-2">
              <div class="text-[11px] text-slate/70">
                当前结果 / 历史批次
              </div>
              <div class="mt-1 text-xs font-600">
                {{ filteredRetryResultItems.length }} / {{ filteredRetryHistory.length }}
              </div>
            </div>
          </div>
          <div class="mt-3 flex flex-wrap items-center gap-3">
            <NSelect v-if="retryHistory.length > 1" v-model:value="retryHistoryFilter" :options="retryHistoryFilterOptions" class="w-full sm:w-36" />
            <NSelect v-if="retryHistoryOptions.length > 1" v-model:value="selectedRetryHistoryKey" :options="retryHistoryOptions" class="w-full lg:w-72" @update:value="handleRetryHistoryChange" />
            <NSelect v-model:value="retryResultFilter" :options="retryResultFilterOptions" class="w-full sm:w-40" />
            <NButton quaternary size="small" :disabled="!lastRetryResult?.created_at" :loading="deletingRetryHistoryItem" @click="handleDeleteCurrentRetryHistory">
              删除当前记录
            </NButton>
            <NButton v-if="filteredRetryResultItems.length > 6" quaternary size="small" @click="showAllRetryResults = !showAllRetryResults">
              {{ showAllRetryResults ? '收起结果' : `展开全部 (${filteredRetryResultItems.length})` }}
            </NButton>
            <NButton quaternary size="small" @click="lastRetryResult = null">
              清空结果
            </NButton>
          </div>
          <div v-if="filteredRetryResultItems.length" class="mt-3 grid min-w-0 gap-2">
            <div class="min-w-0 overflow-x-auto">
              <NDataTable
                :columns="retryResultColumns"
                :data="visibleRetryResultItems"
                :pagination="false"
                :scroll-x="640"
                size="small"
              />
            </div>
            <div v-if="hiddenRetryResultCount > 0" class="text-sm text-slate">
              还有 {{ hiddenRetryResultCount }} 条结果未展开。
            </div>
          </div>
          <div v-else class="mt-3 text-sm text-slate">
            当前筛选下没有重试结果。
          </div>

          <div v-if="filteredRetryHistory.length" class="mt-4 grid min-w-0 gap-2">
            <div class="text-sm text-slate">
              最近批次
            </div>
            <div class="min-w-0 overflow-x-auto">
              <NDataTable
                :columns="retryHistoryColumns"
                :data="filteredRetryHistory"
                :pagination="false"
                :scroll-x="460"
                size="small"
              />
            </div>
          </div>
        </div>

        <div class="flex-1 min-h-0 min-w-0 flex flex-col overflow-hidden">
          <NDataTable
            flex-height
            class="flex-1 min-h-0"
            :columns="alertColumns"
            :data="filteredAlerts"
            :loading="loading"
            :pagination="{ pageSize: 5 }"
            :scroll-x="820"
            size="small"
          />
        </div>
      </NCard>
    </section>
  </div>
</template>

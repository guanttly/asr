<script setup lang="ts">
import { NButton, NTag, NTooltip, useMessage } from 'naive-ui'
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { deleteMeeting, getMeetings } from '@/api/meeting'
import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'

interface MeetingItem {
  id: number
  title: string
  status: string
  duration: number
  sync_fail_count?: number
  last_sync_error?: string
  last_sync_at?: string
  next_sync_at?: string
  created_at?: string
  createdAt?: string
  updated_at?: string
  updatedAt?: string
}

const router = useRouter()
const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()
const { subscribe: subscribeBusinessTopic } = useBusinessSocket()
const loading = ref(false)
const keyword = ref('')
const meetings = ref<MeetingItem[]>([])
const deletingMeetingId = ref<number | null>(null)
const nowTimestamp = ref(Date.now())
let clockTimer: number | null = null
let stopBusinessSubscription: (() => void) | null = null

function formatDateTime(value?: string) {
  if (!value)
    return '-'

  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value

  return date.toLocaleString('zh-CN', { hour12: false })
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

function parseDateValue(value?: string) {
  if (!value)
    return 0
  const timestamp = new Date(value).getTime()
  return Number.isNaN(timestamp) ? 0 : timestamp
}

function isMeetingActive(row: MeetingItem) {
  return row.status === 'processing'
}

function isMeetingDeletable(row: MeetingItem) {
  return ['uploaded', 'completed', 'failed'].includes(row.status)
}

function formatElapsedDuration(row: MeetingItem) {
  const start = parseDateValue(row.createdAt || row.created_at)
  if (!start)
    return '-'
  const end = isMeetingActive(row)
    ? nowTimestamp.value
    : parseDateValue(row.updatedAt || row.updated_at) || nowTimestamp.value
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

function meetingStatusMeta(status?: string) {
  const map: Record<string, { type: string, text: string }> = {
    uploaded: { type: 'warning', text: '待转写' },
    pending: { type: 'default', text: '待处理' },
    processing: { type: 'info', text: '转写中' },
    completed: { type: 'success', text: '已完成' },
    failed: { type: 'error', text: '失败' },
  }
  return map[status || ''] || { type: 'default', text: status || '-' }
}

function meetingFailureSummary(row: MeetingItem) {
  return row.last_sync_error?.trim() || ''
}

function meetingFailureTooltip(row: MeetingItem) {
  if (row.status !== 'failed')
    return ''

  const parts = [`状态：${meetingStatusMeta(row.status).text}`]
  if (row.sync_fail_count)
    parts.push(`失败次数：${row.sync_fail_count}`)
  if (row.last_sync_error?.trim())
    parts.push(`原因：${row.last_sync_error.trim()}`)
  if (row.last_sync_at)
    parts.push(`上次失败：${formatDateTime(row.last_sync_at)}`)
  if (row.next_sync_at)
    parts.push(`下次重试：${formatDateTime(row.next_sync_at)}`)
  return parts.join('\n')
}

async function handleDeleteMeeting(row: MeetingItem) {
  if (!isMeetingDeletable(row) || deletingMeetingId.value != null)
    return

  const confirmed = await confirmDelete({
    entityType: '会议记录',
    entityName: row.title || `#${row.id}`,
    description: '删除后，这条会议记录、逐字稿和摘要会一并移除。',
  })
  if (!confirmed)
    return

  deletingMeetingId.value = row.id
  try {
    await deleteMeeting(row.id)
    meetings.value = meetings.value.filter(item => item.id !== row.id)
    message.success('会议记录已删除')
  }
  catch {
    message.error('会议记录删除失败')
  }
  finally {
    deletingMeetingId.value = null
  }
}

const columns = [
  { title: 'ID', key: 'id', width: 64 },
  {
    title: '会议标题',
    key: 'title',
    minWidth: 260,
    render: (row: MeetingItem) => h('div', { class: 'min-w-0' }, [
      h('div', {
        class: 'truncate text-sm font-600 text-ink',
        title: row.title || undefined,
      }, row.title || `会议 #${row.id}`),
      row.status === 'failed' && meetingFailureSummary(row)
        ? h(NTooltip, { placement: 'top-start' }, {
            trigger: () => h('div', {
              class: 'mt-1 truncate text-xs text-rose-700',
              title: meetingFailureSummary(row),
            }, `失败原因：${meetingFailureSummary(row)}`),
            default: () => h('div', { class: 'max-w-84 whitespace-pre-wrap text-xs leading-6' }, meetingFailureTooltip(row)),
          })
        : null,
    ]),
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: (row: MeetingItem) => {
      const item = meetingStatusMeta(row.status)
      const tag = h(NTag, { type: item.type as any, size: 'small', round: true, bordered: false }, { default: () => item.text })
      if (row.status !== 'failed')
        return tag
      return h(NTooltip, { placement: 'top' }, {
        trigger: () => tag,
        default: () => h('div', { class: 'max-w-84 whitespace-pre-wrap text-xs leading-6' }, meetingFailureTooltip(row) || '当前没有返回失败原因。'),
      })
    },
  },
  {
    title: '音频时长',
    key: 'duration',
    width: 96,
    render: (row: MeetingItem) => formatAudioDuration(row.duration),
  },
  {
    title: '处理耗时',
    key: 'elapsed',
    width: 120,
    render: (row: MeetingItem) => formatElapsedDuration(row),
  },
  {
    title: '创建时间',
    key: 'created_at',
    width: 164,
    render: (row: MeetingItem) => formatDateTime(row.createdAt || row.created_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 132,
    render: (row: MeetingItem) => h('div', { class: 'flex items-center gap-1' }, [
      h(NButton, {
        text: true,
        type: 'primary',
        size: 'small',
        onClick: () => router.push(`/meetings/${row.id}`),
      }, { default: () => '详情' }),
      isMeetingDeletable(row)
        ? h(NButton, {
            text: true,
            type: 'error',
            size: 'small',
            loading: deletingMeetingId.value === row.id,
            onClick: () => handleDeleteMeeting(row),
          }, { default: () => '删除' })
        : null,
    ]),
  },
]

const filteredMeetings = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  if (!value)
    return meetings.value
  return meetings.value.filter(item =>
    item.title.toLowerCase().includes(value)
    || item.status.toLowerCase().includes(value),
  )
})

async function loadMeetings() {
  loading.value = true
  try {
    const result = await getMeetings({ offset: 0, limit: 100 })
    meetings.value = result.data.items
  }
  catch {
    message.error('会议列表加载失败')
  }
  finally {
    loading.value = false
  }
}

function applyMeetingUpdate(meeting?: MeetingItem) {
  if (!meeting?.id)
    return

  const index = meetings.value.findIndex(item => item.id === meeting.id)
  if (index >= 0) {
    meetings.value[index] = {
      ...meetings.value[index],
      ...meeting,
    }
    return
  }

  meetings.value = [meeting, ...meetings.value]
}

onMounted(() => {
  void loadMeetings()
  clockTimer = window.setInterval(() => {
    nowTimestamp.value = Date.now()
  }, 1000)

  stopBusinessSubscription = subscribeBusinessTopic(['meeting.updated'], (event) => {
    const payload = event.payload as { meeting?: MeetingItem } | undefined
    applyMeetingUpdate(payload?.meeting)
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
  <div class="flex-1 flex flex-col min-h-0 gap-5">
    <NCard class="card-main flex flex-col min-h-0 flex-1" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-sm font-600">会议列表</span>
          <div class="flex flex-wrap items-center gap-2">
            <NInput v-model:value="keyword" clearable placeholder="搜索会议标题 / 状态" size="small" class="w-full sm:!w-56" />
            <NButton quaternary size="small" @click="loadMeetings">
              刷新
            </NButton>
            <NButton quaternary size="small" @click="router.push('/meetings/voiceprints')">
              声纹库
            </NButton>
            <NButton type="primary" size="small" color="#0f766e" @click="router.push('/meetings/upload')">
              新建会议
            </NButton>
          </div>
        </div>
      </template>
      <div v-if="filteredMeetings.length > 0 || loading" class="flex-1 flex flex-col min-h-0 min-w-0 overflow-hidden">
        <NDataTable flex-height class="flex-1 min-h-0" :columns="columns" :data="filteredMeetings" :loading="loading" :pagination="{ pageSize: 10 }" :scroll-x="720" size="small" />
      </div>
      <div v-else class="flex-1 flex items-center justify-center min-h-0">
        <NEmpty description="暂无会议记录，可先创建一条会议任务" class="empty-shell py-10" />
      </div>
    </NCard>
  </div>
</template>

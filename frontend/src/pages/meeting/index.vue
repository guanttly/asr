<script setup lang="ts">
import { computed, h, onMounted, ref } from 'vue'
import { NButton, NTag, useMessage } from 'naive-ui'
import { useRouter } from 'vue-router'

import { getMeetings } from '@/api/meeting'

type MeetingItem = {
  id: number
  title: string
  status: string
  duration: number
  created_at?: string
  createdAt?: string
}

const router = useRouter()
const message = useMessage()
const loading = ref(false)
const keyword = ref('')
const meetings = ref<MeetingItem[]>([])

function formatDateTime(value?: string) {
  if (!value)
    return '-'

  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value

  return date.toLocaleString('zh-CN', { hour12: false })
}

const columns = [
  { title: 'ID', key: 'id', width: 64 },
  {
    title: '会议标题',
    key: 'title',
    ellipsis: { tooltip: true },
  },
  {
    title: '状态',
    key: 'status',
    width: 100,
    render: (row: MeetingItem) => {
      const map: Record<string, { type: string, text: string }> = {
        pending: { type: 'default', text: '待处理' },
        processing: { type: 'info', text: '处理中' },
        completed: { type: 'success', text: '已完成' },
        failed: { type: 'error', text: '失败' },
      }
      const item = map[row.status] || { type: 'default', text: row.status }
      return h(NTag, { type: item.type as any, size: 'small', round: true, bordered: false }, { default: () => item.text })
    },
  },
  {
    title: '时长',
    key: 'duration',
    width: 88,
    render: (row: MeetingItem) => row.duration ? `${row.duration}s` : '-',
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
    width: 88,
    render: (row: MeetingItem) => h(NButton, {
      text: true,
      type: 'primary',
      size: 'small',
      onClick: () => router.push(`/meetings/${row.id}`),
    }, { default: () => '详情' }),
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

onMounted(loadMeetings)
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 gap-5">

    <NCard class="card-main flex flex-col min-h-0 flex-1" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-sm font-600">会议列表</span>
          <div class="flex flex-wrap items-center gap-2">
            <NInput v-model:value="keyword" clearable placeholder="搜索会议标题 / 状态" size="small" class="w-full sm:!w-56" />
            <NButton quaternary size="small" @click="loadMeetings">刷新</NButton>
            <NButton type="primary" size="small" color="#0f766e" @click="router.push('/meetings/upload')">新建会议</NButton>
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
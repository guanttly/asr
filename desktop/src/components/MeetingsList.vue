<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'

import MeetingDetail from './MeetingDetail.vue'

import { useConfirm } from '@/composables/useConfirm'
import { deleteMeeting, listMeetings, type MeetingItem } from '@/utils/meetings'
import { debugLog } from '@/utils/debug'

const PAGE_SIZE = 10
const POLL_INTERVAL_MS = 6000

const items = ref<MeetingItem[]>([])
const total = ref(0)
const page = ref(1)
const loading = ref(false)
const refreshing = ref(false)
const errorText = ref('')
const search = ref('')
const selectedId = ref<number | null>(null)

const { confirm } = useConfirm()
let pollTimer: ReturnType<typeof setInterval> | null = null

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / PAGE_SIZE)))

const filteredItems = computed(() => {
  const keyword = search.value.trim().toLowerCase()
  if (!keyword)
    return items.value
  return items.value.filter(item => (item.title || '').toLowerCase().includes(keyword))
})

const hasProcessing = computed(() => items.value.some(item => item.status === 'uploaded' || item.status === 'processing'))

function statusLabel(status: string) {
  switch (status) {
    case 'completed':
      return '已完成'
    case 'processing':
      return '生成中'
    case 'uploaded':
      return '排队中'
    case 'failed':
      return '失败'
    default:
      return status || '未知'
  }
}

function statusTone(status: string) {
  switch (status) {
    case 'completed':
      return 'success'
    case 'processing':
      return 'pending'
    case 'uploaded':
      return 'pending'
    case 'failed':
      return 'danger'
    default:
      return 'plain'
  }
}

function formatDate(value?: string) {
  if (!value)
    return '--'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return new Intl.DateTimeFormat('zh-CN', {
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
  }).format(date)
}

function formatDuration(value?: number) {
  if (!value || value <= 0)
    return '--'
  if (value < 60)
    return `${Math.round(value)} 秒`
  const minutes = Math.floor(value / 60)
  const seconds = Math.round(value % 60)
  return seconds > 0 ? `${minutes} 分 ${seconds} 秒` : `${minutes} 分钟`
}

async function loadMeetings(opts?: { silent?: boolean }) {
  if (!opts?.silent)
    refreshing.value = true
  if (!items.value.length)
    loading.value = true

  try {
    const offset = (page.value - 1) * PAGE_SIZE
    const result = await listMeetings({ offset, limit: PAGE_SIZE })
    items.value = result.items
    total.value = result.total
    if (page.value > totalPages.value) {
      page.value = totalPages.value
      const offset2 = (page.value - 1) * PAGE_SIZE
      const result2 = await listMeetings({ offset: offset2, limit: PAGE_SIZE })
      items.value = result2.items
      total.value = result2.total
    }
    errorText.value = ''
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '加载会议列表失败'
    void debugLog('meetings.list', 'failed to load meetings', error instanceof Error ? {
      message: error.message,
      stack: error.stack,
    } : error)
  }
  finally {
    loading.value = false
    refreshing.value = false
  }
}

async function handleDelete(item: MeetingItem) {
  const ok = await confirm({
    title: '删除会议',
    message: `确定删除会议「${item.title || `#${item.id}`}」吗？已生成的纪要也会被一并删除。`,
    confirmText: '删除',
    tone: 'danger',
  })
  if (!ok)
    return
  try {
    await deleteMeeting(item.id)
    if (selectedId.value === item.id)
      selectedId.value = null
    await loadMeetings()
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '删除会议失败'
  }
}

function setPage(next: number) {
  if (next < 1 || next > totalPages.value || next === page.value)
    return
  page.value = next
  void loadMeetings()
}

function startPolling() {
  if (pollTimer)
    return
  pollTimer = setInterval(() => {
    if (hasProcessing.value)
      void loadMeetings({ silent: true })
  }, POLL_INTERVAL_MS)
}

function stopPolling() {
  if (pollTimer) {
    clearInterval(pollTimer)
    pollTimer = null
  }
}

onMounted(() => {
  void loadMeetings()
  startPolling()
})

onBeforeUnmount(() => {
  stopPolling()
})
</script>

<template>
  <div class="meeting-list">
    <div class="meeting-toolbar">
      <div class="meeting-search">
        <svg class="search-icon" viewBox="0 0 16 16" width="14" height="14">
          <path fill="currentColor" d="M11.742 10.344a6.5 6.5 0 1 0-1.397 1.398h-.001q.044.06.098.115l3.85 3.85a1 1 0 0 0 1.415-1.414l-3.85-3.85a1 1 0 0 0-.115-.1zM12 6.5a5.5 5.5 0 1 1-11 0a5.5 5.5 0 0 1 11 0" />
        </svg>
        <input v-model="search" type="text" placeholder="按标题搜索会议" spellcheck="false">
      </div>
      <button class="meeting-refresh" :disabled="refreshing" @click="loadMeetings()">
        {{ refreshing ? '刷新中' : '刷新' }}
      </button>
    </div>

    <p v-if="errorText" class="meeting-alert">{{ errorText }}</p>

    <div class="meeting-scroller">
      <div v-if="loading && items.length === 0" class="meeting-empty">正在加载会议纪要...</div>
      <div v-else-if="filteredItems.length === 0" class="meeting-empty">
        {{ items.length === 0 ? '暂无会议纪要，开启会议模式录音后将自动生成' : '没有匹配的会议' }}
      </div>

      <article
        v-for="item in filteredItems"
        :key="item.id"
        class="meeting-card"
        :class="{ active: selectedId === item.id }"
        @click="selectedId = item.id"
      >
        <div class="meeting-card-head">
          <div class="meeting-title">{{ item.title || `会议 #${item.id}` }}</div>
          <span class="status-pill" :class="statusTone(item.status)">
            <span v-if="statusTone(item.status) === 'pending'" class="pulse-dot" />
            {{ statusLabel(item.status) }}
          </span>
        </div>
        <div class="meeting-card-meta">
          <span>{{ formatDate(item.created_at) }}</span>
          <span class="meta-sep">·</span>
          <span>{{ formatDuration(item.duration) }}</span>
        </div>
        <div v-if="item.last_sync_error" class="meeting-error">{{ item.last_sync_error }}</div>
        <div class="meeting-card-actions">
          <button class="link-btn" type="button" @click.stop="selectedId = item.id">查看纪要</button>
          <button class="link-btn danger" type="button" @click.stop="handleDelete(item)">删除</button>
        </div>
      </article>
    </div>

    <div v-if="total > PAGE_SIZE" class="meeting-pager">
      <button :disabled="page <= 1" @click="setPage(page - 1)">上一页</button>
      <span class="meeting-pager-info">第 {{ page }} / {{ totalPages }} 页 · 共 {{ total }} 条</span>
      <button :disabled="page >= totalPages" @click="setPage(page + 1)">下一页</button>
    </div>

    <MeetingDetail
      v-if="selectedId != null"
      :meeting-id="selectedId"
      @close="selectedId = null"
      @deleted="() => { selectedId = null; void loadMeetings() }"
      @saved="() => { void loadMeetings({ silent: true }) }"
    />
  </div>
</template>

<style scoped>
.meeting-list {
  display: flex;
  flex-direction: column;
  height: 100%;
  padding: 12px;
  gap: 12px;
}

.meeting-toolbar {
  display: flex;
  gap: 8px;
  align-items: center;
}

.meeting-search {
  flex: 1;
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 999px;
  background: rgba(241, 245, 249, 0.92);
  border: 1px solid rgba(226, 232, 240, 0.85);
  color: #94a3b8;
  transition: border-color 0.18s ease;
}

.meeting-search:focus-within {
  border-color: #0f766e;
  color: #0f766e;
}

.meeting-search input {
  flex: 1;
  border: 0;
  outline: none;
  background: transparent;
  font-size: 12px;
  color: #0f172a;
}

.meeting-refresh {
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  background: #ffffff;
  color: #475569;
  padding: 6px 14px;
  font-size: 12px;
  cursor: pointer;
  transition: background 0.18s ease;
}

.meeting-refresh:hover:not(:disabled) {
  background: rgba(241, 245, 249, 0.92);
}

.meeting-refresh:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.meeting-alert {
  margin: 0;
  padding: 8px 12px;
  font-size: 12px;
  color: #b91c1c;
  background: rgba(254, 242, 242, 0.92);
  border-radius: 10px;
}

.meeting-scroller {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 10px;
  padding-right: 2px;
}

.meeting-empty {
  padding: 32px 0;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}

.meeting-card {
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.84);
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(248, 250, 252, 0.96));
  padding: 12px 14px;
  display: flex;
  flex-direction: column;
  gap: 6px;
  cursor: pointer;
  transition: transform 0.16s ease, box-shadow 0.18s ease, border-color 0.18s ease;
}

.meeting-card:hover {
  transform: translateY(-1px);
  box-shadow: 0 12px 22px rgba(148, 163, 184, 0.18);
}

.meeting-card.active {
  border-color: #0f766e;
  box-shadow: 0 12px 22px rgba(15, 118, 110, 0.18);
}

.meeting-card-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 8px;
}

.meeting-title {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
  flex: 1;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meeting-card-meta {
  font-size: 11px;
  color: #94a3b8;
  display: flex;
  align-items: center;
  gap: 4px;
}

.meta-sep {
  margin: 0 4px;
}

.meeting-error {
  font-size: 11px;
  color: #b91c1c;
  background: rgba(254, 242, 242, 0.6);
  padding: 4px 8px;
  border-radius: 6px;
}

.meeting-card-actions {
  display: flex;
  justify-content: flex-end;
  gap: 12px;
  margin-top: 4px;
}

.link-btn {
  border: 0;
  background: transparent;
  color: #0f766e;
  font-size: 12px;
  cursor: pointer;
}

.link-btn:hover {
  text-decoration: underline;
}

.link-btn.danger {
  color: #b91c1c;
}

.status-pill {
  flex-shrink: 0;
  display: inline-flex;
  align-items: center;
  gap: 4px;
  border-radius: 999px;
  padding: 3px 10px;
  font-size: 11px;
  font-weight: 600;
}

.status-pill.success {
  background: rgba(220, 252, 231, 0.9);
  color: #166534;
}

.status-pill.pending {
  background: rgba(254, 249, 195, 0.92);
  color: #854d0e;
}

.status-pill.danger {
  background: rgba(254, 242, 242, 0.92);
  color: #b91c1c;
}

.status-pill.plain {
  background: rgba(241, 245, 249, 0.94);
  color: #475569;
}

.pulse-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: currentColor;
  animation: pulse 1.4s ease-in-out infinite;
}

@keyframes pulse {
  0%, 100% { opacity: 0.4; transform: scale(0.9); }
  50% { opacity: 1; transform: scale(1.1); }
}

.meeting-pager {
  display: flex;
  justify-content: space-between;
  align-items: center;
  font-size: 12px;
  color: #475569;
}

.meeting-pager button {
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  background: #ffffff;
  color: #334155;
  padding: 5px 14px;
  font-size: 12px;
  cursor: pointer;
}

.meeting-pager button:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}

.meeting-pager-info {
  flex: 1;
  text-align: center;
  color: #64748b;
}

.search-icon {
  flex-shrink: 0;
  color: inherit;
}
</style>

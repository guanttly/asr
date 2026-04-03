<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useRoute } from 'vue-router'

import { getMeetingDetail } from '@/api/meeting'

type TranscriptItem = {
  speaker_label: string
  text: string
  start_time: number
  end_time: number
}

type SummaryItem = {
  content: string
  model_version: string
  created_at?: string
}

type MeetingDetail = {
  id: number
  title: string
  status: string
  duration: number
  transcripts: TranscriptItem[]
  summary?: SummaryItem | null
}

const route = useRoute()
const message = useMessage()
const loading = ref(false)
const detail = ref<MeetingDetail | null>(null)

const transcript = computed(() => detail.value?.transcripts || [])

function formatTime(value: number) {
  const minute = Math.floor(value / 60).toString().padStart(2, '0')
  const second = Math.floor(value % 60).toString().padStart(2, '0')
  return `${minute}:${second}`
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

onMounted(loadDetail)
</script>

<template>
  <div class="grid gap-5">

    <div class="grid grid-cols-2 gap-3 md:grid-cols-4">
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">会议 ID</div>
        <div class="mt-1.5 text-sm font-700 text-ink">#{{ route.params.id }}</div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">状态</div>
        <div class="mt-1.5 text-sm font-700 text-ink">{{ detail?.status || '-' }}</div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">时长</div>
        <div class="mt-1.5 text-sm font-700 text-ink">{{ detail?.duration ?? '-' }} 秒</div>
      </div>
      <div class="subtle-panel">
        <div class="text-xs text-slate/70">片段数</div>
        <div class="mt-1.5 text-sm font-700 text-ink">{{ transcript.length }}</div>
      </div>
    </div>

    <NCard class="card-main" :loading="loading">
      <NTabs type="line" animated>
      <NTabPane name="transcript" tab="逐字稿">
        <div class="grid gap-4">
          <div v-for="(item, index) in transcript" :key="`${index}-${item.start_time}`" class="subtle-panel">
            <div class="flex items-center justify-between">
              <div class="font-600 text-ink">{{ item.speaker_label }}</div>
              <div class="text-xs text-slate/70">{{ formatTime(item.start_time) }} - {{ formatTime(item.end_time) }}</div>
            </div>
            <div class="mt-3 text-sm leading-6 text-ink">{{ item.text }}</div>
          </div>

          <NEmpty v-if="!loading && transcript.length === 0" description="当前会议还没有逐字稿内容" class="empty-shell" />
        </div>
      </NTabPane>
      <NTabPane name="summary" tab="会议摘要">
        <div class="subtle-panel">
          <div class="font-600 text-ink">核心内容</div>
          <p class="mt-3 whitespace-pre-line leading-7 text-slate">{{ detail?.summary?.content || '当前会议还没有生成摘要。' }}</p>
          <div v-if="detail?.summary" class="mt-4 text-xs text-slate">
            模型版本：{{ detail.summary.model_version }}
          </div>
        </div>
      </NTabPane>
      </NTabs>
    </NCard>
  </div>
</template>
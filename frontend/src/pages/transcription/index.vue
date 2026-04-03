<script setup lang="ts">
import type { StreamingMessage } from '@/types/asr'

import { useMessage } from 'naive-ui'
import { computed, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { createTranscriptionTask } from '@/api/asr'
import { useAudioRecorder } from '@/composables/useAudioRecorder'
import { useWebSocket } from '@/composables/useWebSocket'
import { useTranscriptionStore } from '@/stores/transcription'
import { useUserStore } from '@/stores/user'

const message = useMessage()
const router = useRouter()
const store = useTranscriptionStore()
const userStore = useUserStore()
const { start, stop, pause, resume, isRecording, isPaused } = useAudioRecorder()
const { connect, disconnect, connected, messages, totalMessages, send, sendJSON } = useWebSocket()
const savingSession = ref(false)
const stoppingSession = ref(false)
const recordingStartedAt = ref<number | null>(null)

const chunkCount = computed(() => totalMessages.value)
const finalCount = computed(() => store.totalSentenceCount)

function exportTranscript() {
  if (!store.transcriptText.trim()) {
    message.warning('当前没有可导出的转写结果')
    return
  }

  const blob = new Blob([store.transcriptText], { type: 'text/plain;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = `transcription-${new Date().toISOString().replace(/[:.]/g, '-')}.txt`
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

async function copyTranscript() {
  if (!store.transcriptText.trim()) {
    message.warning('当前没有可复制的转写结果')
    return
  }

  try {
    await navigator.clipboard.writeText(store.transcriptText)
    message.success('完整转写已复制')
  }
  catch {
    message.error('复制失败，请检查浏览器剪贴板权限')
  }
}

function waitForStopAck(timeoutMs = 2000) {
  return new Promise<void>((resolve) => {
    if (!connected.value) {
      resolve()
      return
    }

    let settled = false
    let timer: ReturnType<typeof setTimeout> | null = null
    let stopMessageWatch: (() => void) | null = null
    let stopConnectedWatch: (() => void) | null = null

    const cleanup = () => {
      if (settled)
        return
      settled = true
      stopMessageWatch?.()
      stopConnectedWatch?.()
      if (timer)
        clearTimeout(timer)
      resolve()
    }

    stopMessageWatch = watch(messages, (value) => {
      const latest = value.at(-1)
      if (!latest)
        return
      if (latest.type === 'ack' && latest.text === 'control:stop')
        cleanup()
      if (latest.type === 'error')
        cleanup()
    }, { deep: true })

    stopConnectedWatch = watch(connected, (value) => {
      if (!value)
        cleanup()
    })

    timer = setTimeout(cleanup, timeoutMs)
    sendJSON({ type: 'control', event: 'stop' })
  })
}

function currentDurationSeconds() {
  if (!recordingStartedAt.value)
    return 0
  return Math.max(0, Math.round((Date.now() - recordingStartedAt.value) / 1000))
}

async function persistRealtimeSession() {
  const transcript = store.transcriptText.trim()
  if (!transcript)
    return

  savingSession.value = true
  try {
    const result = await createTranscriptionTask({
      type: 'realtime',
      result_text: transcript,
      duration: currentDurationSeconds(),
    })
    const taskId = result?.data?.id
    if (taskId)
      message.success(`实时转写已保存到历史任务 #${taskId}`)
    else
      message.success('实时转写已保存到历史任务')
  }
  catch {
    message.warning('实时转写已停止，但保存历史任务失败')
  }
  finally {
    savingSession.value = false
  }
}

async function handleStart() {
  try {
    store.reset()
    await connect(userStore.token)
    sendJSON({ type: 'control', event: 'start' })
    await start((chunk) => {
      send(chunk)
    })
    store.isRecording = true
    recordingStartedAt.value = Date.now()
    message.success('录音已启动')
  }
  catch (error) {
    stop()
    disconnect()
    store.isRecording = false
    recordingStartedAt.value = null
    const detail = error instanceof Error ? error.message : '请检查登录态、浏览器麦克风权限和 asr-api 服务'
    message.error(detail)
  }
}

async function handleStop() {
  if (stoppingSession.value)
    return

  stoppingSession.value = true
  stop()
  try {
    await waitForStopAck()
  }
  finally {
    disconnect()
    store.isRecording = false
    await persistRealtimeSession()
    recordingStartedAt.value = null
    stoppingSession.value = false
  }
}

function pushMockSentence() {
  send('当前是转写演示文本')
}

watch(messages, (value: StreamingMessage[]) => {
  const latest = value.at(-1)
  if (!latest)
    return

  if (latest.type === 'error') {
    message.error(latest.text || '实时转写链路异常')
    stop()
    disconnect()
    store.isRecording = false
    recordingStartedAt.value = null
    return
  }

  if (latest.type === 'ack') {
    return
  }

  store.setDraftText(latest.text)
  if (latest.is_final || latest.type === 'sentence')
    store.appendSentence(latest.text)
}, { deep: true })
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 gap-5">
    <section class="card-main p-4 sm:p-5 shrink-0">
      <div class="flex flex-col gap-4 xl:flex-row xl:items-start xl:justify-between">
        <div class="grid grid-cols-2 gap-3 lg:grid-cols-4 xl:flex-1">
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              连接状态
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ connected ? 'WebSocket 已连接' : '等待连接' }}
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              录音状态
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ isRecording ? (isPaused ? '暂停中' : '录音中') : '空闲' }}
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              术语字典
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              医疗查房词库
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              累计消息 / 句子
            </div>
            <div class="mt-1.5 text-sm font-700 text-ink">
              {{ chunkCount }} / {{ finalCount }}
            </div>
          </div>
        </div>

        <div class="flex flex-wrap gap-2 xl:max-w-[420px] xl:justify-end">
          <NButton size="small" type="primary" color="#0f766e" :disabled="isRecording" @click="handleStart">
            开始录音
          </NButton>
          <NButton size="small" :disabled="!isRecording || isPaused" @click="pause">
            暂停
          </NButton>
          <NButton size="small" :disabled="!isRecording || !isPaused" @click="resume">
            继续
          </NButton>
          <NButton size="small" tertiary :disabled="!isRecording || stoppingSession" :loading="stoppingSession" @click="handleStop">
            停止
          </NButton>
          <NButton size="small" quaternary @click="pushMockSentence">
            模拟片段
          </NButton>
          <NButton size="small" quaternary :disabled="!store.transcriptText || savingSession" :loading="savingSession" @click="copyTranscript">
            复制全文
          </NButton>
          <NButton size="small" quaternary :disabled="!store.transcriptText" @click="exportTranscript">
            导出文本
          </NButton>
          <NButton size="small" quaternary @click="router.push('/transcription/history')">
            批量转写
          </NButton>
        </div>
      </div>
    </section>

    <section class="flex-1 min-h-0 grid grid-cols-1 gap-5 xl:grid-cols-[1.2fr_0.8fr]">
      <NCard class="card-main flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <span class="text-sm font-600">实时转写流</span>
        </template>
        <div class="flex-1 min-h-0 overflow-y-auto rounded-2.5 bg-[#fbfdff] p-4">
          <div v-if="store.liveSentences.length === 0" class="text-slate">
            开始录音后，这里会逐句显示模型输出。
          </div>
          <div v-for="(line, index) in store.liveSentences" :key="`${index}-${line}`" class="mb-2.5 rounded-2.5 bg-mist/60 px-4 py-3 text-sm leading-6 text-ink last:mb-0">
            {{ line }}
          </div>
        </div>
      </NCard>

      <NCard class="card-main flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <span class="text-sm font-600">当前策略</span>
        </template>
        <div class="flex-1 min-h-0 flex flex-col overflow-hidden">
          <div class="flex-none grid gap-4 grid-cols-1 mb-4">
            <div class="subtle-panel m-0">
              <div class="text-sm font-600 text-ink">
                攒句输出
              </div>
              <div class="mt-1 text-sm text-slate">
                标点、静音 600ms、最大缓冲 3s
              </div>
            </div>
            <div class="subtle-panel m-0">
              <div class="text-sm font-600 text-ink">
                纠错管道
              </div>
              <div class="mt-1 text-sm text-slate">
                精确词典 → 编辑距离 → 拼音音近
              </div>
            </div>
            <div class="subtle-panel m-0">
              <div class="text-sm font-600 text-ink">
                草稿预览
              </div>
              <div class="mt-1 text-sm text-slate">
                {{ store.draftText || '尚无最新草稿' }}
              </div>
            </div>
          </div>

          <div class="subtle-panel flex-1 flex flex-col min-h-0 m-0">
            <div class="text-sm font-600 text-ink shrink-0">
              完整转写
            </div>
            <div class="mt-2 flex-1 min-h-0 overflow-y-auto whitespace-pre-wrap text-sm leading-6 text-slate">
              {{ store.transcriptText || '完整转写内容会在这里持续累积，页面左侧只保留最近 500 句用于渲染。' }}
            </div>
          </div>
        </div>
      </NCard>
    </section>
  </div>
</template>

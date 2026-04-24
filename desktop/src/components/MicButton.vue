<script setup lang="ts">
import { computed, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { SCENE_MODES } from '@/constants/product'
import { useAudioRecorder } from '@/composables/useAudioRecorder'
import { useTranscribe } from '@/composables/useTranscribe'
import { useAppStore } from '@/stores/app'
import { debugLog } from '@/utils/debug'

const appStore = useAppStore()
const appWindow = getCurrentWindow()
const recorder = useAudioRecorder()
const transcribe = useTranscribe()
let hideWindowAfterStart = false

const isActive = computed(() => appStore.isRecording)
const isCommandMode = computed(() => isActive.value && appStore.voiceCommandActive)
const isCommandProcessing = computed(() => isActive.value && appStore.voiceCommandProcessing)

type VisualState = 'idle' | 'command' | 'meeting' | 'report'
const visualState = computed<VisualState>(() => {
  if (!isActive.value) return 'idle'
  if (isCommandMode.value) return 'command'
  return appStore.sceneMode === SCENE_MODES.MEETING ? SCENE_MODES.MEETING : SCENE_MODES.REPORT
})

const sceneLabel = computed(() => appStore.sceneMode === SCENE_MODES.MEETING ? '会议模式' : '报告模式')
const sceneIcon = computed(() => appStore.sceneMode === SCENE_MODES.MEETING ? '会' : '报')

const statusLabel = computed(() => {
  if (!isActive.value) return `点击开始 · ${sceneLabel.value}`
  if (isCommandProcessing.value) return '理解指令中...'
  if (isCommandMode.value) {
    const remain = Math.max(0, Math.ceil(appStore.voiceCommandRemainingMs / 1000))
    return `等待指令...${remain}s`
  }
  switch (transcribe.status.value) {
    case 'collecting': return `${sceneLabel.value} · 收句中`
    case 'uploading': return `${sceneLabel.value} · 识别中`
    case 'processing': return `${sceneLabel.value} · 处理中`
    default: return `${sceneLabel.value} · 监听中`
  }
})

const isCompact = computed(() => !appStore.expanded)

const levelPercent = computed(() => {
  return Math.min(100, Math.round(transcribe.listeningLevel.value / 0.05 * 100))
})

async function toggle() {
  void debugLog('recorder.toggle', 'toggle record button clicked', { currentlyRecording: isActive.value, scene: appStore.sceneMode })

  if (isActive.value) {
    hideWindowAfterStart = false
    appStore.isRecording = false
  }
  else {
    hideWindowAfterStart = appStore.autoHideWindowOnRecordStart && appStore.expanded
    appStore.isRecording = true
  }
}

watch(() => appStore.isRecording, async (recording) => {
  void debugLog('recorder.state', 'recording state changed', { recording, scene: appStore.sceneMode })

  if (recording) {
    try {
      transcribe.reset()
      await recorder.start((chunk) => transcribe.handleChunk(chunk))
      void debugLog('recorder.state', 'audio recorder started successfully')
      if (hideWindowAfterStart) {
        window.setTimeout(() => {
          void appWindow.hide().catch(() => undefined)
        }, 120)
      }
    }
    catch (e) {
      hideWindowAfterStart = false
      appStore.isRecording = false
      transcribe.lastError.value = e instanceof Error ? e.message : '录音启动失败'
      void debugLog('recorder.error', 'failed to start recorder', e instanceof Error ? { message: e.message, stack: e.stack } : e)
    }
  }
  else {
    hideWindowAfterStart = false
    recorder.stop()
    await transcribe.stopAndFlush()
    void debugLog('recorder.state', 'recorder stopped and flushed')
  }
})
</script>

<template>
  <div class="mic-container" :data-state="visualState">
    <div
      class="level-ring"
      :class="{ active: isActive }"
      :style="{ '--level': `${levelPercent}%` }"
    />
    <div v-if="isCommandMode" class="command-halo" />
    <div v-if="isCommandProcessing" class="processing-ring" />

    <button
      class="mic-btn"
      :class="{ recording: isActive, error: !isActive && !!transcribe.lastError.value }"
      :title="transcribe.lastError.value || statusLabel"
      @click="toggle"
    >
      <svg v-if="!isActive" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z" />
        <path d="M19 10v2a7 7 0 0 1-14 0v-2" />
        <line x1="12" y1="19" x2="12" y2="23" />
        <line x1="8" y1="23" x2="16" y2="23" />
      </svg>
      <svg v-else-if="isCommandProcessing" class="spinner" width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 3a9 9 0 1 0 9 9" />
      </svg>
      <svg v-else-if="isCommandMode" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
        <circle cx="12" cy="12" r="9" />
        <path d="M9.5 9a2.5 2.5 0 1 1 5 0c0 1.7-2.5 2-2.5 4" />
        <line x1="12" y1="17" x2="12.01" y2="17" />
      </svg>
      <!-- 报告模式：文档图标 -->
      <svg v-else-if="appStore.sceneMode === 'report'" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M14 3H7a2 2 0 0 0-2 2v14a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V8z" />
        <polyline points="14 3 14 8 19 8" />
        <line x1="9" y1="13" x2="15" y2="13" />
        <line x1="9" y1="17" x2="15" y2="17" />
      </svg>
      <!-- 会议模式：人群/对话图标 -->
      <svg v-else width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M17 11a4 4 0 1 0-3.2-6.4" />
        <circle cx="9" cy="8" r="4" />
        <path d="M2 21v-1a6 6 0 0 1 6-6h2a6 6 0 0 1 6 6v1" />
        <path d="M22 21v-1a5 5 0 0 0-4-4.9" />
      </svg>
    </button>

    <!-- 模式徽章：左上角小标签，强化视觉区分 -->
    <div v-if="isActive && !isCommandMode" class="scene-badge" :data-scene="appStore.sceneMode">
      {{ sceneIcon }}
    </div>

    <div v-if="!isCompact" class="status-row">
      <span class="status-dot" :class="{ active: isActive, processing: isCommandProcessing }" />
      <span class="status-label">{{ statusLabel }}</span>
    </div>

    <div v-if="!isCompact && transcribe.lastError.value" class="error-text">
      {{ transcribe.lastError.value }}
    </div>

    <div v-if="!isCompact && (isActive || transcribe.totalSegments.value > 0)" class="counters">
      <span>句段 {{ transcribe.totalSegments.value }}</span>
      <span v-if="transcribe.pendingCount.value > 0"> · 队列 {{ transcribe.pendingCount.value }}</span>
    </div>
  </div>
</template>

<style scoped>
.mic-container {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
  position: relative;
  --accent: #94a3b8;
  --accent-soft: rgba(148, 163, 184, 0.35);
  --accent-strong: rgba(148, 163, 184, 0.55);
}

.mic-container[data-state="report"] {
  --accent: #16a34a;
  --accent-soft: rgba(22, 163, 74, 0.32);
  --accent-strong: rgba(22, 163, 74, 0.6);
}

.mic-container[data-state="meeting"] {
  --accent: #c026d3;
  --accent-soft: rgba(192, 38, 211, 0.32);
  --accent-strong: rgba(192, 38, 211, 0.62);
}

.mic-container[data-state="command"] {
  --accent: #2563eb;
  --accent-soft: rgba(37, 99, 235, 0.35);
  --accent-strong: rgba(37, 99, 235, 0.65);
}

.level-ring {
  position: absolute;
  width: 72px;
  height: 72px;
  border-radius: 50%;
  border: 1.5px solid transparent;
  top: 32px;
  left: 50%;
  transform: translate(-50%, -50%);
  transition: border-color 0.2s, box-shadow 0.2s;
  pointer-events: none;
}

.level-ring.active {
  border-color: var(--accent-soft);
  box-shadow: 0 0 calc(var(--level, 0%) * 0.2 + 4px) var(--accent-strong);
  animation: pulse-ring 1.5s ease-in-out infinite;
}

.command-halo {
  position: absolute;
  width: 90px;
  height: 90px;
  border-radius: 50%;
  border: 2px dashed var(--accent-strong);
  top: 32px;
  left: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
  animation: command-spin 3.6s linear infinite, command-pulse 1.4s ease-in-out infinite;
}

.processing-ring {
  position: absolute;
  width: 96px;
  height: 96px;
  border-radius: 50%;
  border: 3px solid rgba(99, 102, 241, 0.18);
  border-top-color: #6366f1;
  border-right-color: #818cf8;
  top: 32px;
  left: 50%;
  transform: translate(-50%, -50%);
  pointer-events: none;
  animation: processing-spin 0.85s linear infinite;
  z-index: 2;
}

@keyframes processing-spin {
  to { transform: translate(-50%, -50%) rotate(360deg); }
}

.spinner {
  animation: spinner-rotate 0.85s linear infinite;
}

@keyframes spinner-rotate {
  to { transform: rotate(360deg); }
}

.scene-badge {
  position: absolute;
  top: 6px;
  left: calc(50% + 18px);
  width: 22px;
  height: 22px;
  border-radius: 50%;
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 12px;
  font-weight: 700;
  color: white;
  border: 2px solid white;
  box-shadow: 0 2px 6px rgba(15, 23, 42, 0.25);
  z-index: 3;
  pointer-events: none;
  user-select: none;
}

.scene-badge[data-scene="report"] {
  background: #16a34a;
}

.scene-badge[data-scene="meeting"] {
  background: #c026d3;
}

@keyframes pulse-ring {
  0%, 100% { transform: translate(-50%, -50%) scale(1); opacity: 0.6; }
  50% { transform: translate(-50%, -50%) scale(1.08); opacity: 1; }
}

@keyframes command-spin {
  to { transform: translate(-50%, -50%) rotate(360deg); }
}

@keyframes command-pulse {
  0%, 100% { opacity: 0.55; }
  50% { opacity: 1; }
}

.mic-btn {
  width: 64px;
  height: 64px;
  border-radius: 50%;
  border: 2px solid rgba(67, 82, 102, 0.2);
  background: rgba(67, 82, 102, 0.06);
  color: #435266;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  transition: background 0.25s ease, border-color 0.25s ease, box-shadow 0.25s ease, color 0.25s ease, transform 0.2s ease;
  position: relative;
  z-index: 1;
}

.mic-btn:hover {
  background: rgba(67, 82, 102, 0.12);
  transform: scale(1.04);
}

.mic-btn.recording {
  background: var(--accent);
  border-color: var(--accent);
  color: white;
  box-shadow: 0 4px 16px var(--accent-soft);
}

.mic-container[data-state="command"] .mic-btn.recording {
  background: linear-gradient(135deg, #3b82f6, #6366f1);
  animation: command-breath 1.4s ease-in-out infinite;
}

@keyframes command-breath {
  0%, 100% { box-shadow: 0 4px 14px rgba(37, 99, 235, 0.45); }
  50% { box-shadow: 0 6px 22px rgba(99, 102, 241, 0.7); }
}

.mic-btn.error {
  border-color: rgba(245, 158, 11, 0.8);
  box-shadow: 0 0 0 4px rgba(245, 158, 11, 0.16);
  animation: error-shake 0.5s ease;
}

@keyframes error-shake {
  0%, 100% { transform: translateX(0); }
  20% { transform: translateX(-3px); }
  40% { transform: translateX(3px); }
  60% { transform: translateX(-2px); }
  80% { transform: translateX(2px); }
}

.mic-btn.recording:hover {
  filter: brightness(0.92);
}

.status-row {
  display: flex;
  align-items: center;
  gap: 6px;
}

.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: #94a3b8;
  transition: background 0.2s;
}

.status-dot.active {
  background: var(--accent);
  animation: blink 1s ease-in-out infinite;
}

.status-dot.processing {
  background: #6366f1;
  animation: blink 0.5s ease-in-out infinite;
}

@keyframes blink {
  0%, 100% { opacity: 1; }
  50% { opacity: 0.3; }
}

.status-label {
  font-size: 12px;
  color: #64748b;
}

.error-text {
  font-size: 11px;
  color: #ef4444;
  max-width: 280px;
  text-align: center;
  word-break: break-all;
}

.counters {
  font-size: 11px;
  color: #94a3b8;
}
</style>

<script setup lang="ts">
import { computed, watch } from 'vue'
import { getCurrentWindow } from '@tauri-apps/api/window'
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
const statusLabel = computed(() => {
  if (!isActive.value) return '点击开始'
  switch (transcribe.status.value) {
    case 'collecting': return '正在收句...'
    case 'uploading': return '识别中...'
    case 'processing': return '处理中...'
    default: return '监听中'
  }
})

const isCompact = computed(() => !appStore.expanded)

const levelPercent = computed(() => {
  return Math.min(100, Math.round(transcribe.listeningLevel.value / 0.05 * 100))
})

async function toggle() {
  void debugLog('recorder.toggle', 'toggle record button clicked', { currentlyRecording: isActive.value })

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
  void debugLog('recorder.state', 'recording state changed', { recording })

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
    transcribe.stopAndFlush()
    void debugLog('recorder.state', 'recorder stopped and flushed')
  }
})
</script>

<template>
  <div class="mic-container">
    <!-- Level ring -->
    <div
      class="level-ring"
      :class="{ active: isActive }"
      :style="{ '--level': levelPercent + '%' }"
    />

    <!-- Main button -->
    <button
      class="mic-btn"
      :class="{ recording: isActive, error: !isActive && !!transcribe.lastError.value }"
      :title="transcribe.lastError.value || statusLabel"
      @click="toggle"
    >
      <svg v-if="!isActive" width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
        <path d="M12 1a3 3 0 0 0-3 3v8a3 3 0 0 0 6 0V4a3 3 0 0 0-3-3z"/>
        <path d="M19 10v2a7 7 0 0 1-14 0v-2"/>
        <line x1="12" y1="19" x2="12" y2="23"/>
        <line x1="8" y1="23" x2="16" y2="23"/>
      </svg>
      <svg v-else width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" stroke-linejoin="round">
        <rect x="6" y="6" width="12" height="12" rx="2"/>
      </svg>
    </button>

    <!-- Status text -->
    <div v-if="!isCompact" class="status-row">
      <span class="status-dot" :class="{ active: isActive }" />
      <span class="status-label">{{ statusLabel }}</span>
    </div>

    <!-- Error display -->
    <div v-if="!isCompact && transcribe.lastError.value" class="error-text">
      {{ transcribe.lastError.value }}
    </div>

    <!-- Counters -->
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
}

.level-ring {
  position: absolute;
  width: 68px;
  height: 68px;
  border-radius: 50%;
  border: 2px solid transparent;
  top: -2px;
  transition: border-color 0.2s, box-shadow 0.2s;
  pointer-events: none;
}

.level-ring.active {
  border-color: rgba(239, 68, 68, 0.4);
  box-shadow: 0 0 calc(var(--level, 0%) * 0.2 + 4px) rgba(239, 68, 68, 0.3);
  animation: pulse-ring 1.5s ease-in-out infinite;
}

@keyframes pulse-ring {
  0%, 100% { transform: scale(1); opacity: 0.6; }
  50% { transform: scale(1.08); opacity: 1; }
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
  transition: all 0.25s ease;
  position: relative;
  z-index: 1;
}

.mic-btn:hover {
  background: rgba(67, 82, 102, 0.12);
  transform: scale(1.04);
}

.mic-btn.recording {
  background: #ef4444;
  border-color: #ef4444;
  color: white;
  box-shadow: 0 4px 16px rgba(239, 68, 68, 0.35);
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
  background: #dc2626;
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
  background: #ef4444;
  animation: blink 1s ease-in-out infinite;
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

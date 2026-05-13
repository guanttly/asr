<script setup lang="ts">
import { computed, onBeforeUnmount, ref } from 'vue'
import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { SCENE_MODES } from '@/constants/product'
import MicButton from './MicButton.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const appWindow = getCurrentWindow()
const dragHandleActive = ref(false)

const DRAG_HANDLE_RADIUS_RATIO = 0.45
const DRAG_START_THRESHOLD_PX = 4

let pendingDrag: { startX: number, startY: number } | null = null
let dragInProgress = false
let suppressNextClick = false

const orbState = computed(() => {
  if (!appStore.isRecording) return 'idle'
  if (appStore.voiceCommandActive) return 'command'
  return appStore.sceneMode === SCENE_MODES.MEETING ? SCENE_MODES.MEETING : SCENE_MODES.REPORT
})

async function openSettings() {
  await invoke('open_settings_window').catch(() => undefined)
}

function isFormTarget(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest('input, textarea, select, a, [data-no-drag]'))
}

function isInsideCenterDragHandle(event: MouseEvent, element: HTMLElement) {
  const rect = element.getBoundingClientRect()
  const centerX = rect.left + rect.width / 2
  const centerY = rect.top + rect.height / 2
  const deltaX = event.clientX - centerX
  const deltaY = event.clientY - centerY
  const radius = Math.min(rect.width, rect.height) * DRAG_HANDLE_RADIUS_RATIO
  return Math.hypot(deltaX, deltaY) <= radius
}

function updateDragHandle(event: MouseEvent) {
  const element = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  dragHandleActive.value = element ? isInsideCenterDragHandle(event, element) : false
}

function clearPendingDrag() {
  pendingDrag = null
  window.removeEventListener('mousemove', handleDragMove)
  window.removeEventListener('mouseup', endDrag)
}

function endDrag() {
  clearPendingDrag()
  if (dragInProgress) {
    dragInProgress = false
    // Tauri 没有 stopDragging（系统接管 mouseup 自动结束），
    // Electron Win7 用此调用通知主进程结束 cursor 轮询拖拽。
    const stop = (appWindow as { stopDragging?: () => Promise<void> }).stopDragging
    if (typeof stop === 'function')
      void stop.call(appWindow).catch(() => undefined)
  }
}

function handleDragMove(event: MouseEvent) {
  if (!pendingDrag)
    return

  const moved = Math.hypot(event.clientX - pendingDrag.startX, event.clientY - pendingDrag.startY)
  if (moved < DRAG_START_THRESHOLD_PX)
    return

  pendingDrag = null
  window.removeEventListener('mousemove', handleDragMove)
  dragInProgress = true
  suppressNextClick = true
  window.setTimeout(() => {
    suppressNextClick = false
  }, 500)
  void appWindow.startDragging().catch(() => undefined)
}

function startDrag(event: MouseEvent) {
  if (isFormTarget(event.target))
    return

  const element = event.currentTarget instanceof HTMLElement ? event.currentTarget : null
  if (!element || !isInsideCenterDragHandle(event, element))
    return

  pendingDrag = { startX: event.clientX, startY: event.clientY }
  window.addEventListener('mousemove', handleDragMove)
  window.addEventListener('mouseup', endDrag, { once: true })
}

function suppressClickAfterDrag(event: MouseEvent) {
  if (!suppressNextClick)
    return

  suppressNextClick = false
  event.preventDefault()
  event.stopPropagation()
}

onBeforeUnmount(() => {
  clearPendingDrag()
  if (dragInProgress) {
    dragInProgress = false
    const stop = (appWindow as { stopDragging?: () => Promise<void> }).stopDragging
    if (typeof stop === 'function')
      void stop.call(appWindow).catch(() => undefined)
  }
})
</script>

<template>
  <div class="recorder-shell" title="右键打开设置" @contextmenu.prevent="openSettings">
    <div
      class="orb-frame"
      :class="{ 'drag-handle': dragHandleActive }"
      :data-state="orbState"
      @mousemove="updateDragHandle"
      @mouseleave="dragHandleActive = false"
      @mousedown.left="startDrag"
      @click.capture="suppressClickAfterDrag"
    >
      <MicButton />
    </div>
  </div>
</template>

<style scoped>
.recorder-shell {
  position: relative;
  width: 100%;
  height: 100%;
  display: flex;
  align-items: center;
  justify-content: center;
  background: transparent;
}

.orb-frame {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100px;
  height: 100px;
  border-radius: 50%;
  overflow: visible;
  border: 1.5px solid rgba(15, 118, 110, 0.28);
  background:
    radial-gradient(circle at 30% 28%, rgba(255, 255, 255, 0.96) 0%, rgba(240, 253, 250, 0.88) 60%, rgba(204, 251, 241, 0.72) 100%);
  box-shadow:
    0 6px 20px rgba(15, 118, 110, 0.18),
    0 1px 0 rgba(255, 255, 255, 0.95) inset,
    0 0 0 1px rgba(255, 255, 255, 0.5) inset;
  cursor: pointer;
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.3s ease, transform 0.25s ease;
  animation: orb-breath 3.2s ease-in-out infinite;
}

.orb-frame :deep(*) {
  cursor: pointer;
}

.orb-frame:hover {
  transform: translateY(-1px);
  border-color: rgba(15, 118, 110, 0.5);
  box-shadow:
    0 10px 28px rgba(15, 118, 110, 0.28),
    0 1px 0 rgba(255, 255, 255, 0.95) inset;
}

.orb-frame::before {
  content: '';
  position: absolute;
  inset: -6px;
  border-radius: 50%;
  background: radial-gradient(circle at center, rgba(20, 184, 166, 0.18) 0%, rgba(20, 184, 166, 0) 65%);
  opacity: 0.65;
  pointer-events: none;
  animation: orb-halo 3.2s ease-in-out infinite;
  z-index: -1;
}

@keyframes orb-breath {
  0%, 100% {
    box-shadow:
      0 6px 20px rgba(15, 118, 110, 0.18),
      0 1px 0 rgba(255, 255, 255, 0.95) inset,
      0 0 0 1px rgba(255, 255, 255, 0.5) inset;
  }
  50% {
    box-shadow:
      0 8px 26px rgba(15, 118, 110, 0.28),
      0 1px 0 rgba(255, 255, 255, 0.95) inset,
      0 0 0 1px rgba(255, 255, 255, 0.5) inset;
  }
}

@keyframes orb-halo {
  0%, 100% { transform: scale(1); opacity: 0.55; }
  50%      { transform: scale(1.08); opacity: 0.85; }
}

.orb-frame[data-state="report"],
.orb-frame[data-state="meeting"],
.orb-frame[data-state="command"] {
  animation: none;
}

.orb-frame[data-state="report"]::before,
.orb-frame[data-state="meeting"]::before,
.orb-frame[data-state="command"]::before {
  display: none;
}

.orb-frame[data-state="report"] {
  border: 2px solid #16a34a;
  box-shadow: 0 6px 22px rgba(22, 163, 74, 0.28), inset 0 0 0 1px rgba(22, 163, 74, 0.08);
  background: radial-gradient(circle at center, rgba(220, 252, 231, 0.7), rgba(255, 255, 255, 0.95));
}

.orb-frame[data-state="meeting"] {
  border: 2px dashed #c026d3;
  box-shadow: 0 6px 22px rgba(192, 38, 211, 0.32), inset 0 0 0 1px rgba(192, 38, 211, 0.08);
  background: radial-gradient(circle at center, rgba(250, 232, 255, 0.75), rgba(255, 255, 255, 0.95));
}

.orb-frame[data-state="command"] {
  border: 2px solid #2563eb;
  box-shadow: 0 6px 26px rgba(37, 99, 235, 0.4), 0 0 0 4px rgba(37, 99, 235, 0.15);
  background: radial-gradient(circle at center, rgba(219, 234, 254, 0.85), rgba(255, 255, 255, 0.95));
}
</style>

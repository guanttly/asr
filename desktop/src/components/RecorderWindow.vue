<script setup lang="ts">
import { computed } from 'vue'
import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import { SCENE_MODES } from '@/constants/product'
import MicButton from './MicButton.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const appWindow = getCurrentWindow()

const orbState = computed(() => {
  if (!appStore.isRecording) return 'idle'
  if (appStore.voiceCommandActive) return 'command'
  return appStore.sceneMode === SCENE_MODES.MEETING ? SCENE_MODES.MEETING : SCENE_MODES.REPORT
})

async function openSettings() {
  await invoke('open_settings_window').catch(() => undefined)
}

function isInteractiveTarget(target: EventTarget | null) {
  return target instanceof Element && Boolean(target.closest('button, input, textarea, select, a, [data-no-drag]'))
}

async function startDrag(event: MouseEvent) {
  if (isInteractiveTarget(event.target))
    return
  await appWindow.startDragging().catch(() => undefined)
}
</script>

<template>
  <div class="recorder-shell" title="右键打开设置" @contextmenu.prevent="openSettings" @mousedown.left="startDrag">
    <div class="orb-frame" :data-state="orbState">
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
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.3s ease, transform 0.25s ease;
  animation: orb-breath 3.2s ease-in-out infinite;
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

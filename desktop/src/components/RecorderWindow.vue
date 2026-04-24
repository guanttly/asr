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
  border: 1.5px solid rgba(148, 163, 184, 0.32);
  background: rgba(255, 255, 255, 0.92);
  transition: border-color 0.3s ease, box-shadow 0.3s ease, background 0.3s ease;
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

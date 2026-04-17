<script setup lang="ts">
import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'
import MicButton from './MicButton.vue'

const appWindow = getCurrentWindow()

async function openSettings() {
  await invoke('open_settings_window').catch(() => undefined)
}

function isInteractiveTarget(target: EventTarget | null) {
  return target instanceof HTMLElement && Boolean(target.closest('button, input, textarea, select, a, [data-no-drag]'))
}

async function startDrag(event: MouseEvent) {
  if (isInteractiveTarget(event.target))
    return
  await appWindow.startDragging().catch(() => undefined)
}
</script>

<template>
  <div class="recorder-shell" title="右键打开设置" @contextmenu.prevent="openSettings" @mousedown.left="startDrag">
    <div class="orb-frame">
      <div class="orb-glow" />
      <MicButton />
    </div>
  </div>
</template>

<style scoped>
.recorder-shell {
  position: relative;
  width: 100%;
  height: 100%;
  padding: 8px;
  border-radius: 999px;
  overflow: hidden;
  background:
    radial-gradient(circle at 30% 25%, rgba(255, 255, 255, 0.92), rgba(255, 255, 255, 0.7) 38%, rgba(208, 239, 233, 0.92) 100%),
    linear-gradient(145deg, rgba(15, 118, 110, 0.14), rgba(6, 95, 70, 0.04));
  border: 1px solid rgba(15, 118, 110, 0.18);
  box-shadow: 0 14px 34px rgba(15, 118, 110, 0.18), inset 0 1px 0 rgba(255, 255, 255, 0.6);
}

.orb-frame {
  position: relative;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 100%;
  height: 100%;
  border-radius: 999px;
  overflow: hidden;
}

.orb-glow {
  position: absolute;
  inset: 14px;
  border-radius: 999px;
  background: radial-gradient(circle at 50% 35%, rgba(15, 118, 110, 0.12), transparent 68%);
  pointer-events: none;
}
</style>
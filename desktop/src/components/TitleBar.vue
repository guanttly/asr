<script setup lang="ts">
import { getCurrentWindow } from '@tauri-apps/api/window'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const appWindow = getCurrentWindow()

function startDrag() {
  appWindow.startDragging()
}

function toggleExpand() {
  appStore.expanded = !appStore.expanded
}

function minimize() {
  appWindow.minimize()
}

function closeWindow() {
  appWindow.close()
}
</script>

<template>
  <div class="title-bar" @mousedown="startDrag">
    <div class="title-left">
      <span class="title-text">语音速录</span>
    </div>
    <div class="title-actions" @mousedown.stop>
      <button class="title-btn" title="展开/收起" @click="toggleExpand">
        <span v-if="appStore.expanded" class="icon">▾</span>
        <span v-else class="icon">▸</span>
      </button>
      <button class="title-btn" title="最小化到托盘" @click="minimize">
        <span class="icon">—</span>
      </button>
      <button class="title-btn danger" title="关闭" @click="closeWindow">
        <span class="icon">×</span>
      </button>
    </div>
  </div>
</template>

<style scoped>
.title-bar {
  display: flex;
  align-items: center;
  justify-content: space-between;
  height: 32px;
  padding: 0 10px;
  background: rgba(255, 255, 255, 0.5);
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  cursor: grab;
  flex-shrink: 0;
}

.title-bar:active {
  cursor: grabbing;
}

.title-text {
  font-size: 12px;
  color: #435266;
  font-weight: 500;
  letter-spacing: 0.5px;
}

.title-actions {
  display: flex;
  gap: 2px;
}

.title-btn {
  width: 24px;
  height: 24px;
  border: none;
  background: transparent;
  border-radius: 6px;
  cursor: pointer;
  display: flex;
  align-items: center;
  justify-content: center;
  color: #435266;
  font-size: 12px;
  transition: background 0.15s;
}

.title-btn:hover {
  background: rgba(0, 0, 0, 0.06);
}

.title-btn.danger:hover {
  background: rgba(239, 68, 68, 0.12);
  color: #b91c1c;
}

.icon {
  line-height: 1;
}
</style>

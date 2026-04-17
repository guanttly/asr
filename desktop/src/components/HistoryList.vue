<script setup lang="ts">
import { useInjector } from '@/composables/useInjector'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const { injectText } = useInjector()

async function copyToClipboard(text: string) {
  await injectText(text)
}
</script>

<template>
  <div class="history-list">
    <div v-if="appStore.history.length === 0" class="empty-state">
      暂无转写记录
    </div>
    <template v-else>
      <div class="list-header">
        <span class="list-count">共 {{ appStore.history.length }} 条</span>
        <button class="clear-btn" @click="appStore.clearHistory()">清空</button>
      </div>
      <div
        v-for="(item, index) in appStore.history"
        :key="index"
        class="history-item"
        title="点击注入到光标处"
        @click="copyToClipboard(item)"
      >
        <span class="item-text">{{ item }}</span>
        <span class="item-action">↗</span>
      </div>
    </template>
  </div>
</template>

<style scoped>
.history-list {
  padding: 8px;
}

.empty-state {
  padding: 24px 0;
  text-align: center;
  font-size: 12px;
  color: #94a3b8;
}

.list-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0 4px 6px;
}

.list-count {
  font-size: 11px;
  color: #94a3b8;
}

.clear-btn {
  font-size: 11px;
  color: #ef4444;
  background: none;
  border: none;
  cursor: pointer;
  padding: 2px 6px;
  border-radius: 4px;
}

.clear-btn:hover {
  background: rgba(239, 68, 68, 0.06);
}

.history-item {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 8px 10px;
  margin-bottom: 4px;
  border-radius: 8px;
  background: rgba(0, 0, 0, 0.02);
  cursor: pointer;
  transition: background 0.15s;
}

.history-item:hover {
  background: rgba(15, 118, 110, 0.06);
}

.item-text {
  font-size: 13px;
  color: #16202c;
  flex: 1;
  word-break: break-all;
}

.item-action {
  font-size: 12px;
  color: #94a3b8;
  margin-left: 8px;
  flex-shrink: 0;
}
</style>

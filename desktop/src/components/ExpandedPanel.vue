<script setup lang="ts">
import { ref } from 'vue'
import SettingsPanel from './SettingsPanel.vue'
import HistoryList from './HistoryList.vue'

const activeTab = ref<'history' | 'settings'>('history')
</script>

<template>
  <div class="expanded-panel">
    <div class="tab-bar">
      <button
        class="tab-item"
        :class="{ active: activeTab === 'history' }"
        @click="activeTab = 'history'"
      >
        历史记录
      </button>
      <button
        class="tab-item"
        :class="{ active: activeTab === 'settings' }"
        @click="activeTab = 'settings'"
      >
        设置
      </button>
    </div>

    <div class="tab-content">
      <HistoryList v-if="activeTab === 'history'" />
      <SettingsPanel v-if="activeTab === 'settings'" />
    </div>
  </div>
</template>

<style scoped>
.expanded-panel {
  width: 100%;
  flex: 1;
  display: flex;
  flex-direction: column;
  min-height: 0;
  border-top: 1px solid rgba(0, 0, 0, 0.06);
}

.tab-bar {
  display: flex;
  gap: 0;
  border-bottom: 1px solid rgba(0, 0, 0, 0.06);
  flex-shrink: 0;
}

.tab-item {
  flex: 1;
  padding: 6px 0;
  font-size: 12px;
  color: #94a3b8;
  background: none;
  border: none;
  border-bottom: 2px solid transparent;
  cursor: pointer;
  transition: all 0.15s;
}

.tab-item:hover {
  color: #475569;
}

.tab-item.active {
  color: #0f766e;
  border-bottom-color: #0f766e;
}

.tab-content {
  flex: 1;
  overflow-y: auto;
  min-height: 0;
}
</style>

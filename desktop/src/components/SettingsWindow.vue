<script setup lang="ts">
import { computed, ref } from 'vue'
import HistoryList from './HistoryList.vue'
import SettingsPanel from './SettingsPanel.vue'
import { useAppStore } from '@/stores/app'

const appStore = useAppStore()
const activeTab = ref<'history' | 'settings'>('settings')
const userLabel = computed(() => appStore.displayName || appStore.username || '未连接')
const machineSnippet = computed(() => appStore.machineCode ? appStore.machineCode.slice(0, 12) : '未生成')
</script>

<template>
  <div class="settings-window">
    <header class="hero">
      <img src="/logo.png" alt="ASR" class="hero-logo">
      <div class="hero-copy">
        <p class="hero-tag">Desktop Voice Dictation</p>
        <h1>语音速录助手</h1>
        <p class="hero-subtitle">{{ userLabel }} · {{ machineSnippet }}</p>
      </div>
    </header>

    <div class="tab-bar">
      <button class="tab-item" :class="{ active: activeTab === 'settings' }" @click="activeTab = 'settings'">
        连接设置
      </button>
      <button class="tab-item" :class="{ active: activeTab === 'history' }" @click="activeTab = 'history'">
        转写记录
      </button>
    </div>

    <div class="tab-content">
      <SettingsPanel v-if="activeTab === 'settings'" />
      <HistoryList v-else />
    </div>
  </div>
</template>

<style scoped>
.settings-window {
  display: flex;
  flex-direction: column;
  height: 100%;
  background:
    radial-gradient(circle at top left, rgba(15, 118, 110, 0.16), transparent 32%),
    linear-gradient(180deg, rgba(248, 250, 252, 0.98), rgba(241, 245, 249, 0.96));
}

.hero {
  display: flex;
  align-items: center;
  gap: 14px;
  padding: 20px 20px 16px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.16);
}

.hero-logo {
  width: 56px;
  height: 56px;
  border-radius: 16px;
  box-shadow: 0 12px 24px rgba(15, 118, 110, 0.18);
}

.hero-copy h1 {
  font-size: 20px;
  font-weight: 700;
  color: #0f172a;
}

.hero-tag {
  font-size: 11px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #0f766e;
  margin-bottom: 4px;
}

.hero-subtitle {
  margin-top: 6px;
  font-size: 12px;
  color: #64748b;
}

.tab-bar {
  display: flex;
  gap: 8px;
  padding: 12px 16px 0;
}

.tab-item {
  flex: 1;
  border: none;
  border-radius: 12px 12px 0 0;
  padding: 10px 14px;
  background: rgba(255, 255, 255, 0.7);
  color: #64748b;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.2s ease, color 0.2s ease;
}

.tab-item.active {
  background: rgba(255, 255, 255, 0.96);
  color: #0f766e;
  box-shadow: 0 -1px 0 rgba(15, 118, 110, 0.1), 0 8px 18px rgba(148, 163, 184, 0.14);
}

.tab-content {
  flex: 1;
  margin: 0 16px 16px;
  overflow: hidden;
  border-radius: 0 16px 16px 16px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 20px 40px rgba(148, 163, 184, 0.16);
}
</style>
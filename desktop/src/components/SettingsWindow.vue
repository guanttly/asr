<script setup lang="ts">
import { computed, ref } from 'vue'
import AppConfirm from './AppConfirm.vue'
import HistoryList from './HistoryList.vue'
import MeetingsList from './MeetingsList.vue'
import SettingsPanel from './SettingsPanel.vue'
import { PRODUCT_CAPABILITY_KEYS } from '@/constants/product'
import { useAppStore } from '@/stores/app'

type MainTab = 'history' | 'meetings'

const appStore = useAppStore()
const settingsOpen = ref(false)
const meetingEnabled = computed(() => appStore.hasCapability(PRODUCT_CAPABILITY_KEYS.MEETING))
const activeTab = ref<MainTab>('history')

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
      <button
        class="hero-gear"
        :class="{ active: settingsOpen }"
        type="button"
        :aria-label="settingsOpen ? '收起连接设置' : '展开连接设置'"
        @click="settingsOpen = !settingsOpen"
      >
        <svg viewBox="0 0 24 24" width="18" height="18">
          <path
            fill="currentColor"
            d="M19.14 12.94a7.5 7.5 0 0 0 .05-1.88l2.04-1.6a.5.5 0 0 0 .12-.64l-1.93-3.34a.5.5 0 0 0-.6-.22l-2.4.97a7.6 7.6 0 0 0-1.62-.94l-.36-2.55a.5.5 0 0 0-.5-.43h-3.86a.5.5 0 0 0-.5.43l-.36 2.55c-.58.23-1.13.55-1.62.94l-2.4-.97a.5.5 0 0 0-.6.22L2.66 8.82a.5.5 0 0 0 .12.64l2.04 1.6a7.5 7.5 0 0 0 0 1.88l-2.04 1.6a.5.5 0 0 0-.12.64l1.93 3.34a.5.5 0 0 0 .6.22l2.4-.97c.49.39 1.04.71 1.62.94l.36 2.55a.5.5 0 0 0 .5.43h3.86a.5.5 0 0 0 .5-.43l.36-2.55c.58-.23 1.13-.55 1.62-.94l2.4.97a.5.5 0 0 0 .6-.22l1.93-3.34a.5.5 0 0 0-.12-.64l-2.04-1.6zM12 15.5a3.5 3.5 0 1 1 0-7a3.5 3.5 0 0 1 0 7"
          />
        </svg>
      </button>
    </header>

    <div class="tab-bar">
      <button class="tab-item" :class="{ active: activeTab === 'history' }" @click="activeTab = 'history'">
        转写记录
      </button>
      <button
        v-if="meetingEnabled"
        class="tab-item"
        :class="{ active: activeTab === 'meetings' }"
        @click="activeTab = 'meetings'"
      >
        会议纪要
      </button>
    </div>

    <div class="tab-content">
      <Transition name="tab-swap" mode="out-in">
        <KeepAlive>
          <component :is="activeTab === 'history' ? HistoryList : MeetingsList" :key="activeTab" />
        </KeepAlive>
      </Transition>
    </div>

    <Transition name="settings-fade">
      <div v-if="settingsOpen" class="settings-overlay" @click.self="settingsOpen = false">
        <div class="settings-sheet">
          <header class="sheet-head">
            <span class="sheet-title">连接设置</span>
            <button class="sheet-close" type="button" @click="settingsOpen = false">×</button>
          </header>
          <div class="sheet-body">
            <SettingsPanel />
          </div>
        </div>
      </div>
    </Transition>

    <AppConfirm />
  </div>
</template>

<style scoped>
.settings-window {
  position: relative;
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
  width: 48px;
  height: 48px;
  border-radius: 14px;
  box-shadow: 0 12px 24px rgba(15, 118, 110, 0.18);
  flex-shrink: 0;
}

.hero-copy {
  flex: 1;
  min-width: 0;
}

.hero-copy h1 {
  font-size: 18px;
  font-weight: 700;
  color: #0f172a;
  margin: 0;
}

.hero-tag {
  font-size: 10px;
  letter-spacing: 0.14em;
  text-transform: uppercase;
  color: #0f766e;
  margin: 0 0 4px;
}

.hero-subtitle {
  margin: 4px 0 0;
  font-size: 11px;
  color: #64748b;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hero-gear {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  justify-content: center;
  width: 36px;
  height: 36px;
  border-radius: 50%;
  background: rgba(255, 255, 255, 0.84);
  border: 1px solid rgba(148, 163, 184, 0.24);
  color: #475569;
  cursor: pointer;
  transition: transform 0.4s ease, color 0.18s ease, background 0.18s ease, border-color 0.18s ease;
}

.hero-gear:hover {
  background: #ffffff;
  color: #0f766e;
  border-color: rgba(15, 118, 110, 0.4);
}

.hero-gear.active {
  background: #0f766e;
  color: #ffffff;
  border-color: #0f766e;
  transform: rotate(60deg);
}

.tab-bar {
  display: flex;
  gap: 8px;
  padding: 12px 16px 0;
}

.tab-item {
  position: relative;
  border: none;
  border-radius: 12px 12px 0 0;
  padding: 10px 18px;
  background: rgba(255, 255, 255, 0.7);
  color: #64748b;
  font-size: 13px;
  cursor: pointer;
  transition: background 0.2s ease, color 0.2s ease;
}

.tab-item.active {
  background: rgba(255, 255, 255, 0.96);
  color: #0f766e;
  font-weight: 600;
  box-shadow: 0 -1px 0 rgba(15, 118, 110, 0.1), 0 8px 18px rgba(148, 163, 184, 0.14);
}

.tab-content {
  flex: 1;
  position: relative;
  margin: 0 16px 16px;
  overflow: hidden;
  border-radius: 0 16px 16px 16px;
  background: rgba(255, 255, 255, 0.96);
  box-shadow: 0 20px 40px rgba(148, 163, 184, 0.16);
}

.tab-swap-enter-active,
.tab-swap-leave-active {
  transition: opacity 0.18s ease, transform 0.18s ease;
}

.tab-swap-enter-from {
  opacity: 0;
  transform: translateY(6px);
}

.tab-swap-leave-to {
  opacity: 0;
  transform: translateY(-6px);
}

.settings-overlay {
  position: absolute;
  inset: 0;
  z-index: 4000;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(6px);
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding: 16px;
}

.settings-sheet {
  width: 100%;
  max-width: 460px;
  max-height: calc(100% - 32px);
  background: #ffffff;
  border-radius: 18px;
  box-shadow: 0 32px 60px rgba(15, 23, 42, 0.18);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.sheet-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.7);
}

.sheet-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}

.sheet-close {
  border: 0;
  background: transparent;
  cursor: pointer;
  font-size: 20px;
  color: #94a3b8;
  line-height: 1;
}

.sheet-close:hover {
  color: #475569;
}

.sheet-body {
  flex: 1;
  overflow-y: auto;
  padding: 8px 4px;
}

.settings-fade-enter-active,
.settings-fade-leave-active {
  transition: opacity 0.2s ease;
}

.settings-fade-enter-active .settings-sheet,
.settings-fade-leave-active .settings-sheet {
  transition: transform 0.22s cubic-bezier(0.32, 0.72, 0.32, 1);
}

.settings-fade-enter-from,
.settings-fade-leave-to {
  opacity: 0;
}

.settings-fade-enter-from .settings-sheet,
.settings-fade-leave-to .settings-sheet {
  transform: translateY(-12px) scale(0.98);
}
</style>

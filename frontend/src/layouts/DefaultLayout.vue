<script setup lang="ts">
import type { MenuOption } from 'naive-ui'

import { computed } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const userStore = useUserStore()
const { status: businessSocketStatus, subscribedTopicCount, lastPongAt } = useBusinessSocket()

const menuOptions: MenuOption[] = [
  { label: '数据看板', key: '/dashboard' },
  { label: '转写工作台', key: '/transcription' },
  { label: '转写历史', key: '/transcription/history' },
  { label: '会议管理', key: '/meetings' },
  { label: '术语库管理', key: '/terminology' },
  { label: '纠错规则', key: '/terminology/rules' },
  { label: '用户管理', key: '/system/users' },
  { label: '角色管理', key: '/system/roles' },
]

const currentPath = computed(() => route.path)
const contentWrapperClass = computed(() => route.meta.pageManagedScroll ? 'content-frame' : 'content-scroll')
const businessSocketLabel = computed(() => {
  const count = subscribedTopicCount.value
  const suffix = count > 0 ? ` · ${count} 个主题` : ''
  switch (businessSocketStatus.value) {
    case 'connected':
      return `业务总线已连接${suffix}`
    case 'connecting':
      return '业务总线连接中'
    case 'reconnecting':
      return '业务总线重连中'
    case 'error':
      return '业务总线异常'
    default:
      return '业务总线未连接'
  }
})
const businessSocketBadgeClass = computed(() => {
  switch (businessSocketStatus.value) {
    case 'connected':
      return 'bg-teal/12 text-teal'
    case 'connecting':
    case 'reconnecting':
      return 'bg-amber-500/12 text-amber-700'
    case 'error':
      return 'bg-red-500/12 text-red-600'
    default:
      return 'bg-mist/80 text-slate'
  }
})
const businessSocketDotClass = computed(() => {
  switch (businessSocketStatus.value) {
    case 'connected':
      return 'bg-teal'
    case 'connecting':
    case 'reconnecting':
      return 'bg-amber-500'
    case 'error':
      return 'bg-red-500'
    default:
      return 'bg-slate/50'
  }
})
const businessSocketTooltip = computed(() => lastPongAt.value
  ? `${businessSocketLabel.value}，最近心跳 ${new Date(lastPongAt.value).toLocaleString('zh-CN', { hour12: false })}`
  : businessSocketLabel.value)

function handleMenuSelect(key: string) {
  router.push(key)
}

function handleLogout() {
  userStore.logout()
  router.push('/login')
}
</script>

<template>
  <NLayout has-sider class="page-shell overflow-hidden" content-style="overflow: hidden; display: flex;">
    <NLayoutSider
      bordered
      collapse-mode="width"
      :collapsed-width="74"
      :width="244"
      :collapsed="appStore.siderCollapsed"
      content-class="scroll-shell px-3 py-5 sm:px-4 sm:py-6"
      class="min-h-0 !bg-white/72 !border-r-gray-200/60 !backdrop-blur-2xl"
    >
      <div class="mb-7 flex items-center gap-3 px-2">
        <div class="h-11 w-11 rounded-3 bg-gradient-to-br from-tide to-teal-800 text-white flex items-center justify-center font-display text-base font-700 shadow-md shadow-tide/25">
          ASR
        </div>
        <div v-if="!appStore.siderCollapsed">
          <div class="font-display text-base font-700 text-ink leading-snug">
            语音转写系统
          </div>
          <div class="text-xs text-slate/70">
            Private LAN Edition
          </div>
        </div>
      </div>

      <NMenu :value="currentPath" :options="menuOptions" @update:value="handleMenuSelect" />
    </NLayoutSider>

    <div class="flex-1 flex flex-col min-h-0 min-w-0 h-full overflow-hidden bg-transparent">
      <header class="shrink-0 px-3 pt-3 pb-3 sm:px-6 sm:pt-5 sm:pb-5 border-b border-transparent">
        <div class="flex flex-col gap-3 rounded-4 border border-white/70 bg-white/60 px-4 py-3 backdrop-blur-xl shadow-sm sm:flex-row sm:items-center sm:justify-between sm:px-5 sm:py-3.5">
          <div class="flex min-w-0 items-center gap-3 sm:gap-4">
            <div class="hidden h-8 w-8 rounded-2 bg-tide/10 text-tide flex items-center justify-center text-sm font-700 lg:flex">
              &gt;_
            </div>
            <div class="min-w-0">
              <div class="truncate font-display text-sm font-700 text-ink leading-snug sm:text-base">
                {{ route.meta.title || '语音转写系统' }}
              </div>
              <div class="hidden text-xs text-slate/70 sm:block">
                {{ route.meta.desc || '实时转写与会议分析控制台' }}
              </div>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2 sm:justify-end sm:gap-2.5">
            <div
              class="hidden items-center gap-2 rounded-full px-3.5 py-1.5 text-xs font-600 lg:flex"
              :class="businessSocketBadgeClass"
              :title="businessSocketTooltip"
            >
              <span class="h-2 w-2 rounded-full" :class="businessSocketDotClass" />
              <span>{{ businessSocketLabel }}</span>
            </div>
            <div class="hidden rounded-full bg-mist/80 px-3.5 py-1.5 text-xs font-500 text-slate lg:block">
              {{ userStore.profile?.displayName || userStore.profile?.username || '未登录用户' }}
            </div>
            <NButton quaternary size="small" @click="appStore.toggleSider()">
              {{ appStore.siderCollapsed ? '展开' : '收起' }}
            </NButton>
            <NButton size="small" type="primary" color="#0f766e" @click="handleLogout">
              退出登录
            </NButton>
          </div>
        </div>
      </header>

      <main class="flex flex-col flex-1 min-h-0 min-w-0 overflow-hidden px-3 pb-4 sm:px-6 sm:pb-6">
        <div class="flex flex-col flex-1 min-h-0 min-w-0" :class="[contentWrapperClass]">
          <RouterView />
        </div>
      </main>
    </div>
  </NLayout>
</template>

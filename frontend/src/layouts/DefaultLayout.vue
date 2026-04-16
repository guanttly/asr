<script setup lang="ts">
import type { MenuOption } from 'naive-ui'

import { NIcon, NTooltip } from 'naive-ui'
import { computed, h, ref, watch } from 'vue'

import { useRoute, useRouter } from 'vue-router'

import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const userStore = useUserStore()
const { status: businessSocketStatus, subscribedTopicCount, lastPongAt } = useBusinessSocket()

interface IconShape {
  tag: 'circle' | 'line' | 'path' | 'polyline' | 'rect'
  attrs: Record<string, number | string>
}

const menuIconShapes: Record<string, IconShape[]> = {
  dashboard: [
    { tag: 'rect', attrs: { x: 3, y: 4, width: 8, height: 7, rx: 2 } },
    { tag: 'rect', attrs: { x: 13, y: 4, width: 8, height: 11, rx: 2 } },
    { tag: 'rect', attrs: { x: 3, y: 13, width: 8, height: 7, rx: 2 } },
    { tag: 'rect', attrs: { x: 13, y: 17, width: 8, height: 3, rx: 1.5 } },
  ],
  realtime: [
    { tag: 'path', attrs: { d: 'M12 4v8' } },
    { tag: 'path', attrs: { d: 'M8 8.5a4 4 0 0 0 8 0' } },
    { tag: 'path', attrs: { d: 'M6 10a6 6 0 0 0 12 0' } },
    { tag: 'path', attrs: { d: 'M9 20h6' } },
    { tag: 'path', attrs: { d: 'M12 16v4' } },
  ],
  transcription: [
    { tag: 'path', attrs: { d: 'M8 3.5h6l4 4V19a1.5 1.5 0 0 1-1.5 1.5h-8A1.5 1.5 0 0 1 7 19V5A1.5 1.5 0 0 1 8.5 3.5Z' } },
    { tag: 'path', attrs: { d: 'M14 3.5V8h4' } },
    { tag: 'path', attrs: { d: 'M10 12h6' } },
    { tag: 'path', attrs: { d: 'M10 15.5h6' } },
  ],
  meetings: [
    { tag: 'path', attrs: { d: 'M7 8a2.5 2.5 0 1 0 0-5 2.5 2.5 0 0 0 0 5Z' } },
    { tag: 'path', attrs: { d: 'M17 10a2 2 0 1 0 0-4 2 2 0 0 0 0 4Z' } },
    { tag: 'path', attrs: { d: 'M3.5 18a3.5 3.5 0 0 1 7 0' } },
    { tag: 'path', attrs: { d: 'M13 18a4 4 0 0 0-2-3.46' } },
    { tag: 'path', attrs: { d: 'M20.5 18a3.5 3.5 0 0 0-5.64-2.8' } },
  ],
  voiceprints: [
    { tag: 'path', attrs: { d: 'M6.5 16.5a5.5 5.5 0 0 1 0-9' } },
    { tag: 'path', attrs: { d: 'M9.5 14a3 3 0 0 1 0-4' } },
    { tag: 'path', attrs: { d: 'M15 10h5' } },
    { tag: 'path', attrs: { d: 'M15 14h3.5' } },
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 1.5 } },
  ],
  workflows: [
    { tag: 'rect', attrs: { x: 3.5, y: 5, width: 6, height: 4.5, rx: 1.5 } },
    { tag: 'rect', attrs: { x: 14.5, y: 4, width: 6, height: 6, rx: 1.5 } },
    { tag: 'rect', attrs: { x: 9, y: 14.5, width: 6, height: 4.5, rx: 1.5 } },
    { tag: 'path', attrs: { d: 'M9.5 7.25h5' } },
    { tag: 'path', attrs: { d: 'M17.5 10v2.5a2 2 0 0 1-2 2H12' } },
    { tag: 'path', attrs: { d: 'M6.5 9.5V12a2.5 2.5 0 0 0 2.5 2.5H9' } },
  ],
  terminology: [
    { tag: 'path', attrs: { d: 'M6.5 4.5h9A2.5 2.5 0 0 1 18 7v12a2.5 2.5 0 0 0-2.5-2.5h-9A2.5 2.5 0 0 0 4 19V7a2.5 2.5 0 0 1 2.5-2.5Z' } },
    { tag: 'path', attrs: { d: 'M8 8.5h6' } },
    { tag: 'path', attrs: { d: 'M8 12h8' } },
  ],
  sensitive: [
    { tag: 'path', attrs: { d: 'M12 3.8c4.3 0 7.8 3.5 7.8 7.8 0 5.7-7.8 8.6-7.8 8.6S4.2 17.3 4.2 11.6c0-4.3 3.5-7.8 7.8-7.8Z' } },
    { tag: 'path', attrs: { d: 'M9.6 11.8 11.2 13.4 14.8 9.8' } },
  ],
  users: [
    { tag: 'circle', attrs: { cx: 12, cy: 8, r: 3 } },
    { tag: 'path', attrs: { d: 'M5 19a7 7 0 0 1 14 0' } },
  ],
  roles: [
    { tag: 'path', attrs: { d: 'M12 3.5 18.5 6v5c0 4.2-2.68 7.27-6.5 9-3.82-1.73-6.5-4.8-6.5-9V6l6.5-2.5Z' } },
    { tag: 'path', attrs: { d: 'm9.5 12 1.7 1.7 3.3-3.7' } },
  ],
  overviewSection: [
    { tag: 'rect', attrs: { x: 3.5, y: 4.5, width: 17, height: 15, rx: 3 } },
    { tag: 'path', attrs: { d: 'M8 9.5h8' } },
    { tag: 'path', attrs: { d: 'M8 14h5' } },
  ],
  applicationsSection: [
    { tag: 'rect', attrs: { x: 4, y: 5, width: 16, height: 14, rx: 3 } },
    { tag: 'path', attrs: { d: 'M9 5v14' } },
    { tag: 'path', attrs: { d: 'M4 10h5' } },
  ],
  appSettings: [
    { tag: 'path', attrs: { d: 'M12 3.8v2.4' } },
    { tag: 'path', attrs: { d: 'M12 17.8v2.4' } },
    { tag: 'path', attrs: { d: 'M4.8 12h2.4' } },
    { tag: 'path', attrs: { d: 'M16.8 12h2.4' } },
    { tag: 'path', attrs: { d: 'm6.9 6.9 1.7 1.7' } },
    { tag: 'path', attrs: { d: 'm15.4 15.4 1.7 1.7' } },
    { tag: 'path', attrs: { d: 'm17.1 6.9-1.7 1.7' } },
    { tag: 'path', attrs: { d: 'm8.6 15.4-1.7 1.7' } },
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 3.2 } },
  ],
  workflowSection: [
    { tag: 'rect', attrs: { x: 4, y: 6, width: 5.5, height: 4.5, rx: 1.4 } },
    { tag: 'rect', attrs: { x: 14.5, y: 6, width: 5.5, height: 4.5, rx: 1.4 } },
    { tag: 'rect', attrs: { x: 9.25, y: 13.5, width: 5.5, height: 4.5, rx: 1.4 } },
    { tag: 'path', attrs: { d: 'M9.5 8.2h5' } },
    { tag: 'path', attrs: { d: 'M12 10.8v2.7' } },
  ],
  terminologySection: [
    { tag: 'path', attrs: { d: 'M6 5.2h10a2 2 0 0 1 2 2V19a2.5 2.5 0 0 0-2.5-2.5H6a2 2 0 0 1-2-2V7.2a2 2 0 0 1 2-2Z' } },
    { tag: 'path', attrs: { d: 'M8 9.5h7' } },
    { tag: 'path', attrs: { d: 'M8 13h5' } },
  ],
  systemSection: [
    { tag: 'circle', attrs: { cx: 12, cy: 12, r: 3 } },
    { tag: 'path', attrs: { d: 'M12 4.5v2.2' } },
    { tag: 'path', attrs: { d: 'M12 17.3v2.2' } },
    { tag: 'path', attrs: { d: 'M19.5 12h-2.2' } },
    { tag: 'path', attrs: { d: 'M6.7 12H4.5' } },
    { tag: 'path', attrs: { d: 'm17.3 6.7-1.5 1.5' } },
    { tag: 'path', attrs: { d: 'm8.2 15.8-1.5 1.5' } },
    { tag: 'path', attrs: { d: 'm17.3 17.3-1.5-1.5' } },
    { tag: 'path', attrs: { d: 'm8.2 8.2-1.5-1.5' } },
  ],
}

function renderMenuIcon(icon: keyof typeof menuIconShapes, tooltipLabel: string) {
  return () => h(
    NTooltip,
    { placement: 'right', disabled: !appStore.siderCollapsed },
    {
      trigger: () => h(
        'span',
        { class: 'nav-menu-icon-trigger' },
        [
          h(
            NIcon,
            { size: 24, class: 'nav-menu-icon' },
            {
              default: () => h(
                'svg',
                {
                  'viewBox': '0 0 24 24',
                  'fill': 'none',
                  'stroke': 'currentColor',
                  'stroke-width': '1.8',
                  'stroke-linecap': 'round',
                  'stroke-linejoin': 'round',
                },
                menuIconShapes[icon].map(({ tag, attrs }) => h(tag, attrs)),
              ),
            },
          ),
        ],
      ),
      default: () => tooltipLabel,
    },
  )
}

const menuOptions: MenuOption[] = [
  {
    label: '数据看板',
    key: '/dashboard',
    icon: renderMenuIcon('dashboard', '数据看板'),
  },
  {
    label: '应用',
    key: 'applications',
    icon: renderMenuIcon('applicationsSection', '应用'),
    children: [
      { label: '实时语音识别', key: '/realtime', icon: renderMenuIcon('realtime', '实时语音识别') },
      { label: '批量转写', key: '/transcription', icon: renderMenuIcon('transcription', '批量转写') },
      { label: '会议纪要', key: '/meetings', icon: renderMenuIcon('meetings', '会议纪要') },
      { label: '声纹库', key: '/meetings/voiceprints', icon: renderMenuIcon('voiceprints', '声纹库') },
    ],
  },
  {
    label: '工作流',
    key: 'workflow-center',
    icon: renderMenuIcon('workflowSection', '工作流'),
    children: [
      { label: '工作流管理', key: '/workflows', icon: renderMenuIcon('workflows', '工作流管理') },
      { label: '节点管理', key: '/workflows/nodes', icon: renderMenuIcon('workflowSection', '节点管理') },
      { label: '应用配置', key: '/workflows/application-settings', icon: renderMenuIcon('appSettings', '应用配置') },
    ],
  },
  {
    label: '术语库',
    key: 'terminology-center',
    icon: renderMenuIcon('terminologySection', '术语库'),
    children: [
      { label: '术语库管理', key: '/terminology', icon: renderMenuIcon('terminology', '术语库管理') },
      { label: '敏感词库', key: '/terminology/sensitive', icon: renderMenuIcon('sensitive', '敏感词库') },
      { label: '纠错规则', key: '/terminology/rules', icon: renderMenuIcon('terminology', '纠错规则') },
    ],
  },
  {
    label: '系统管理',
    key: 'system',
    icon: renderMenuIcon('systemSection', '系统管理'),
    children: [
      { label: '用户管理', key: '/system/users', icon: renderMenuIcon('users', '用户管理') },
      { label: '角色管理', key: '/system/roles', icon: renderMenuIcon('roles', '角色管理') },
    ],
  },
]

function resolveMenuSection(path: string) {
  if (path.startsWith('/system/'))
    return 'system'
  if (path.startsWith('/workflows'))
    return 'workflow-center'
  if (path.startsWith('/terminology'))
    return 'terminology-center'
  if (path.startsWith('/applications/'))
    return 'applications'
  if (path.startsWith('/dashboard'))
    return null
  return 'applications'
}

const currentPath = computed(() => {
  const path = route.path
  if (path.startsWith('/workflows/application-settings'))
    return '/workflows/application-settings'
  if (path.startsWith('/workflows/nodes'))
    return '/workflows/nodes'
  if (path.startsWith('/workflows'))
    return '/workflows'
  if (path.startsWith('/meetings/voiceprints'))
    return '/meetings/voiceprints'
  if (path.startsWith('/meetings'))
    return '/meetings'
  if (path.startsWith('/terminology/sensitive'))
    return '/terminology/sensitive'
  if (path.startsWith('/terminology/rules'))
    return '/terminology/rules'
  if (path.startsWith('/system/users'))
    return '/system/users'
  if (path.startsWith('/system/roles'))
    return '/system/roles'
  if (path.startsWith('/terminology'))
    return '/terminology'
  if (path.startsWith('/transcription'))
    return '/transcription'
  if (path.startsWith('/realtime'))
    return '/realtime'
  return path
})
const initialExpandedSection = resolveMenuSection(route.path)
const expandedMenuKeys = ref<string[]>(initialExpandedSection ? [initialExpandedSection] : [])

watch(
  () => route.path,
  (path) => {
    const sectionKey = resolveMenuSection(path)
    if (!sectionKey) {
      expandedMenuKeys.value = []
      return
    }
    if (!expandedMenuKeys.value.includes(sectionKey))
      expandedMenuKeys.value = [sectionKey]
  },
  { immediate: true },
)

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
const siderToggleTitle = computed(() => appStore.siderCollapsed ? '展开导航' : '收起导航')

function handleMenuSelect(key: string) {
  router.push(key)
}

function handleMenuExpand(keys: string[]) {
  expandedMenuKeys.value = keys.slice(-1)
}

function handleLogout() {
  userStore.logout()
  appStore.resetWorkflowBindings()
  router.push('/login')
}
</script>

<template>
  <NLayout has-sider class="page-shell overflow-hidden" content-style="overflow: hidden; display: flex;">
    <NLayoutSider
      bordered
      collapse-mode="width"
      :collapsed-width="88"
      :width="244"
      :collapsed="appStore.siderCollapsed"
      content-class="scroll-shell px-3 py-5 sm:px-4 sm:py-6"
      class="min-h-0 !bg-white/72 !border-r-gray-200/60 !backdrop-blur-2xl"
    >
      <div class="sidebar-brand-area" :class="{ 'is-collapsed': appStore.siderCollapsed }">
        <div class="sidebar-brand-main">
          <img src="/logo.png" alt="ASR" class="sidebar-logo">
          <div v-if="!appStore.siderCollapsed" class="sidebar-brand-text">
            <div class="sidebar-brand-title">
              语音转写系统
            </div>
            <div class="sidebar-brand-subtitle">
              Private LAN Edition
            </div>
          </div>
        </div>
        <NButton
          quaternary
          circle
          class="sidebar-toggle-button"
          :title="siderToggleTitle"
          :aria-label="siderToggleTitle"
          @click="appStore.toggleSider()"
        >
          <svg class="sidebar-toggle-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
            <rect x="3.5" y="4" width="17" height="16" rx="3" />
            <path d="M9 4v16" />
            <path :d="appStore.siderCollapsed ? 'm14.5 12 2.5 2.5V9.5Z' : 'm12.5 12 2.5-2.5v5Z'" />
          </svg>
        </NButton>
      </div>

      <NMenu
        class="sidebar-menu"
        :value="currentPath"
        :options="menuOptions"
        :expanded-keys="expandedMenuKeys"
        :collapsed="appStore.siderCollapsed"
        :collapsed-width="88"
        :collapsed-icon-size="24"
        accordion
        @update:value="handleMenuSelect"
        @update:expanded-keys="handleMenuExpand"
      />
    </NLayoutSider>

    <div class="flex-1 flex flex-col min-h-0 min-w-0 h-full overflow-hidden bg-transparent">
      <header class="shrink-0 px-3 pt-3 pb-3 sm:px-6 sm:pt-5 sm:pb-5 border-b border-transparent">
        <div class="flex flex-col gap-3 rounded-4 border border-white/70 bg-white/60 px-4 py-3 backdrop-blur-xl shadow-sm sm:flex-row sm:items-center sm:justify-between sm:px-5 sm:py-3.5">
          <div class="flex min-w-0 items-center gap-3 sm:gap-4">
            <div class="topbar-route-icon hidden lg:flex" aria-hidden="true">
              <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
                <rect x="4" y="5" width="16" height="14" rx="3" />
                <path d="M8 10h8" />
                <path d="M8 14h5" />
              </svg>
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

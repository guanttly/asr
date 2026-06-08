<script setup lang="ts">
import type { MenuOption } from 'naive-ui'
import type { RulesTreeNode } from '@/api/rulesCatalog'

import type { CatalogTreeNode } from '@/api/termCatalog'
import { NIcon, NTooltip } from 'naive-ui'

import { computed, h, onMounted, ref, watch } from 'vue'

import { useRoute, useRouter } from 'vue-router'
import { getRulesCatalogTree } from '@/api/rulesCatalog'
import { getTermCatalogTree } from '@/api/termCatalog'
import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { PRODUCT_FEATURE_KEYS } from '@/constants/product'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'
import {
  findFirstRulesFile,
  findRulesNodeByPath,
  findRulesParentPaths,
  RULES_CATALOG_ROUTE,
  rulesCatalogDirMenuKey,
  rulesCatalogMenuLabel,
  rulesCatalogRouteKey,
} from '@/utils/rulesCatalogMenu'
import {
  catalogMenuLabel,
  findCatalogNodeByPath,
  findCatalogParentPaths,
  findFirstCatalogFile,
  TERM_CATALOG_ROUTE,
  termCatalogDirMenuKey,
  termCatalogRouteKey,
} from '@/utils/termCatalogMenu'

const router = useRouter()
const route = useRoute()
const appStore = useAppStore()
const userStore = useUserStore()
const isAdmin = computed(() => userStore.profile?.role === 'admin')
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
  openapi: [
    { tag: 'rect', attrs: { x: 4, y: 5.5, width: 7, height: 13, rx: 2 } },
    { tag: 'path', attrs: { d: 'M7 9h2' } },
    { tag: 'path', attrs: { d: 'M7 12h2' } },
    { tag: 'path', attrs: { d: 'M7 15h2' } },
    { tag: 'circle', attrs: { cx: 17.5, cy: 9, r: 2.2 } },
    { tag: 'circle', attrs: { cx: 17.5, cy: 15, r: 2.2 } },
    { tag: 'path', attrs: { d: 'M11 12h4.3' } },
  ],
  downloads: [
    { tag: 'path', attrs: { d: 'M12 4.5v9.5' } },
    { tag: 'path', attrs: { d: 'm8.5 10.5 3.5 3.5 3.5-3.5' } },
    { tag: 'path', attrs: { d: 'M5 18.5h14' } },
    { tag: 'path', attrs: { d: 'M7.5 15.5v3' } },
    { tag: 'path', attrs: { d: 'M16.5 15.5v3' } },
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

const SYSTEM_TERM_COLLECTION_KEY = 'system-term-collection'
const SYSTEM_RULES_COLLECTION_KEY = 'system-rules-collection'

const termCatalogTree = ref<CatalogTreeNode[]>([])
const rulesCatalogTree = ref<RulesTreeNode[]>([])
const expandedMenuKeys = ref<string[]>([])

function buildTermCatalogMenuOptions(nodes: CatalogTreeNode[]): MenuOption[] {
  const directories = nodes.filter(node => node.is_dir)
  if (directories.length) {
    return directories
      .map((node): MenuOption | null => {
        const target = findFirstCatalogFile(node.children || [])
        if (!target)
          return null
        return {
          label: catalogMenuLabel(node),
          key: termCatalogDirMenuKey(node.path),
        }
      })
      .filter((item): item is MenuOption => Boolean(item))
  }

  const target = findFirstCatalogFile(nodes)
  return target
    ? [{ label: '影像科', key: termCatalogDirMenuKey('') }]
    : []
}

function resolveTermCatalogScopeTarget(scopePath: string) {
  if (!scopePath)
    return findFirstCatalogFile(termCatalogTree.value)
  const node = findCatalogNodeByPath(termCatalogTree.value, scopePath)
  if (!node)
    return null
  if (!node.is_dir)
    return node
  return findFirstCatalogFile(node.children || [])
}

async function loadTermCatalogMenu() {
  try {
    const result = await getTermCatalogTree()
    if (result.data.items?.length)
      termCatalogTree.value = result.data.items
  }
  catch {
    termCatalogTree.value = []
  }
  try {
    const result = await getRulesCatalogTree()
    if (result.data.items?.length)
      rulesCatalogTree.value = result.data.items
  }
  catch {
    rulesCatalogTree.value = []
  }
  expandedMenuKeys.value = resolveExpandedMenuKeys()
}

function buildRulesCatalogMenuOptions(nodes: RulesTreeNode[]): MenuOption[] {
  const directories = nodes.filter(node => node.is_dir)
  if (directories.length) {
    return directories
      .map((node): MenuOption | null => {
        const target = findFirstRulesFile(node.children || [])
        if (!target)
          return null
        return { label: rulesCatalogMenuLabel(node), key: rulesCatalogDirMenuKey(node.path) }
      })
      .filter((item): item is MenuOption => Boolean(item))
  }

  const target = findFirstRulesFile(nodes)
  return target
    ? [{ label: '影像科', key: rulesCatalogDirMenuKey('') }]
    : []
}

function resolveRulesCatalogScopeTarget(scopePath: string) {
  if (!scopePath)
    return findFirstRulesFile(rulesCatalogTree.value)
  const node = findRulesNodeByPath(rulesCatalogTree.value, scopePath)
  if (!node)
    return null
  if (!node.is_dir)
    return node
  return findFirstRulesFile(node.children || [])
}

const menuOptions = computed<MenuOption[]>(() => {
  const applicationChildren: MenuOption[] = [
    { label: '实时语音识别', key: '/realtime', icon: renderMenuIcon('realtime', '实时语音识别') },
    { label: '批量转写', key: '/transcription', icon: renderMenuIcon('transcription', '批量转写') },
  ]
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.MEETING))
    applicationChildren.push({ label: '会议纪要', key: '/meetings', icon: renderMenuIcon('meetings', '会议纪要') })
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICEPRINT))
    applicationChildren.push({ label: '声纹库', key: '/meetings/voiceprints', icon: renderMenuIcon('voiceprints', '声纹库') })

  const terminologyChildren: MenuOption[] = [
    { label: '术语库管理', key: '/terminology', icon: renderMenuIcon('terminology', '术语库管理') },
    { label: '语气词库', key: '/terminology/fillers', icon: renderMenuIcon('terminology', '语气词库') },
    { label: '敏感词库', key: '/terminology/sensitive', icon: renderMenuIcon('sensitive', '敏感词库') },
  ]
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICE_CONTROL))
    terminologyChildren.splice(3, 0, { label: '控制指令库', key: '/terminology/voice-commands', icon: renderMenuIcon('terminology', '控制指令库') })

  const termCollectionChildren = buildTermCatalogMenuOptions(termCatalogTree.value)
  const rulesCollectionChildren = buildRulesCatalogMenuOptions(rulesCatalogTree.value)

  const applicationSection: MenuOption = {
    label: '应用',
    key: 'applications',
    icon: renderMenuIcon('applicationsSection', '应用'),
    children: applicationChildren,
  }

  // 普通用户仅可访问“应用”相关功能，看板 / 工作流 / 术语库 / 系统管理仅管理员可见。
  if (!isAdmin.value)
    return [applicationSection]

  return [
    {
      label: '数据看板',
      key: '/dashboard',
      icon: renderMenuIcon('dashboard', '数据看板'),
    },
    applicationSection,
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
      children: terminologyChildren,
    },
    {
      label: '系统管理',
      key: 'system',
      icon: renderMenuIcon('systemSection', '系统管理'),
      children: [
        { label: '用户管理', key: '/system/users', icon: renderMenuIcon('users', '用户管理') },
        {
          label: '术语收集',
          key: SYSTEM_TERM_COLLECTION_KEY,
          icon: renderMenuIcon('terminologySection', '术语收集'),
          children: termCollectionChildren.length
            ? termCollectionChildren
            : [{ label: '暂无术语目录', key: 'system-term-empty', disabled: true }],
        },
        {
          label: '规则收集',
          key: SYSTEM_RULES_COLLECTION_KEY,
          icon: renderMenuIcon('terminologySection', '规则收集'),
          children: rulesCollectionChildren.length
            ? rulesCollectionChildren
            : [{ label: '暂无规则目录', key: 'system-rules-empty', disabled: true }],
        },
        { label: '对接管理', key: '/system/openapi', icon: renderMenuIcon('openapi', '对接管理') },
        { label: '终端下载', key: '/downloads', icon: renderMenuIcon('downloads', '终端下载') },
      ],
    },
  ]
})

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

function currentTermCatalogPath() {
  const value = route.query.path
  if (typeof value === 'string')
    return value
  if (Array.isArray(value) && typeof value[0] === 'string')
    return value[0]
  return null
}

function resolveExpandedMenuKeys() {
  if (route.path.startsWith(TERM_CATALOG_ROUTE)) {
    return ['system', SYSTEM_TERM_COLLECTION_KEY]
  }
  if (route.path.startsWith(RULES_CATALOG_ROUTE)) {
    return ['system', SYSTEM_RULES_COLLECTION_KEY]
  }
  const sectionKey = resolveMenuSection(route.path)
  return sectionKey ? [sectionKey] : []
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
  if (path.startsWith('/terminology/fillers'))
    return '/terminology/fillers'
  if (path.startsWith('/terminology/voice-commands'))
    return '/terminology/voice-commands'
  if (path.startsWith('/terminology/sensitive'))
    return '/terminology/sensitive'
  if (path.startsWith('/system/openapi'))
    return '/system/openapi'
  if (path.startsWith('/system/users'))
    return '/system/users'
  if (path.startsWith('/system/terms-catalog')) {
    const catalogPath = currentTermCatalogPath()
    if (!catalogPath)
      return '/system/terms-catalog'
    const parentPath = findCatalogParentPaths(termCatalogTree.value, catalogPath)[0]
    if (parentPath)
      return termCatalogDirMenuKey(parentPath)
    if (findCatalogNodeByPath(termCatalogTree.value, catalogPath))
      return termCatalogDirMenuKey('')
    return termCatalogRouteKey(catalogPath)
  }
  if (path.startsWith('/system/rules-catalog')) {
    const catalogPath = currentTermCatalogPath()
    if (!catalogPath)
      return '/system/rules-catalog'
    const parentPath = findRulesParentPaths(rulesCatalogTree.value, catalogPath)[0]
    if (parentPath)
      return rulesCatalogDirMenuKey(parentPath)
    if (findRulesNodeByPath(rulesCatalogTree.value, catalogPath))
      return rulesCatalogDirMenuKey('')
    return rulesCatalogRouteKey(catalogPath)
  }
  if (path.startsWith('/terminology'))
    return '/terminology'
  if (path.startsWith('/transcription'))
    return '/transcription'
  if (path.startsWith('/realtime'))
    return '/realtime'
  return path
})
watch(
  () => route.fullPath,
  () => {
    expandedMenuKeys.value = resolveExpandedMenuKeys()
  },
  { immediate: true },
)

watch(termCatalogTree, () => {
  expandedMenuKeys.value = resolveExpandedMenuKeys()
})

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
  if (key === 'system-term-empty' || key === 'system-rules-empty')
    return
  if (key.startsWith('system-term-dir:')) {
    const scopePath = key.replace('system-term-dir:', '')
    const target = resolveTermCatalogScopeTarget(scopePath)
    if (target)
      router.push(termCatalogRouteKey(target.path))
    return
  }
  if (key.startsWith('system-rules-dir:')) {
    const scopePath = key.replace('system-rules-dir:', '')
    const target = resolveRulesCatalogScopeTarget(scopePath)
    if (target)
      router.push(rulesCatalogRouteKey(target.path))
    return
  }
  router.push(key)
}

function handleMenuExpand(keys: string[]) {
  expandedMenuKeys.value = keys
}

function handleLogout() {
  userStore.logout()
  appStore.resetWorkflowBindings()
  router.push('/login')
}

const buildVersion = __APP_VERSION__
const buildCode = __APP_BUILD_CODE__
const buildDate = __APP_BUILD_DATE__
const buildTitle = `构建日期 ${buildDate}`

onMounted(loadTermCatalogMenu)
</script>

<template>
  <NLayout has-sider class="page-shell overflow-hidden" content-style="overflow: hidden; display: flex;">
    <NLayoutSider
      bordered
      collapse-mode="width"
      :collapsed-width="88"
      :width="244"
      :collapsed="appStore.siderCollapsed"
      content-class="sider-shell px-3 py-5 sm:px-4 sm:py-6"
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
        :root-indent="18"
        :indent="18"
        accordion
        @update:value="handleMenuSelect"
        @update:expanded-keys="handleMenuExpand"
      />
      <div
        class="sidebar-build"
        :class="{ 'is-collapsed': appStore.siderCollapsed }"
        :title="buildTitle"
      >
        <span v-if="appStore.siderCollapsed">v{{ buildVersion }}</span>
        <template v-else>
          <span class="sidebar-build-version">版本 {{ buildVersion }}</span>
          <span class="sidebar-build-meta">构建 {{ buildCode }} · {{ buildDate }}</span>
        </template>
      </div>
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
                {{ route.meta.desc || '语音转写与流程配置控制台' }}
              </div>
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2 sm:justify-end sm:gap-2.5">
            <div
              class="hidden items-center gap-2 rounded-full px-3 py-1.5 text-xs font-600 lg:flex"
              :class="businessSocketBadgeClass"
              :title="businessSocketTooltip"
            >
              <span class="h-1.75 w-1.75 rounded-full" :class="businessSocketDotClass" />
              <span>{{ businessSocketLabel }}</span>
            </div>
            <div class="hidden items-center gap-2 rounded-full bg-mist/70 px-3 py-1.5 text-xs font-600 text-slate lg:flex">
              <span class="inline-flex h-5 w-5 items-center justify-center rounded-full bg-teal/14 text-[10px] font-700 uppercase text-teal">
                {{ (userStore.profile?.displayName || userStore.profile?.username || 'U').slice(0, 1) }}
              </span>
              <span class="max-w-32 truncate">{{ userStore.profile?.displayName || userStore.profile?.username || '未登录用户' }}</span>
            </div>
            <NButton size="small" quaternary class="logout-button" @click="handleLogout">
              <template #icon>
                <NIcon :size="14">
                  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">
                    <path d="M9 21H5a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h4" />
                    <path d="m16 17 5-5-5-5" />
                    <path d="M21 12H9" />
                  </svg>
                </NIcon>
              </template>
              退出
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

import type { RouteRecordRaw } from 'vue-router'

import { createRouter, createWebHistory } from 'vue-router'

import { isProductFeatureKey, PRODUCT_FEATURE_KEYS } from '@/constants/product'
import BlankLayout from '@/layouts/BlankLayout.vue'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'

const routes: RouteRecordRaw[] = [
  {
    path: '/login',
    component: BlankLayout,
    meta: { public: true },
    children: [
      {
        path: '',
        name: 'login',
        component: () => import('@/pages/login.vue'),
      },
    ],
  },
  {
    path: '/downloads',
    component: BlankLayout,
    meta: { public: true, allowAuthenticated: true },
    children: [
      {
        path: '',
        name: 'public-downloads',
        meta: { title: '终端下载', desc: '公开分发桌面端安装包，下载目录来自容器外挂载路径。' },
        component: () => import('@/pages/system/downloads.vue'),
      },
    ],
  },
  {
    path: '/',
    component: DefaultLayout,
    redirect: '/dashboard',
    children: [
      {
        path: 'realtime',
        name: 'realtime',
        meta: { title: '实时语音识别', desc: '采集麦克风音频，并按应用配置中的默认工作流完成保存后的后处理。' },
        component: () => import('@/pages/realtime/index.vue'),
      },
      {
        path: 'dashboard',
        name: 'dashboard',
        meta: { title: '数据看板', desc: '统一查看批量转写、回流和后处理链路的整体健康度。' },
        component: () => import('@/pages/dashboard/index.vue'),
      },
      {
        path: 'transcription',
        name: 'transcription',
        meta: { pageManagedScroll: true, title: '批量转写', desc: '上传本地音频或提交 URL，并按应用配置中的默认工作流执行后处理。' },
        component: () => import('@/pages/transcription/history.vue'),
      },
      {
        path: 'transcription/history',
        redirect: '/transcription',
      },
      {
        path: 'applications/settings',
        redirect: '/workflows/application-settings',
      },
      {
        path: 'workflows/application-settings',
        name: 'application-settings',
        meta: { pageManagedScroll: true, title: '应用配置', desc: '在工作流目录下统一配置各应用默认绑定的工作流。' },
        component: () => import('@/pages/application/settings.vue'),
      },
      {
        path: 'workflows',
        name: 'workflows',
        meta: { pageManagedScroll: true, title: '工作流管理', desc: '管理系统模板与个人工作流，编排纠错、过滤和转写后处理节点。' },
        component: () => import('@/pages/workflow/index.vue'),
      },
      {
        path: 'workflows/nodes',
        name: 'workflow-nodes',
        meta: { pageManagedScroll: true, title: '节点管理', desc: '统一维护节点默认配置，并对单个节点直接做调试验证。' },
        component: () => import('@/pages/workflow/nodes.vue'),
      },
      {
        path: 'workflows/:id',
        name: 'workflow-editor',
        meta: { pageManagedScroll: true, title: '工作流编辑器', desc: '调整节点顺序、配置参数，并对单节点或整条工作流做验证。' },
        component: () => import('@/pages/workflow/editor.vue'),
      },
      {
        path: 'meetings',
        name: 'meetings',
        meta: { requiredFeature: PRODUCT_FEATURE_KEYS.MEETING, title: '会议纪要', desc: '上传会议录音、查看说话人标注，并按应用配置生成结构化纪要。' },
        component: () => import('@/pages/meeting/index.vue'),
      },
      {
        path: 'meetings/upload',
        name: 'meeting-upload',
        meta: { requiredFeature: PRODUCT_FEATURE_KEYS.MEETING, title: '新建会议', desc: '创建会议任务，摘要生成工作流由应用配置页统一维护。' },
        component: () => import('@/pages/meeting/upload.vue'),
      },
      {
        path: 'meetings/voiceprints',
        name: 'meeting-voiceprints',
        meta: { requiredFeature: PRODUCT_FEATURE_KEYS.VOICEPRINT, pageManagedScroll: true, title: '声纹库', desc: '管理会议纪要中 speaker_diarize 节点使用的已注册说话人声纹样本。' },
        component: () => import('@/pages/meeting/voiceprints.vue'),
      },
      {
        path: 'meetings/:id',
        name: 'meeting-detail',
        meta: { requiredFeature: PRODUCT_FEATURE_KEYS.MEETING, title: '会议详情', desc: '查看逐字稿、说话人片段与会议摘要，并按应用配置重新生成。' },
        component: () => import('@/pages/meeting/detail.vue'),
      },
      {
        path: 'terminology',
        name: 'terminology',
        meta: { title: '术语库管理', desc: '查看词库、维护词条和词库附属纠错规则，并为 ASR 热词与后处理提供统一数据源。' },
        component: () => import('@/pages/terminology/index.vue'),
      },
      {
        path: 'terminology/rules',
        redirect: '/terminology',
      },
      {
        path: 'terminology/sensitive',
        name: 'terminology-sensitive',
        meta: { pageManagedScroll: true, title: '敏感词库', desc: '维护基础敏感词库和各业务场景词库，供敏感词过滤节点直接选择。' },
        component: () => import('@/pages/terminology/sensitive.vue'),
      },
      {
        path: 'terminology/fillers',
        name: 'terminology-fillers',
        meta: { pageManagedScroll: true, title: '语气词库', desc: '维护基础语气词库和各业务场景词库，供语气词过滤节点直接选择。' },
        component: () => import('@/pages/terminology/fillers.vue'),
      },
      {
        path: 'terminology/voice-commands',
        name: 'terminology-voice-commands',
        meta: { requiredFeature: PRODUCT_FEATURE_KEYS.VOICE_CONTROL, pageManagedScroll: true, title: '控制指令库', desc: '维护语音控制工作流里可识别的控制指令组、候选话术与有效分组。' },
        component: () => import('@/pages/terminology/voice-commands.vue'),
      },
      {
        path: 'system/users',
        name: 'system-users',
        meta: { title: '用户管理', desc: '维护系统账号、显示名称与角色权限。' },
        component: () => import('@/pages/system/users.vue'),
      },
      {
        path: 'system/terms-catalog',
        name: 'system-terms-catalog',
        meta: { pageManagedScroll: true, title: '影像术语库浏览', desc: '按科室浏览影像报告易错术语，一键创建/移除「影像术语·科室」专属术语库，或对单条术语进行加入与移除。' },
        component: () => import('@/pages/system/terms-catalog.vue'),
      },
      {
        path: 'system/rules-catalog',
        name: 'system-rules-catalog',
        meta: { pageManagedScroll: true, title: '影像规则库浏览', desc: '按科室浏览影像报告书写规则与纠错映射，下载内置 Excel 后可到术语库管理批量导入。' },
        component: () => import('@/pages/system/rules-catalog.vue'),
      },
      {
        path: 'system/openapi',
        name: 'system-openapi',
        meta: { pageManagedScroll: true, title: 'OpenAPI 管理', desc: '管理开放平台应用凭证、能力授权、默认工作流与调用日志。' },
        component: () => import('@/pages/system/openapi.vue'),
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

// 仅管理员可访问的路由前缀；普通用户只能使用“应用”相关页面。
const ADMIN_PATH_PREFIXES = ['/dashboard', '/workflows', '/terminology', '/system']

function isAdminOnlyPath(path: string) {
  return ADMIN_PATH_PREFIXES.some(prefix => path === prefix || path.startsWith(`${prefix}/`))
}

router.beforeEach((to) => {
  const userStore = useUserStore()
  const appStore = useAppStore()
  const isAdmin = userStore.profile?.role === 'admin'
  const homePath = isAdmin ? '/dashboard' : '/realtime'
  if (to.meta.public && userStore.token && !to.meta.allowAuthenticated)
    return homePath
  if (to.meta.public)
    return true
  if (!userStore.token)
    return '/login'
  // 已加载用户资料且非管理员时，拦截管理员专属页面。
  if (userStore.profile && !isAdmin && isAdminOnlyPath(to.path))
    return '/realtime'
  const requiredFeature = to.meta.requiredFeature
  if (isProductFeatureKey(requiredFeature) && !appStore.hasCapability(requiredFeature))
    return homePath
  return true
})

export default router

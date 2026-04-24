import type { RouteRecordRaw } from 'vue-router'

import { createRouter, createWebHistory } from 'vue-router'

import BlankLayout from '@/layouts/BlankLayout.vue'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
import { PRODUCT_FEATURE_KEYS, isProductFeatureKey } from '@/constants/product'
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
        meta: { title: '术语库管理', desc: '查看词库、浏览词条，并直接维护术语数据，为三层纠错管道提供可运营底座。' },
        component: () => import('@/pages/terminology/index.vue'),
      },
      {
        path: 'terminology/rules',
        name: 'terminology-rules',
        meta: { title: '纠错规则', desc: '管理词库对应的三层纠错规则，统一维护替换链路。' },
        component: () => import('@/pages/terminology/rules.vue'),
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
        meta: { title: '用户管理', desc: '当前已接入 admin-api 用户查询接口，可用于联调登录、权限和系统管理。' },
        component: () => import('@/pages/system/users.vue'),
      },
      {
        path: 'system/roles',
        name: 'system-roles',
        meta: { title: '角色管理', desc: '管理员、转写员、审校员等角色矩阵将在这里配置。' },
        component: () => import('@/pages/system/roles.vue'),
      },
    ],
  },
]

const router = createRouter({
  history: createWebHistory(),
  routes,
})

router.beforeEach((to) => {
  const userStore = useUserStore()
  const appStore = useAppStore()
  if (to.meta.public && userStore.token && !to.meta.allowAuthenticated)
    return '/dashboard'
  if (to.meta.public)
    return true
  if (!userStore.token)
    return '/login'
  const requiredFeature = to.meta.requiredFeature
  if (isProductFeatureKey(requiredFeature) && !appStore.hasCapability(requiredFeature))
    return '/dashboard'
  return true
})

export default router

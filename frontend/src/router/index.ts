import type { RouteRecordRaw } from 'vue-router'

import { createRouter, createWebHistory } from 'vue-router'

import BlankLayout from '@/layouts/BlankLayout.vue'
import DefaultLayout from '@/layouts/DefaultLayout.vue'
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
    path: '/',
    component: DefaultLayout,
    redirect: '/dashboard',
    children: [
      {
        path: 'dashboard',
        name: 'dashboard',
        meta: { title: '数据看板', desc: '统一查看批量转写、回流和后处理链路的整体健康度。' },
        component: () => import('@/pages/dashboard/index.vue'),
      },
      {
        path: 'transcription',
        name: 'transcription',
        meta: { title: '实时转写工作台', desc: '使用 WebSocket 推送 300ms chunk，并在前端按句子聚合展示。' },
        component: () => import('@/pages/transcription/index.vue'),
      },
      {
        path: 'transcription/history',
        name: 'transcription-history',
        meta: { pageManagedScroll: true, title: '转写历史', desc: '查看实时转写与批量转写任务状态，便于联调 ASR 流程。' },
        component: () => import('@/pages/transcription/history.vue'),
      },
      {
        path: 'meetings',
        name: 'meetings',
        meta: { title: '会议管理', desc: '上传会议录音、查看说话人标注与结构化纪要。' },
        component: () => import('@/pages/meeting/index.vue'),
      },
      {
        path: 'meetings/upload',
        name: 'meeting-upload',
        meta: { title: '新建会议', desc: '当前阶段先使用音频 URL 创建会议任务，后续再接入真实文件上传与对象存储。' },
        component: () => import('@/pages/meeting/upload.vue'),
      },
      {
        path: 'meetings/:id',
        name: 'meeting-detail',
        meta: { title: '会议详情', desc: '查看逐字稿、说话人片段与会议摘要。' },
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
  if (to.meta.public && userStore.token)
    return '/dashboard'
  if (to.meta.public)
    return true
  if (!userStore.token)
    return '/login'
  return true
})

export default router

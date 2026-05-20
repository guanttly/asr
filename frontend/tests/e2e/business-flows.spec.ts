import type { Page } from '@playwright/test'

import { expect, test } from '@playwright/test'

import { loginByStorage, mockFrontendAPI } from './support/apiMock'

const protectedFlows = [
  { path: '/dashboard', title: '数据看板', marker: '批量待处理' },
  { path: '/realtime', title: '实时语音识别', marker: '识别通道' },
  { path: '/transcription', title: '批量转写', marker: '发起批量转写' },
  { path: '/workflows', title: '工作流管理', marker: '工作流列表' },
  { path: '/workflows/nodes', title: '节点管理', marker: '节点默认配置' },
  { path: '/workflows/application-settings', title: '应用配置', marker: '已配置应用' },
  { path: '/workflows/2', title: '工作流编辑器', marker: '工作流基础信息' },
  { path: '/meetings', title: '会议纪要', marker: '科室晨会' },
  { path: '/meetings/upload', title: '新建会议', marker: '会议录音导入' },
  { path: '/meetings/voiceprints', title: '声纹库', marker: '张三' },
  { path: '/meetings/1', title: '会议详情', marker: '会议摘要工作流' },
  { path: '/terminology', title: '术语库管理', marker: '影像术语' },
  { path: '/terminology/fillers', title: '语气词库', marker: '基础语气词' },
  { path: '/terminology/sensitive', title: '敏感词库', marker: '基础敏感词' },
  { path: '/terminology/voice-commands', title: '控制指令库', marker: '基础控制指令' },
  { path: '/system/users', title: '用户管理', marker: '系统管理员' },
  { path: '/system/terms-catalog', title: '影像术语库浏览', marker: '影像术语总览' },
  { path: '/system/rules-catalog', title: '影像规则库浏览', marker: '影像规则总览' },
  { path: '/system/openapi', title: 'OpenAPI 管理', marker: 'HIS 对接' },
  { path: '/downloads', title: '终端安装包下载', marker: 'asr-desktop-win10.exe' },
]

async function expectBusinessFlow(page: Page, flow: typeof protectedFlows[number]) {
  const titleScope = flow.path === '/downloads' ? page.locator('body') : page.locator('header')
  const markerScope = flow.path === '/downloads' ? page.locator('body') : page.locator('main')
  await expect(titleScope.getByText(flow.title, { exact: true }).first()).toBeVisible()
  await expect(markerScope.getByText(flow.marker).first()).toBeVisible()
}

test.beforeEach(async ({ page }) => {
  await mockFrontendAPI(page)
})

test('登录流程会完成鉴权并进入数据看板', async ({ page }) => {
  await page.goto('/login')

  await expect(page.getByText('登录').first()).toBeVisible()
  await page.getByRole('button', { name: '进入系统' }).click()

  await expect(page.locator('header').getByText('数据看板', { exact: true })).toBeVisible()
  await expect(page.getByText('批量待处理')).toBeVisible()
})

for (const flow of protectedFlows) {
  test(`业务页面可达：${flow.title}`, async ({ page }) => {
    await loginByStorage(page)
    await page.goto(flow.path)

    await expectBusinessFlow(page, flow)
  })
}

test('批量转写可提交 URL 任务并打开任务详情', async ({ page }) => {
  await loginByStorage(page)
  await page.goto('/transcription')

  await page.getByText('提交音频 URL', { exact: true }).click()
  await page.getByPlaceholder('https://example.com/audio/demo.wav').fill('https://example.com/audio/new.wav')
  await page.getByRole('button', { name: '提交 URL 任务' }).click()

  await expect(page.getByText('患者肺部小结节，建议随访。').first()).toBeVisible()
  await page.getByRole('button', { name: '详情' }).first().click()
  await expect(page.getByText('转写任务详情')).toBeVisible()
  await expect(page.getByText('工作流执行记录')).toBeVisible()
})

test('工作流创建会进入编辑器', async ({ page }) => {
  await loginByStorage(page)
  await page.goto('/workflows')

  await page.getByRole('button', { name: '新建工作流' }).click()
  await page.getByPlaceholder('例如：医疗转写纠错模板').fill('E2E 新建工作流')
  await page.getByRole('button', { name: '创建并编辑' }).click()

  await expect(page).toHaveURL(/\/workflows\/99$/)
  await expect(page.locator('header').getByText('工作流编辑器', { exact: true })).toBeVisible()
})

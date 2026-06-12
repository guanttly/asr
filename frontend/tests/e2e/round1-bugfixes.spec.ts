import { expect, test } from '@playwright/test'

import { loginByStorage, mockFrontendAPI } from './support/apiMock'

// 第一轮测试遗留的 3 条 BUG 的复现 / 回归脚本。
// 14852：普通版进入节点管理不应再请求控制指令库（避免 403 触发的报错提示）。
// 14854：语气词库列表里“默认叠加”与“当前词库”标识不应叠加在一起。
// 14853：LLM 纠错节点的 <think> 推理泄漏属于后端逻辑，已由 Go 集成测试覆盖；
//        这里补一个前端节点测试面板的 UI 冒烟，确认流式结果能正常展示。

test('14852｜普通版节点管理不请求控制指令库且无加载失败提示', async ({ page }) => {
  await loginByStorage(page)
  await mockFrontendAPI(page, { edition: 'standard' })

  const voiceCommandRequests: string[] = []
  page.on('request', (req) => {
    if (req.url().includes('/api/admin/voice-command-dicts'))
      voiceCommandRequests.push(req.url())
  })

  await page.goto('/workflows/nodes')

  // 页面正常渲染（节点默认配置可见）。
  await expect(page.locator('header').getByText('节点管理', { exact: true }).first()).toBeVisible()
  await expect(page.locator('main').getByText('节点默认配置').first()).toBeVisible()

  // 关键断言：普通版不应再调用控制指令库接口，因此不会触发 403 -> 报错提示。
  expect(voiceCommandRequests, '普通版不应请求 voice-command-dicts').toHaveLength(0)
  await expect(page.getByText('控制指令组加载失败')).toHaveCount(0)

  // 语音控制节点本身也不应出现在普通版节点列表里。
  await expect(page.getByText('控制指令识别')).toHaveCount(0)
})

test('14854｜语气词库“默认叠加”与“当前词库”标识不重叠', async ({ page }) => {
  await loginByStorage(page)
  await mockFrontendAPI(page)

  await page.goto('/terminology/fillers')
  await expect(page.locator('main').getByText('基础语气词').first()).toBeVisible()

  const overlayTag = page.getByText('默认叠加', { exact: true }).first()
  const currentTag = page.getByText('当前词库', { exact: true }).first()
  await expect(overlayTag).toBeVisible()
  await expect(currentTag).toBeVisible()

  await page.screenshot({ path: 'test-results/fillers-14854.png', fullPage: true })

  const overlayBox = await overlayTag.boundingBox()
  const currentBox = await currentTag.boundingBox()
  expect(overlayBox).not.toBeNull()
  expect(currentBox).not.toBeNull()
  if (overlayBox && currentBox) {
    const horizontalOverlap = overlayBox.x + overlayBox.width > currentBox.x
      && currentBox.x + currentBox.width > overlayBox.x
    const verticalOverlap = overlayBox.y + overlayBox.height > currentBox.y
      && currentBox.y + currentBox.height > overlayBox.y
    expect(horizontalOverlap && verticalOverlap, '两个标识不应在同一区域重叠').toBe(false)
  }
})

test('14853｜LLM 纠错节点测试面板能展示流式纠错结果', async ({ page }) => {
  await loginByStorage(page)
  await mockFrontendAPI(page)

  // 用 ndjson 模拟后端已剥离 <think> 之后的流式输出（后端剥离逻辑由 Go 测试覆盖）。
  await page.route('**/api/admin/workflows/test-node?stream=1', async (route) => {
    const lines = [
      JSON.stringify({ type: 'status', message: '节点执行中' }),
      JSON.stringify({ type: 'delta', delta: '患者主诉', output_text: '患者主诉' }),
      JSON.stringify({ type: 'delta', delta: '头痛三天。', output_text: '患者主诉头痛三天。' }),
      JSON.stringify({ type: 'done', output_text: '患者主诉头痛三天。', detail: { model: 'qwen3-4b', streamed: true } }),
    ]
    await route.fulfill({
      status: 200,
      contentType: 'application/x-ndjson',
      body: `${lines.join('\n')}\n`,
    })
  })

  await page.goto('/workflows/nodes')
  await expect(page.locator('main').getByText('节点默认配置').first()).toBeVisible()
  // 选中 LLM 校对节点。
  await page.getByText('LLM 校对', { exact: true }).first().click()
  await page.getByPlaceholder('输入测试文本，验证当前节点默认配置下的输出。').fill('患者主诉头痛三天')
  await page.getByRole('button', { name: '测试当前节点' }).click()

  // 流式结果展示为干净的纠错文本，且不含 <think> 推理内容。
  await expect(page.locator('main').getByText('患者主诉头痛三天。').first()).toBeVisible()
  await expect(page.getByText('<think>')).toHaveCount(0)
})

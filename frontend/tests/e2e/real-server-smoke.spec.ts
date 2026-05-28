import { existsSync } from 'node:fs'
import { basename, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { expect, test } from '@playwright/test'

const repoRoot = fileURLToPath(new URL('../../..', import.meta.url))

const realSmokeEnabled = process.env.ASR_REAL_SMOKE === '1'
const realBaseURL = normalizeBaseURL(process.env.ASR_REAL_BASE_URL || 'https://192.168.40.221:9856')
const adminUsername = process.env.ASR_REAL_ADMIN_USERNAME || 'admin'
const adminPassword = process.env.ASR_REAL_ADMIN_PASSWORD
const audioFilePath = process.env.ASR_REAL_AUDIO_FILE || resolve(repoRoot, 'tests/test1.wav')

function normalizeBaseURL(value: string) {
  return value.trim().replace(/\/+$/, '')
}

function getAdminPassword() {
  if (!adminPassword)
    throw new Error('Set ASR_REAL_ADMIN_PASSWORD to run against the real server.')
  return adminPassword
}

async function proxyBackendRequests(page: import('@playwright/test').Page) {
  await page.route('**/*', async (route) => {
    const originalURL = new URL(route.request().url())
    const shouldProxy = originalURL.pathname.startsWith('/api/') || originalURL.pathname.startsWith('/uploads/')
    if (!shouldProxy) {
      await route.fallback()
      return
    }

    try {
      const targetURL = new URL(`${originalURL.pathname}${originalURL.search}`, `${realBaseURL}/`)
      const response = await route.fetch({ url: targetURL.toString(), timeout: 120_000 })
      await route.fulfill({ response })
    }
    catch (error) {
      const message = error instanceof Error ? error.message : String(error)
      if (page.isClosed() || /(?:target page|context|browser).*closed/i.test(message)) {
        await route.abort('aborted').catch(() => {})
        return
      }
      throw error
    }
  })
}

async function login(page: import('@playwright/test').Page) {
  await page.goto('/login')
  await page.getByPlaceholder('请输入用户名').fill(adminUsername)
  await page.getByPlaceholder('请输入密码').fill(getAdminPassword())
  const dashboardResponsePromise = page.waitForResponse(
    response => response.url().includes('/api/admin/dashboard/overview') && response.request().method() === 'GET',
    { timeout: 30_000 },
  ).catch(() => null)
  const loginResponsePromise = page.waitForResponse(
    response => response.url().includes('/api/admin/auth/login') && response.request().method() === 'POST',
    { timeout: 30_000 },
  )
  await page.getByRole('button', { name: '进入系统' }).click()

  const loginResponse = await loginResponsePromise
  if (!loginResponse.ok()) {
    let detail = `HTTP ${loginResponse.status()}`
    try {
      const body = await loginResponse.json() as { message?: string }
      if (body.message)
        detail = `${detail}: ${body.message}`
    }
    catch {
      const body = await loginResponse.text()
      if (body.trim())
        detail = `${detail}: ${body.trim().slice(0, 200)}`
    }
    throw new Error(`Login failed against ${realBaseURL}: ${detail}`)
  }

  await expect(page.locator('header').getByText('数据看板', { exact: true })).toBeVisible({ timeout: 30_000 })
  await dashboardResponsePromise
  await page.waitForLoadState('networkidle', { timeout: 5_000 }).catch(() => {})
}

test.describe('real server smoke', () => {
  test.use({ ignoreHTTPSErrors: true })

  test('logs in and opens the dashboard', async ({ page }) => {
    test.skip(!realSmokeEnabled, 'Set ASR_REAL_SMOKE=1 to run against the real server.')
    test.skip(!adminPassword, 'Set ASR_REAL_ADMIN_PASSWORD to run against the real server.')
    test.setTimeout(90_000)

    await proxyBackendRequests(page)
    await login(page)
  })

  test('uploads test1.wav for batch transcription', async ({ page }) => {
    test.skip(!realSmokeEnabled, 'Set ASR_REAL_SMOKE=1 to run against the real server.')
    test.skip(!adminPassword, 'Set ASR_REAL_ADMIN_PASSWORD to run against the real server.')
    test.skip(!existsSync(audioFilePath), `Missing audio fixture: ${audioFilePath}`)
    test.setTimeout(180_000)

    await proxyBackendRequests(page)
    await login(page)
    await page.goto('/transcription')
    await expect(page.locator('header').getByText('批量转写', { exact: true })).toBeVisible()

    await page.locator('input[type="file"]').setInputFiles(audioFilePath)
    await expect(page.getByPlaceholder('请选择 wav / mp3 音频文件')).toHaveValue(basename(audioFilePath))

    const uploadResponse = page.waitForResponse(
      response => response.url().includes('/api/asr/tasks/upload') && response.request().method() === 'POST',
      { timeout: 120_000 },
    )
    await page.getByRole('button', { name: '上传并转写' }).click()

    expect((await uploadResponse).ok()).toBe(true)
    await expect(page.getByText('任务列表')).toBeVisible()
  })
})
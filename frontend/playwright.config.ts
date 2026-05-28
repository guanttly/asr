import process from 'node:process'

import { defineConfig, devices } from '@playwright/test'

const e2ePort = Number(process.env.ASR_E2E_PORT || 5173)
const e2eBaseURL = `http://127.0.0.1:${e2ePort}`

export default defineConfig({
  testDir: './tests/e2e',
  fullyParallel: false,
  workers: 1,
  timeout: 45_000,
  expect: {
    timeout: 10_000,
  },
  reporter: process.env.CI ? 'github' : 'list',
  use: {
    baseURL: e2eBaseURL,
    trace: 'on-first-retry',
  },
  webServer: {
    command: `pnpm dev --host 127.0.0.1 --port ${e2ePort}`,
    url: e2eBaseURL,
    env: {
      VITE_DEV_HTTPS: 'false',
    },
    reuseExistingServer: !process.env.CI,
    timeout: 120_000,
  },
  projects: [
    {
      name: 'chromium',
      use: { ...devices['Desktop Chrome'], channel: 'chromium' },
    },
  ],
})

import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const repoRoot = fileURLToPath(new URL('../..', import.meta.url))

function readRepoFile(relativePath: string) {
  return readFileSync(resolve(repoRoot, relativePath), 'utf8')
}

describe('playwright e2e server configuration', () => {
  it('uses a configurable port and cross-platform HTTPS env wiring', () => {
    const source = readRepoFile('frontend/playwright.config.ts')

    expect(source).toContain('ASR_E2E_PORT')
    expect(source).toMatch(/--port \$\{e2ePort\}/)
    expect(source).toContain('VITE_DEV_HTTPS: \'false\'')
    expect(source).not.toContain('VITE_DEV_HTTPS=false pnpm dev')
  })
})

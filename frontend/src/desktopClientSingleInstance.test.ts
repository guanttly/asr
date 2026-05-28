import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const repoRoot = fileURLToPath(new URL('../..', import.meta.url))

function readRepoFile(relativePath: string) {
  return readFileSync(resolve(repoRoot, relativePath), 'utf8')
}

describe('desktop client single instance guard', () => {
  it('Electron client acquires the instance lock before creating windows', () => {
    const source = readRepoFile('desktop-electron/electron/main/index.ts')
    const lockIndex = source.indexOf('requestSingleInstanceLock')
    const readyIndex = source.indexOf('app.whenReady')

    expect(lockIndex).toBeGreaterThanOrEqual(0)
    expect(readyIndex).toBeGreaterThanOrEqual(0)
    expect(lockIndex).toBeLessThan(readyIndex)
      expect(source).toMatch(/app\.on\(['"]second-instance['"]/)
    expect(source).toMatch(/showMainWindow|ensureMainWindow/)
  })

  it('Tauri client registers the single-instance plugin and focuses the running window', () => {
    const cargo = readRepoFile('desktop/src-tauri/Cargo.toml')
    const source = readRepoFile('desktop/src-tauri/src/lib.rs')

    expect(cargo).toMatch(/tauri-plugin-single-instance\s*=/)
    expect(source).toContain('tauri_plugin_single_instance')
    expect(source).toMatch(/show_main_window|set_focus/)
  })
})
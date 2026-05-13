import { promises as fs, existsSync, mkdirSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'

export interface MainWindowState {
  x: number
  y: number
}

export function runtimeRootDir() {
  if (process.platform === 'win32') {
    const local = process.env.LOCALAPPDATA
    if (local && local.length > 0)
      return path.join(local, 'asr-desktop')
  }
  return path.join(os.tmpdir(), 'asr-desktop')
}

export function runtimeLogPath() {
  return path.join(runtimeRootDir(), 'logs', 'startup.log')
}

export function mainWindowStatePath() {
  return path.join(runtimeRootDir(), 'main-window-state.json')
}

export async function loadMainWindowState(): Promise<MainWindowState | null> {
  try {
    const raw = await fs.readFile(mainWindowStatePath(), 'utf-8')
    const parsed = JSON.parse(raw)
    if (typeof parsed?.x === 'number' && typeof parsed?.y === 'number')
      return { x: parsed.x, y: parsed.y }
    return null
  }
  catch {
    return null
  }
}

export async function persistMainWindowState(state: MainWindowState) {
  const filePath = mainWindowStatePath()
  const dir = path.dirname(filePath)
  if (!existsSync(dir))
    mkdirSync(dir, { recursive: true })
  await fs.writeFile(filePath, JSON.stringify(state), 'utf-8')
}

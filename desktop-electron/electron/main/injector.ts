import { spawn } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { app, clipboard } from 'electron'

export interface InjectResult {
  success: boolean
  message: string
}

let injectionQueue = Promise.resolve()
const NATIVE_HELPER_TIMEOUT_MS = 1500
const INJECT_HELPER_NAME = 'asr-inject-helper.exe'

function enqueueInjection<T>(task: () => Promise<T>) {
  const run = injectionQueue.then(task, task)
  injectionQueue = run.then(() => undefined, () => undefined)
  return run
}

function resolveInjectHelperPath() {
  const devHelperPath = path.join(__dirname, '..', '..', 'build', 'native', 'win32-x64', INJECT_HELPER_NAME)
  const packagedHelperPath = path.join(process.resourcesPath, 'bin', INJECT_HELPER_NAME)
  const candidate = app.isPackaged ? packagedHelperPath : devHelperPath
  return fs.existsSync(candidate) ? candidate : ''
}

function parseHelperResult(stdout: string): InjectResult | null {
  const lines = stdout
    .split(/\r?\n/u)
    .map(line => line.trim())
    .filter(Boolean)
  const line = lines.at(-1)
  if (!line)
    return null

  const tabIndex = line.indexOf('\t')
  if (tabIndex <= 0)
    return null

  const flag = line.slice(0, tabIndex)
  if (flag !== '0' && flag !== '1')
    return null

  return {
    success: flag === '1',
    message: line.slice(tabIndex + 1) || (flag === '1' ? '注入成功' : '注入失败'),
  }
}

// Win7 注入改为独立原生 helper：避免 PowerShell / Add-Type 被安全策略拦截，
// 并复用与 Tauri Windows 端一致的 Win32 粘贴策略。
function runNativeInjectHelper(text: string): Promise<InjectResult> {
  return new Promise((resolve, reject) => {
    const helperPath = resolveInjectHelperPath()
    if (!helperPath) {
      resolve({
        success: false,
        message: '未找到 Win7 原生注入组件，请重新构建或重新安装应用',
      })
      return
    }

    const child = spawn(helperPath, [], {
      windowsHide: true,
      stdio: ['pipe', 'pipe', 'pipe'],
    })

    let stderr = ''
    let stdout = ''
    let settled = false
    const finish = (result?: InjectResult, error?: Error) => {
      if (settled)
        return
      settled = true
      clearTimeout(timeout)
      if (error)
        reject(error)
      else if (result)
        resolve(result)
      else
        reject(new Error('native inject helper returned no result'))
    }

    const timeout = setTimeout(() => {
      if (settled)
        return
      child.kill()
      finish(undefined, new Error(`native inject helper timed out after ${NATIVE_HELPER_TIMEOUT_MS}ms`))
    }, NATIVE_HELPER_TIMEOUT_MS)

    child.stderr.on('data', (chunk) => { stderr += String(chunk) })
    child.stdout.on('data', (chunk) => { stdout += String(chunk) })
    child.on('error', (error) => {
      finish(undefined, error)
    })
    child.on('close', (code) => {
      if (settled)
        return

      const result = parseHelperResult(stdout)
      if (result) {
        finish(result)
        return
      }

      const details = stderr.trim() || stdout.trim()
      finish(undefined, new Error(`native inject helper exited with code ${code}: ${details || 'no output'}`))
    })

    child.stdin.end(text)
  })
}

export function injectText(text: string): Promise<InjectResult> {
  return enqueueInjection(async () => {
    if (process.platform !== 'win32') {
      return {
        success: false,
        message: '当前操作系统不支持自动粘贴，请手动按 Ctrl+V',
      }
    }

    try {
      return await runNativeInjectHelper(text)
    }
    catch (err) {
      return { success: false, message: `模拟按键失败: ${(err as Error).message}` }
    }
  })
}

export function readClipboard(): string {
  return clipboard.readText()
}

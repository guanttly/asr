import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import { app, clipboard } from 'electron'

export interface InjectResult {
  success: boolean
  message: string
  targetId?: string
  displayName?: string
  state?: string
}

export interface InputBridgeTargetView {
  targetId: string
  displayName: string
  status: string
  processName?: string
  topTitle?: string
  controlClassName?: string
  appType?: string
  lastUsedAt?: number
  useCount?: number
}

export interface InputBridgeStateView {
  supported: boolean
  state: string
  lockedTarget?: InputBridgeTargetView | null
  candidateTarget?: InputBridgeTargetView | null
  history: InputBridgeTargetView[]
  message: string
}

interface BridgeResponse<T> {
  id?: string | number | null
  result?: T
  error?: { code: string, message: string }
}

interface PendingRequest<T> {
  resolve: (value: T) => void
  reject: (error: Error) => void
  timeout: NodeJS.Timeout
}

let injectionQueue = Promise.resolve()
let bridgeProcess: ChildProcessWithoutNullStreams | null = null
let bridgeStdoutBuffer = ''
let nextBridgeId = 1
const pendingBridgeRequests = new Map<string, PendingRequest<unknown>>()

const BRIDGE_REQUEST_TIMEOUT_MS = 4500
const BRIDGE_NAME = 'voice-input-bridge.exe'

function enqueueInjection<T>(task: () => Promise<T>) {
  const run = injectionQueue.then(task, task)
  injectionQueue = run.then(() => undefined, () => undefined)
  return run
}

function resolveBridgePath() {
  const devBridgePath = path.join(__dirname, '..', '..', 'build', 'native', 'win32-x64', BRIDGE_NAME)
  const packagedBridgePath = path.join(process.resourcesPath, 'bin', BRIDGE_NAME)
  const candidate = app.isPackaged ? packagedBridgePath : devBridgePath
  return fs.existsSync(candidate) ? candidate : ''
}

function ensureBridgeProcess() {
  if (bridgeProcess && !bridgeProcess.killed)
    return bridgeProcess

  const bridgePath = resolveBridgePath()
  if (!bridgePath)
    throw new Error('未找到 Windows 输入桥组件，请重新构建或重新安装应用')

  bridgeStdoutBuffer = ''
  bridgeProcess = spawn(bridgePath, ['--stdio'], {
    windowsHide: true,
    stdio: ['pipe', 'pipe', 'pipe'],
  })

  bridgeProcess.stdout.setEncoding('utf8')
  bridgeProcess.stderr.setEncoding('utf8')
  bridgeProcess.stdout.on('data', chunk => handleBridgeStdout(String(chunk)))
  bridgeProcess.stderr.on('data', chunk => console.warn('[voice-input-bridge]', String(chunk).trim()))
  bridgeProcess.on('error', error => rejectAllPending(error))
  bridgeProcess.on('close', code => {
    const error = new Error(`输入桥进程已退出: ${code ?? 'unknown'}`)
    bridgeProcess = null
    rejectAllPending(error)
  })

  return bridgeProcess
}

function handleBridgeStdout(chunk: string) {
  bridgeStdoutBuffer += chunk
  while (true) {
    const newlineIndex = bridgeStdoutBuffer.indexOf('\n')
    if (newlineIndex < 0)
      break

    const line = bridgeStdoutBuffer.slice(0, newlineIndex).trim()
    bridgeStdoutBuffer = bridgeStdoutBuffer.slice(newlineIndex + 1)
    if (!line)
      continue

    let response: BridgeResponse<unknown>
    try {
      response = JSON.parse(line) as BridgeResponse<unknown>
    }
    catch (error) {
      console.warn('[voice-input-bridge] bad json response', line, error)
      continue
    }

    const id = String(response.id ?? '')
    const pending = pendingBridgeRequests.get(id)
    if (!pending)
      continue

    pendingBridgeRequests.delete(id)
    clearTimeout(pending.timeout)
    if (response.error)
      pending.reject(new Error(response.error.message || response.error.code))
    else
      pending.resolve(response.result)
  }
}

function rejectAllPending(error: Error) {
  for (const pending of pendingBridgeRequests.values()) {
    clearTimeout(pending.timeout)
    pending.reject(error)
  }
  pendingBridgeRequests.clear()
}

function sendBridgeRequest<T>(method: string, params: Record<string, unknown> = {}, timeoutMs = BRIDGE_REQUEST_TIMEOUT_MS): Promise<T> {
  if (process.platform !== 'win32')
    return Promise.reject(new Error('当前操作系统不支持 Windows 输入桥'))

  return new Promise((resolve, reject) => {
    let child: ChildProcessWithoutNullStreams
    try {
      child = ensureBridgeProcess()
    }
    catch (error) {
      reject(error as Error)
      return
    }

    const id = String(nextBridgeId++)
    const timeout = setTimeout(() => {
      pendingBridgeRequests.delete(id)
      reject(new Error(`输入桥请求 ${method} 超时`))
    }, timeoutMs)

    pendingBridgeRequests.set(id, { resolve: resolve as (value: unknown) => void, reject, timeout })
    child.stdin.write(`${JSON.stringify({ id, method, params })}\n`, (error) => {
      if (!error)
        return
      pendingBridgeRequests.delete(id)
      clearTimeout(timeout)
      reject(error)
    })
  })
}

function unsupportedState(): InputBridgeStateView {
  return {
    supported: false,
    state: 'Unsupported',
    lockedTarget: null,
    candidateTarget: null,
    history: [],
    message: '当前操作系统不支持 Windows 输入目标绑定。',
  }
}

export function injectText(text: string, source = 'desktop-electron', segmentId?: string): Promise<InjectResult> {
  return enqueueInjection(async () => {
    if (process.platform !== 'win32') {
      return {
        success: false,
        message: '当前操作系统不支持自动粘贴，请手动按 Ctrl+V',
      }
    }

    try {
      return await sendBridgeRequest<InjectResult>('text.paste', {
        text,
        source,
        segmentId,
      }, 6500)
    }
    catch (err) {
      return { success: false, message: `输入桥写入失败: ${(err as Error).message}` }
    }
  })
}

export async function getInputBridgeState(): Promise<InputBridgeStateView> {
  if (process.platform !== 'win32')
    return unsupportedState()
  return await sendBridgeRequest<InputBridgeStateView>('state.get')
}

export async function lockInputTarget(): Promise<InjectResult> {
  if (process.platform !== 'win32')
    return { success: false, message: '当前操作系统不支持输入目标绑定' }
  return await sendBridgeRequest<InjectResult>('target.lockCurrent')
}

export async function unlockInputTarget(): Promise<InjectResult> {
  if (process.platform !== 'win32')
    return { success: false, message: '当前操作系统不支持输入目标绑定' }
  return await sendBridgeRequest<InjectResult>('target.unlock')
}

export async function useHistoryTarget(targetId: string): Promise<InjectResult> {
  if (process.platform !== 'win32')
    return { success: false, message: '当前操作系统不支持历史目标恢复' }
  return await sendBridgeRequest<InjectResult>('target.useHistory', { targetId })
}

export async function deleteHistoryTarget(targetId: string): Promise<InjectResult> {
  if (process.platform !== 'win32')
    return { success: false, message: '当前操作系统不支持历史目标管理' }
  return await sendBridgeRequest<InjectResult>('target.deleteHistory', { targetId })
}

export async function flashInputTargetOverlay(durationMs = 2000): Promise<InjectResult> {
  if (process.platform !== 'win32')
    return { success: false, message: '当前操作系统不支持 Overlay 提示' }
  return await sendBridgeRequest<InjectResult>('overlay.flash', { durationMs })
}

export async function readClipboard(): Promise<string> {
  if (process.platform !== 'win32')
    return clipboard.readText()
  return await sendBridgeRequest<string>('clipboard.readText')
}

export function disposeInputBridge() {
  rejectAllPending(new Error('输入桥进程已关闭'))
  if (bridgeProcess && !bridgeProcess.killed)
    bridgeProcess.kill()
  bridgeProcess = null
}

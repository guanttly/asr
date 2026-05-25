import { app, BrowserWindow, dialog, ipcMain, screen } from 'electron'
import { promises as fs, existsSync, mkdirSync, readFileSync } from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import { createHash } from 'node:crypto'
import { networkInterfaces } from 'node:os'
import { clearInterval, setInterval } from 'node:timers'
import { execFileSync } from 'node:child_process'

import { configureHotkeys } from './hotkeys'
import { deleteHistoryTarget, flashInputTargetOverlay, getInputBridgeState, injectText, lockInputTarget, readClipboard, unlockInputTarget, useHistoryTarget } from './injector'
import { runtimeLogPath, runtimeRootDir } from './window-state'

interface WindowController {
  showMainWindow(): void
  openSettingsWindow(): Promise<void>
  toggleSettingsWindow(): void
  toggleMainWindow(): void
}

let controller: WindowController | null = null

export function bindWindowController(c: WindowController) {
  controller = c
}

function ensureDir(dir: string) {
  if (!existsSync(dir))
    mkdirSync(dir, { recursive: true })
}

function appendRuntimeLog(scope: string, message: string) {
  const filePath = runtimeLogPath()
  ensureDir(path.dirname(filePath))
  const timestamp = (Date.now() / 1000).toFixed(3)
  const line = `[${timestamp} pid=${process.pid}] [${scope}] ${message}\n`
  try {
    require('node:fs').appendFileSync(filePath, line)
  }
  catch (err) {
    console.error('failed to append runtime log', err)
  }
}

function normalizeMachineSeed(value: string | undefined | null): string | null {
  const normalized = String(value || '').trim().replace(/\0/g, '').toLowerCase()
  if (!normalized || normalized === 'none' || normalized === 'unknown' || normalized === 'to be filled by o.e.m.')
    return null
  if (/^[0-]+$/.test(normalized) || /^[f-]+$/.test(normalized))
    return null
  return normalized
}

function commandOutput(command: string, args: string[]): string | null {
  try {
    return execFileSync(command, args, { encoding: 'utf8', timeout: 2500, windowsHide: true }).trim()
  }
  catch {
    return null
  }
}

function platformMachineUuid(): string | null {
  if (process.platform === 'win32') {
    const wmicOutput = commandOutput('wmic', ['csproduct', 'get', 'uuid'])
    if (wmicOutput) {
      for (const line of wmicOutput.split(/\r?\n/)) {
        if (line.trim().toLowerCase() === 'uuid')
          continue
        const uuid = normalizeMachineSeed(line)
        if (uuid)
          return uuid
      }
    }

    const regOutput = commandOutput('reg', ['query', 'HKLM\\SOFTWARE\\Microsoft\\Cryptography', '/v', 'MachineGuid'])
    if (regOutput) {
      for (const line of regOutput.split(/\r?\n/)) {
        if (!line.includes('MachineGuid'))
          continue
        const uuid = normalizeMachineSeed(line.trim().split(/\s+/).at(-1))
        if (uuid)
          return uuid
      }
    }
  }

  if (process.platform === 'linux') {
    for (const filePath of ['/sys/class/dmi/id/product_uuid', '/etc/machine-id']) {
      try {
        const uuid = normalizeMachineSeed(readFileSync(filePath, 'utf8'))
        if (uuid)
          return uuid
      }
      catch {}
    }
  }

  if (process.platform === 'darwin') {
    const output = commandOutput('ioreg', ['-rd1', '-c', 'IOPlatformExpertDevice'])
    if (output) {
      for (const line of output.split(/\r?\n/)) {
        if (!line.includes('IOPlatformUUID'))
          continue
        const uuid = normalizeMachineSeed(line.split('=').at(1)?.trim().replace(/^"|"$/g, ''))
        if (uuid)
          return uuid
      }
    }
  }

  return null
}

function buildMachineCode(hostname: string, platform: string, ipAddresses: string[], macAddresses: string[]) {
  const hardwareSeed = platformMachineUuid()
  const macSeed = normalizeMachineSeed(macAddresses[0])
  const stableSeed = hardwareSeed ? `hardware:${hardwareSeed}` : (macSeed ? `mac:${macSeed}` : null)
  const fingerprint = stableSeed
    ? JSON.stringify({ version: 2, stable_id: stableSeed })
    : JSON.stringify({ version: 1, hostname, platform, ip_addresses: ipAddresses, mac_addresses: macAddresses })
  return createHash('sha256').update(fingerprint).digest('hex')
}

async function readRuntimeLogTail(lines: number): Promise<string> {
  const max = Math.min(Math.max(lines || 120, 1), 400)
  try {
    const content = await fs.readFile(runtimeLogPath(), 'utf-8')
    const arr = content.split(/\r?\n/)
    return arr.slice(-max).join('\n')
  }
  catch (err: unknown) {
    if ((err as NodeJS.ErrnoException).code === 'ENOENT')
      return ''
    throw err
  }
}

function collectMachineIdentity() {
  const hostname = os.hostname().trim()
  const platform = `${process.platform}-${process.arch}`
  const ipSet = new Set<string>()
  const macSet = new Set<string>()

  for (const list of Object.values(networkInterfaces())) {
    if (!list)
      continue
    for (const iface of list) {
      if (iface.internal)
        continue
      ipSet.add(iface.address)
      if (iface.mac && iface.mac !== '00:00:00:00:00:00')
        macSet.add(iface.mac)
    }
  }

  const ipAddresses = [...ipSet].sort()
  const macAddresses = [...macSet].map(item => item.toLowerCase()).sort()
  const machineCode = buildMachineCode(hostname, platform, ipAddresses, macAddresses)

  return {
    machine_code: machineCode,
    hostname,
    platform,
    ip_addresses: ipAddresses,
    mac_addresses: macAddresses,
  }
}

async function savePdfFile(suggestedName: string, pdfBase64: string): Promise<boolean> {
  const focused = BrowserWindow.getFocusedWindow() ?? BrowserWindow.getAllWindows()[0]
  const result = await dialog.showSaveDialog(focused ?? undefined as unknown as BrowserWindow, {
    defaultPath: suggestedName,
    filters: [{ name: 'PDF', extensions: ['pdf'] }],
  })
  if (result.canceled || !result.filePath)
    return false

  let target = result.filePath
  if (!target.toLowerCase().endsWith('.pdf'))
    target += '.pdf'

  await fs.writeFile(target, Buffer.from(pdfBase64, 'base64'))
  appendRuntimeLog('runtime', `saved pdf file to ${target}`)
  return true
}

interface DragSession {
  win: BrowserWindow
  offsetX: number
  offsetY: number
  interval: NodeJS.Timeout
}

let activeDrag: DragSession | null = null

function stopManualDrag() {
  if (!activeDrag)
    return
  clearInterval(activeDrag.interval)
  activeDrag = null
}

function startManualDrag(win: BrowserWindow) {
  stopManualDrag()
  if (win.isDestroyed())
    return

  const cursor = screen.getCursorScreenPoint()
  const [winX, winY] = win.getPosition()
  const offsetX = cursor.x - winX
  const offsetY = cursor.y - winY

  const interval = setInterval(() => {
    if (!activeDrag || activeDrag.win.isDestroyed()) {
      stopManualDrag()
      return
    }
    const point = screen.getCursorScreenPoint()
    activeDrag.win.setPosition(point.x - activeDrag.offsetX, point.y - activeDrag.offsetY)
  }, 16)

  activeDrag = { win, offsetX, offsetY, interval }
}

function performWindowAction(action: string, payload: Record<string, unknown> | undefined, sourceWindow: BrowserWindow | null): unknown {
  const win = sourceWindow ?? BrowserWindow.getFocusedWindow() ?? BrowserWindow.getAllWindows()[0]
  if (!win || win.isDestroyed())
    return undefined

  switch (action) {
    case 'startDragging':
      // Electron 没有原生的 startDragging API（Tauri 上由系统接管），
      // 这里通过定时轮询 cursor 位置在主进程模拟拖动。
      // Win7 的 BrowserWindow 不支持 -webkit-app-region: drag 与点击逻辑共存，
      // 所以必须用这种方式。前端在 mouseup 时会调用 stopDragging 结束。
      startManualDrag(win)
      return undefined
    case 'stopDragging':
      stopManualDrag()
      return undefined
    case 'minimize':
      win.minimize()
      return undefined
    case 'unminimize':
      if (win.isMinimized())
        win.restore()
      return undefined
    case 'close':
      win.close()
      return undefined
    case 'hide':
      win.hide()
      return undefined
    case 'show':
      win.show()
      return undefined
    case 'setFocus':
      win.focus()
      return undefined
    case 'isVisible':
      return win.isVisible()
    case 'setAlwaysOnTop':
      win.setAlwaysOnTop(Boolean(payload?.value))
      return undefined
    case 'setSize': {
      const w = Number(payload?.width)
      const h = Number(payload?.height)
      if (Number.isFinite(w) && Number.isFinite(h))
        win.setSize(Math.round(w), Math.round(h))
      return undefined
    }
    default:
      throw new Error(`unsupported window action: ${action}`)
  }
}

export function registerIpc() {
  ipcMain.handle('asr:invoke', async (event, message: { channel: string, args?: Record<string, unknown>, windowLabel?: string }) => {
    const { channel, args = {}, windowLabel = 'main' } = message
    const sourceWindow = BrowserWindow.fromWebContents(event.sender)

    switch (channel) {
      case 'inject_text':
        return injectText(String(args.text ?? ''))
      case 'input_bridge_get_state':
        return getInputBridgeState()
      case 'input_bridge_lock_current':
        return lockInputTarget()
      case 'input_bridge_unlock':
        return unlockInputTarget()
      case 'input_bridge_use_history':
        return useHistoryTarget(String(args.targetId ?? ''))
      case 'input_bridge_delete_history':
        return deleteHistoryTarget(String(args.targetId ?? ''))
      case 'input_bridge_flash_overlay':
        return flashInputTargetOverlay(Number(args.durationMs ?? 2000))
      case 'input_bridge_paste_text':
        return injectText(String(args.text ?? ''), String(args.source ?? 'desktop-electron'), args.segmentId == null ? undefined : String(args.segmentId))
      case 'read_clipboard':
        return readClipboard()
      case 'configure_hotkeys':
        return configureHotkeys((args.bindings ?? []) as never)
      case 'get_machine_identity':
        return collectMachineIdentity()
      case 'open_settings_window':
        await controller?.openSettingsWindow()
        return undefined
      case 'open_devtools':
        sourceWindow?.webContents.openDevTools({ mode: 'detach' })
        return undefined
      case 'append_runtime_log':
        appendRuntimeLog(String(args.scope ?? ''), String(args.message ?? ''))
        return undefined
      case 'read_runtime_log_tail':
        return readRuntimeLogTail(Number(args.lines ?? 120))
      case 'get_runtime_log_path':
        return runtimeLogPath()
      case 'save_pdf_file':
        return savePdfFile(String(args.suggestedName ?? 'document.pdf'), String(args.pdfBase64 ?? ''))
      case 'window:action':
        return performWindowAction(String(args.action ?? ''), args, sourceWindow)
      case 'event:subscribe':
        // 仅记录订阅，没有需要主进程额外操作的逻辑
        return undefined
      default:
        throw new Error(`unknown ipc channel: ${channel}`)
    }
  })
}

export function disposeIpc() {
  ipcMain.removeHandler('asr:invoke')
}

export { runtimeRootDir }

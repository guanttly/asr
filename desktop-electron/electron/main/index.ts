import { app, BrowserWindow, Menu, session } from 'electron'
import path from 'node:path'

import { bindWindowController, registerIpc } from './ipc'
import { resolveTrayIcon, setupTray } from './tray'
import { disposeHotkeys, setHotkeyEmitter } from './hotkeys'
import { disposeInputBridge, lockInputTarget } from './injector'
import { loadMainWindowState, persistMainWindowState } from './window-state'

// 关闭 Win7 上不支持的硬件加速特性，提高启动稳定性；
// Win10/11 用户走 Tauri 客户端，不会进入这条分支。
app.commandLine.appendSwitch('ignore-certificate-errors')
app.commandLine.appendSwitch('allow-insecure-localhost')
app.commandLine.appendSwitch('allow-running-insecure-content')
app.disableHardwareAcceleration()

const RENDERER_DEV_URL = process.env.ELECTRON_RENDERER_URL || ''
const RENDERER_DIST = path.join(__dirname, '..', '..', 'dist', 'index.html')
const PRELOAD_PATH = path.join(__dirname, '..', 'preload', 'index.cjs')

interface ManagedWindows {
  main?: BrowserWindow
  settings?: BrowserWindow
}

interface WindowShapeRect {
  x: number
  y: number
  width: number
  height: number
}

const windows: ManagedWindows = {}
let isQuitting = false

function buildCircularWindowShape(width: number, height: number): WindowShapeRect[] {
  const centerX = (width - 1) / 2
  const centerY = (height - 1) / 2
  const radiusX = width / 2
  const radiusY = height / 2
  const rects: WindowShapeRect[] = []

  for (let y = 0; y < height; y++) {
    const normalizedY = (y - centerY) / radiusY
    const ratio = Math.sqrt(Math.max(0, 1 - normalizedY * normalizedY))
    const rowWidth = Math.max(1, Math.round(width * ratio))
    rects.push({
      x: Math.round((width - rowWidth) / 2),
      y,
      width: rowWidth,
      height: 1,
    })
  }

  return rects
}

const MAIN_WINDOW_SIZE = 132
const MAIN_WINDOW_BACKGROUND = '#F8FAFC'

function applyMainWindowShape(win: BrowserWindow) {
  const shapedWindow = win as BrowserWindow & { setShape?: (rects: WindowShapeRect[]) => void }
  shapedWindow.setShape?.(buildCircularWindowShape(MAIN_WINDOW_SIZE, MAIN_WINDOW_SIZE))
}

function loadRenderer(win: BrowserWindow, label: 'main' | 'settings') {
  const query = `?label=${label}`
  if (RENDERER_DEV_URL) {
    void win.loadURL(`${RENDERER_DEV_URL}/${query}`)
  }
  else {
    void win.loadFile(RENDERER_DIST, { search: query })
  }
}

function ensureMainWindow(): BrowserWindow {
  if (windows.main && !windows.main.isDestroyed())
    return windows.main

  const win = new BrowserWindow({
    width: MAIN_WINDOW_SIZE,
    height: MAIN_WINDOW_SIZE,
    minWidth: MAIN_WINDOW_SIZE,
    minHeight: MAIN_WINDOW_SIZE,
    maxWidth: MAIN_WINDOW_SIZE,
    maxHeight: MAIN_WINDOW_SIZE,
    x: 100,
    y: 100,
    show: false,
    frame: false,
    // Win7 对透明无边框窗口的合成不稳定，透明区会被渲染成黑边。
    // 这里改为不透明窗口，再配合 setShape 保留圆形悬浮球。
    transparent: false,
    backgroundColor: MAIN_WINDOW_BACKGROUND,
    hasShadow: false,
    alwaysOnTop: true,
    resizable: false,
    skipTaskbar: true,
    title: '巨鲨语音助手',
    webPreferences: {
      preload: PRELOAD_PATH,
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      backgroundThrottling: false,
      webSecurity: false,
      allowRunningInsecureContent: true,
    },
  })

  ;(win as unknown as { __label: string }).__label = 'main'
  windows.main = win
  applyMainWindowShape(win)

  void loadMainWindowState().then((state) => {
    if (state)
      win.setPosition(state.x, state.y)
  })

  win.on('moved', () => {
    const [x, y] = win.getPosition()
    void persistMainWindowState({ x, y })
  })

  win.on('close', (event) => {
    if (isQuitting)
      return
    event.preventDefault()
    const [x, y] = win.getPosition()
    void persistMainWindowState({ x, y })
    win.hide()
  })

  win.once('ready-to-show', () => {
    win.show()
    win.focus()
  })

  loadRenderer(win, 'main')
  return win
}

async function ensureSettingsWindow(): Promise<BrowserWindow> {
  if (windows.settings && !windows.settings.isDestroyed()) {
    if (windows.settings.isMinimized())
      windows.settings.restore()
    windows.settings.show()
    windows.settings.focus()
    return windows.settings
  }

  const win = new BrowserWindow({
    width: 440,
    height: 680,
    minWidth: 400,
    minHeight: 560,
    show: false,
    // 设置窗口恢复原生标题栏，避免 Electron 手动拖动在 Win7 上出现“黏鼠标”问题。
    frame: true,
    autoHideMenuBar: true,
    title: '巨鲨语音助手设置',
    backgroundColor: MAIN_WINDOW_BACKGROUND,
    webPreferences: {
      preload: PRELOAD_PATH,
      contextIsolation: true,
      nodeIntegration: false,
      sandbox: false,
      webSecurity: false,
      allowRunningInsecureContent: true,
    },
  })

  ;(win as unknown as { __label: string }).__label = 'settings'
  windows.settings = win
  win.center()
  win.setMenuBarVisibility(false)

  win.on('closed', () => {
    if (windows.settings === win)
      windows.settings = undefined
  })

  win.once('ready-to-show', () => {
    win.show()
    win.focus()
  })

  loadRenderer(win, 'settings')
  return win
}

function showMainWindow() {
  const win = ensureMainWindow()
  if (win.isMinimized())
    win.restore()
  win.show()
  win.focus()
}

function toggleMainWindow() {
  const win = ensureMainWindow()
  if (win.isVisible() && !win.isMinimized())
    win.hide()
  else
    showMainWindow()
}

function toggleSettingsWindow() {
  const win = windows.settings
  if (win && !win.isDestroyed() && win.isVisible()) {
    win.hide()
    return
  }
  void ensureSettingsWindow()
}

bindWindowController({
  showMainWindow,
  openSettingsWindow: () => ensureSettingsWindow().then(() => undefined),
  toggleSettingsWindow,
  toggleMainWindow,
})

setHotkeyEmitter((action) => {
  switch (action) {
    case 'toggleSettingsWindow':
      toggleSettingsWindow()
      return true
    case 'toggleFloatingWindow':
      toggleMainWindow()
      return true
    case 'lockInputTarget':
      void lockInputTarget().then((result) => {
        if (!result.success)
          console.warn('[input-bridge] lock target from hotkey failed:', result.message)
      }).catch((error) => {
        console.warn('[input-bridge] lock target from hotkey failed:', error)
      })
      return true
    default:
      return false
  }
})

app.whenReady().then(() => {
  Menu.setApplicationMenu(null)

  // 自动允许桌面客户端的麦克风/摄像头/剪贴板请求，与 Tauri 端 PermissionRequested handler 等价
  session.defaultSession.setPermissionRequestHandler((_wc, permission, callback) => {
    if (permission === 'media' || permission === 'clipboard-read' || permission === 'clipboard-sanitized-write')
      callback(true)
    else
      callback(true)
  })
  session.defaultSession.setPermissionCheckHandler(() => true)
  session.defaultSession.setCertificateVerifyProc((_request, callback) => {
    callback(0)
  })

  registerIpc()
  ensureMainWindow()
  setupTray({ showMain: showMainWindow, openSettings: () => void ensureSettingsWindow() }, resolveTrayIcon(__dirname))
})

app.on('certificate-error', (event, _webContents, _url, _error, _certificate, callback) => {
  event.preventDefault()
  callback(true)
})

app.on('window-all-closed', () => {
  // 托盘常驻：不要因为所有窗口关闭就退出
})

app.on('before-quit', () => { isQuitting = true })
app.on('will-quit', () => {
  disposeHotkeys()
  disposeInputBridge()
})

import { app, BrowserWindow, Menu, nativeImage, Tray } from 'electron'
import path from 'node:path'

let trayInstance: Tray | null = null

interface TrayHandlers {
  showMain: () => void
  openSettings: () => void
}

export function setupTray(handlers: TrayHandlers, iconPath?: string) {
  if (trayInstance)
    return trayInstance

  let icon = iconPath ? nativeImage.createFromPath(iconPath) : nativeImage.createEmpty()
  if (icon.isEmpty()) {
    // 兜底：用 16x16 纯色图，保证托盘能显示
    icon = nativeImage.createFromBuffer(Buffer.alloc(16 * 16 * 4, 0xff))
  }

  trayInstance = new Tray(icon)
  trayInstance.setToolTip('巨鲨语音助手')

  const menu = Menu.buildFromTemplate([
    { label: '显示悬浮球', click: () => handlers.showMain() },
    { label: '打开设置', click: () => handlers.openSettings() },
    { type: 'separator' },
    { label: '退出', click: () => app.exit(0) },
  ])
  trayInstance.setContextMenu(menu)

  trayInstance.on('click', () => handlers.showMain())

  return trayInstance
}

export function disposeTray() {
  if (trayInstance) {
    trayInstance.destroy()
    trayInstance = null
  }
}

export function resolveTrayIcon(rendererRoot: string): string {
  // 与 Tauri 端共用 desktop/src-tauri/icons/icon.png 资源；electron-builder 会把它当 extraResource 一起打包
  const candidates = [
    path.join(process.resourcesPath ?? '', 'icons', 'icon.png'),
    path.join(rendererRoot, '..', '..', 'desktop', 'src-tauri', 'icons', 'icon.png'),
    path.join(rendererRoot, 'icon.png'),
  ]
  for (const candidate of candidates) {
    try {
      if (require('node:fs').existsSync(candidate))
        return candidate
    }
    catch {
      // ignore
    }
  }
  return ''
}

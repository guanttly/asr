import { spawn } from 'node:child_process'
import { clipboard } from 'electron'

export interface InjectResult {
  success: boolean
  message: string
}

// 用 PowerShell 的 SendKeys 模拟 Ctrl+V，避免引入与 Electron 22 ABI 绑定的原生依赖；
// Win7 起 .NET Framework 自带 System.Windows.Forms，可在所有目标平台直接执行。
function sendCtrlV(): Promise<void> {
  return new Promise((resolve, reject) => {
    const command = [
      'Add-Type -AssemblyName System.Windows.Forms;',
      "[System.Windows.Forms.SendKeys]::SendWait('^v')",
    ].join(' ')

    const child = spawn('powershell.exe', [
      '-NoProfile',
      '-NonInteractive',
      '-WindowStyle', 'Hidden',
      '-Command', command,
    ], { windowsHide: true })

    let stderr = ''
    child.stderr.on('data', (chunk) => { stderr += String(chunk) })
    child.on('error', reject)
    child.on('close', (code) => {
      if (code === 0)
        resolve()
      else
        reject(new Error(`powershell exited with code ${code}: ${stderr}`))
    })
  })
}

export async function injectText(text: string): Promise<InjectResult> {
  try {
    clipboard.writeText(text)
  }
  catch (err) {
    return { success: false, message: `写入剪贴板失败: ${(err as Error).message}` }
  }

  if (process.platform !== 'win32') {
    return {
      success: false,
      message: '当前操作系统不支持自动粘贴，请手动按 Ctrl+V',
    }
  }

  // 给目标窗口一点时间承接焦点
  await new Promise(r => setTimeout(r, 50))

  try {
    await sendCtrlV()
  }
  catch (err) {
    return { success: false, message: `模拟按键失败: ${(err as Error).message}` }
  }

  return { success: true, message: `已注入 ${text.length} 个字符` }
}

export function readClipboard(): string {
  return clipboard.readText()
}

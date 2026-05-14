import { spawn } from 'node:child_process'
import { clipboard } from 'electron'

export interface InjectResult {
  success: boolean
  message: string
}

let injectionQueue = Promise.resolve()
const POWERSHELL_PASTE_TIMEOUT_MS = 1500

const WINDOWS_NATIVE_PASTE_SCRIPT = `
Add-Type -TypeDefinition @"
using System;
using System.ComponentModel;
using System.Runtime.InteropServices;
using System.Text;
using System.Threading;

public static class NativePaste {
  [StructLayout(LayoutKind.Sequential)]
  private struct INPUT {
    public uint type;
    public InputUnion U;
  }

  [StructLayout(LayoutKind.Explicit)]
  private struct InputUnion {
    [FieldOffset(0)]
    public KEYBDINPUT ki;
  }

  [StructLayout(LayoutKind.Sequential)]
  private struct KEYBDINPUT {
    public ushort wVk;
    public ushort wScan;
    public uint dwFlags;
    public uint time;
    public UIntPtr dwExtraInfo;
  }

  [StructLayout(LayoutKind.Sequential)]
  private struct RECT {
    public int left;
    public int top;
    public int right;
    public int bottom;
  }

  [StructLayout(LayoutKind.Sequential)]
  private struct GUITHREADINFO {
    public int cbSize;
    public int flags;
    public IntPtr hwndActive;
    public IntPtr hwndFocus;
    public IntPtr hwndCapture;
    public IntPtr hwndMenuOwner;
    public IntPtr hwndMoveSize;
    public IntPtr hwndCaret;
    public RECT rcCaret;
  }

  [DllImport("user32.dll", SetLastError = true)]
  private static extern uint SendInput(uint inputCount, INPUT[] inputs, int inputSize);

  [DllImport("user32.dll", SetLastError = true)]
  private static extern bool GetGUIThreadInfo(uint threadId, ref GUITHREADINFO info);

  [DllImport("user32.dll", CharSet = CharSet.Unicode, SetLastError = true)]
  private static extern int GetClassName(IntPtr hWnd, StringBuilder className, int maxCount);

  [DllImport("user32.dll", SetLastError = true)]
  private static extern bool PostMessage(IntPtr hWnd, uint msg, UIntPtr wParam, IntPtr lParam);

  private const uint INPUT_KEYBOARD = 1;
  private const uint KEYEVENTF_KEYUP = 0x0002;
  private const uint KEYEVENTF_SCANCODE = 0x0008;
  private const ushort SCAN_LEFT_CONTROL = 0x001D;
  private const ushort SCAN_V = 0x002F;
  private const uint WM_PASTE = 0x0302;

  private static INPUT CreateScanInput(ushort scanCode, bool keyUp) {
    INPUT input = new INPUT();
    input.type = INPUT_KEYBOARD;
    input.U.ki.wVk = 0;
    input.U.ki.wScan = scanCode;
    input.U.ki.dwFlags = KEYEVENTF_SCANCODE | (keyUp ? KEYEVENTF_KEYUP : 0);
    input.U.ki.time = 0;
    input.U.ki.dwExtraInfo = UIntPtr.Zero;
    return input;
  }

  private static void Send(INPUT[] inputs) {
    uint sent = SendInput((uint)inputs.Length, inputs, Marshal.SizeOf(typeof(INPUT)));
    if (sent != inputs.Length) {
      throw new Win32Exception(
        Marshal.GetLastWin32Error(),
        "SendInput only sent " + sent + "/" + inputs.Length + " keyboard events"
      );
    }
  }

  private static string GetWindowClass(IntPtr hWnd) {
    StringBuilder builder = new StringBuilder(128);
    int length = GetClassName(hWnd, builder, builder.Capacity);
    if (length <= 0) {
      return "";
    }
    return builder.ToString();
  }

  private static bool IsEditableClass(string className) {
    if (String.IsNullOrEmpty(className)) {
      return false;
    }
    return className.IndexOf("Edit", StringComparison.OrdinalIgnoreCase) >= 0
      || className.IndexOf("TextBox", StringComparison.OrdinalIgnoreCase) >= 0
      || className.IndexOf("Scintilla", StringComparison.OrdinalIgnoreCase) >= 0;
  }

  private static bool TryPostPasteToFocusedControl() {
    GUITHREADINFO info = new GUITHREADINFO();
    info.cbSize = Marshal.SizeOf(typeof(GUITHREADINFO));
    if (!GetGUIThreadInfo(0, ref info) || info.hwndFocus == IntPtr.Zero) {
      return false;
    }
    if (!IsEditableClass(GetWindowClass(info.hwndFocus))) {
      return false;
    }
    if (!PostMessage(info.hwndFocus, WM_PASTE, UIntPtr.Zero, IntPtr.Zero)) {
      return false;
    }
    Thread.Sleep(35);
    return true;
  }

  private static void SendCtrlV() {
    Send(new INPUT[] { CreateScanInput(SCAN_LEFT_CONTROL, false) });
    Thread.Sleep(18);
    Send(new INPUT[] { CreateScanInput(SCAN_V, false), CreateScanInput(SCAN_V, true) });
    Thread.Sleep(18);
    Send(new INPUT[] { CreateScanInput(SCAN_LEFT_CONTROL, true) });
  }

  public static void Paste() {
    if (TryPostPasteToFocusedControl()) {
      return;
    }
    SendCtrlV();
  }
}
"@

[NativePaste]::Paste()
`.trim()

function enqueueInjection<T>(task: () => Promise<T>) {
  const run = injectionQueue.then(task, task)
  injectionQueue = run.then(() => undefined, () => undefined)
  return run
}

// Win7 上隐藏 PowerShell 的 SendKeys / keybd_event 对外部前台窗口都不够稳，
// 改为走 user32 SendInput，并串行化每次注入，避免剪贴板与按键事件交叉。
function sendCtrlV(): Promise<void> {
  return new Promise((resolve, reject) => {
    const child = spawn('powershell.exe', [
      '-NoProfile',
      '-ExecutionPolicy', 'Bypass',
      '-NonInteractive',
      '-WindowStyle', 'Hidden',
      '-Command', WINDOWS_NATIVE_PASTE_SCRIPT,
    ], { windowsHide: true })

    let stderr = ''
    let settled = false
    const timeout = setTimeout(() => {
      if (settled)
        return
      settled = true
      child.kill()
      reject(new Error(`powershell paste timed out after ${POWERSHELL_PASTE_TIMEOUT_MS}ms`))
    }, POWERSHELL_PASTE_TIMEOUT_MS)

    child.stderr.on('data', (chunk) => { stderr += String(chunk) })
    child.on('error', (error) => {
      if (settled)
        return
      settled = true
      clearTimeout(timeout)
      reject(error)
    })
    child.on('close', (code) => {
      if (settled)
        return
      settled = true
      clearTimeout(timeout)
      if (code === 0)
        resolve()
      else
        reject(new Error(`powershell exited with code ${code}: ${stderr}`))
    })
  })
}

export function injectText(text: string): Promise<InjectResult> {
  return enqueueInjection(async () => {
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

    // 给目标窗口一点时间承接焦点和最新剪贴板内容。
    await new Promise(r => setTimeout(r, 70))

    try {
      await sendCtrlV()
    }
    catch (err) {
      return { success: false, message: `模拟按键失败: ${(err as Error).message}` }
    }

    return { success: true, message: `已注入 ${text.length} 个字符` }
  })
}

export function readClipboard(): string {
  return clipboard.readText()
}

use serde::{Deserialize, Serialize};
use std::{thread, time::Duration};

#[derive(Debug, Serialize, Deserialize)]
pub struct InjectResult {
    pub success: bool,
    pub message: String,
}

/// Write text to system clipboard and simulate Ctrl+V to paste at cursor position.
#[tauri::command]
pub fn inject_text(text: String) -> InjectResult {
    use arboard::Clipboard;

    // Step 1: Write to clipboard
    let mut clipboard = match Clipboard::new() {
        Ok(c) => c,
        Err(e) => {
            return InjectResult {
                success: false,
                message: format!("无法访问剪贴板: {}", e),
            }
        }
    };

    if let Err(e) = clipboard.set_text(&text) {
        return InjectResult {
            success: false,
            message: format!("写入剪贴板失败: {}", e),
        };
    }

    // Step 2: Small delay to ensure clipboard is ready and target focus is stable.
    thread::sleep(Duration::from_millis(70));

    // Step 3: Paste into the current foreground target.
    if let Err(e) = paste_into_foreground() {
        return InjectResult {
            success: false,
            message: format!("模拟按键失败: {}", e),
        };
    }

    InjectResult {
        success: true,
        message: format!("已注入 {} 个字符", text.len()),
    }
}

/// Read current clipboard content.
#[tauri::command]
pub fn read_clipboard() -> Result<String, String> {
    use arboard::Clipboard;

    let mut clipboard = Clipboard::new().map_err(|e| format!("无法访问剪贴板: {}", e))?;
    clipboard
        .get_text()
        .map_err(|e| format!("读取剪贴板失败: {}", e))
}

#[cfg(windows)]
fn paste_into_foreground() -> Result<(), String> {
    if post_paste_to_focused_edit() {
        return Ok(());
    }
    send_ctrl_v_scancode()
}

#[cfg(windows)]
fn post_paste_to_focused_edit() -> bool {
    use std::mem::size_of;
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        GetClassNameW, GetGUIThreadInfo, PostMessageW, GUITHREADINFO, WM_PASTE,
    };

    unsafe {
        let mut info = GUITHREADINFO {
            cbSize: size_of::<GUITHREADINFO>() as u32,
            ..Default::default()
        };
        if GetGUIThreadInfo(0, &mut info) == 0 || info.hwndFocus.is_null() {
            return false;
        }

        let mut class_buffer = [0u16; 128];
        let class_len = GetClassNameW(
            info.hwndFocus,
            class_buffer.as_mut_ptr(),
            class_buffer.len() as i32,
        );
        if class_len <= 0 {
            return false;
        }

        let class_name = String::from_utf16_lossy(&class_buffer[..class_len as usize]);
        if !is_editable_window_class(&class_name) {
            return false;
        }

        if PostMessageW(info.hwndFocus, WM_PASTE, 0, 0) == 0 {
            return false;
        }
        thread::sleep(Duration::from_millis(35));
        true
    }
}

#[cfg(windows)]
fn is_editable_window_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("edit") || lower.contains("textbox") || lower.contains("scintilla")
}

#[cfg(windows)]
fn send_ctrl_v_scancode() -> Result<(), String> {
    use std::mem::size_of;
    use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
        SendInput, INPUT, INPUT_0, INPUT_KEYBOARD, KEYBDINPUT, KEYEVENTF_KEYUP, KEYEVENTF_SCANCODE,
    };

    const SCAN_LEFT_CONTROL: u16 = 0x1D;
    const SCAN_V: u16 = 0x2F;

    fn input(scan_code: u16, key_up: bool) -> INPUT {
        INPUT {
            r#type: INPUT_KEYBOARD,
            Anonymous: INPUT_0 {
                ki: KEYBDINPUT {
                    wVk: 0,
                    wScan: scan_code,
                    dwFlags: KEYEVENTF_SCANCODE | if key_up { KEYEVENTF_KEYUP } else { 0 },
                    time: 0,
                    dwExtraInfo: 0,
                },
            },
        }
    }

    fn send(inputs: &[INPUT]) -> Result<(), String> {
        let sent = unsafe {
            SendInput(
                inputs.len() as u32,
                inputs.as_ptr(),
                size_of::<INPUT>() as i32,
            )
        };
        if sent != inputs.len() as u32 {
            return Err(format!(
                "SendInput 只发送了 {}/{} 个键盘事件: {}",
                sent,
                inputs.len(),
                std::io::Error::last_os_error()
            ));
        }
        Ok(())
    }

    send(&[input(SCAN_LEFT_CONTROL, false)])?;
    thread::sleep(Duration::from_millis(18));
    send(&[input(SCAN_V, false), input(SCAN_V, true)])?;
    thread::sleep(Duration::from_millis(18));
    send(&[input(SCAN_LEFT_CONTROL, true)])
}

#[cfg(not(windows))]
fn paste_into_foreground() -> Result<(), String> {
    use enigo::{Direction, Enigo, Key, Keyboard, Settings};

    let mut enigo =
        Enigo::new(&Settings::default()).map_err(|e| format!("无法初始化按键模拟: {}", e))?;

    enigo
        .key(Key::Control, Direction::Press)
        .map_err(|e| format!("按下 Ctrl 失败: {}", e))?;
    thread::sleep(Duration::from_millis(10));

    let click_result = enigo.key(Key::Unicode('v'), Direction::Click);
    thread::sleep(Duration::from_millis(10));
    let release_result = enigo.key(Key::Control, Direction::Release);

    click_result.map_err(|e| format!("发送 V 失败: {}", e))?;
    release_result.map_err(|e| format!("释放 Ctrl 失败: {}", e))?;
    Ok(())
}

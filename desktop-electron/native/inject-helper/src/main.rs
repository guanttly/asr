#![cfg_attr(not(windows), allow(dead_code))]

#[cfg(not(windows))]
compile_error!("asr-inject-helper only supports Windows targets");

use std::io::{self, Read, Write};
use std::{thread, time::Duration};

use windows_sys::Win32::Foundation::GlobalFree;
use windows_sys::Win32::System::DataExchange::{
    CloseClipboard, EmptyClipboard, OpenClipboard, SetClipboardData,
};
use windows_sys::Win32::System::Memory::{
    GlobalAlloc, GlobalLock, GlobalUnlock, GMEM_MOVEABLE,
};
use windows_sys::Win32::System::Ole::CF_UNICODETEXT;
use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
    SendInput, INPUT, INPUT_0, INPUT_KEYBOARD, KEYBDINPUT, KEYEVENTF_KEYUP,
    KEYEVENTF_SCANCODE,
};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    GetClassNameW, GetGUIThreadInfo, PostMessageW, GUITHREADINFO, WM_PASTE,
};

const CLIPBOARD_SETTLE_DELAY_MS: u64 = 70;
const POST_PASTE_DELAY_MS: u64 = 35;
const KEYSTROKE_DELAY_MS: u64 = 18;
const CLIPBOARD_OPEN_RETRIES: usize = 10;
const CLIPBOARD_OPEN_RETRY_DELAY_MS: u64 = 40;
const SCAN_LEFT_CONTROL: u16 = 0x1D;
const SCAN_V: u16 = 0x2F;

struct InjectResult {
    success: bool,
    message: String,
}

struct ClipboardGuard;

impl Drop for ClipboardGuard {
    fn drop(&mut self) {
        unsafe {
            CloseClipboard();
        }
    }
}

fn main() {
    let result = match read_stdin_text() {
        Ok(text) => inject_text(text),
        Err(err) => InjectResult {
            success: false,
            message: format!("读取注入文本失败: {err}"),
        },
    };

    emit_result(&result);
}

fn read_stdin_text() -> Result<String, String> {
    let mut text = String::new();
    io::stdin()
        .read_to_string(&mut text)
        .map_err(|err| err.to_string())?;
    if text.is_empty() {
        return Err("注入文本为空".to_string());
    }
    Ok(text)
}

fn inject_text(text: String) -> InjectResult {
    if let Err(err) = write_text_to_clipboard(&text) {
        return InjectResult {
            success: false,
            message: format!("写入剪贴板失败: {err}"),
        };
    }

    thread::sleep(Duration::from_millis(CLIPBOARD_SETTLE_DELAY_MS));

    if let Err(err) = paste_into_foreground() {
        return InjectResult {
            success: false,
            message: format!("模拟按键失败: {err}"),
        };
    }

    InjectResult {
        success: true,
        message: format!("已注入 {} 个字符", text.len()),
    }
}

fn emit_result(result: &InjectResult) {
    let status = if result.success { '1' } else { '0' };
    let sanitized = result.message.replace(['\r', '\n', '\t'], " ");
    let _ = writeln!(io::stdout(), "{status}\t{sanitized}");
    let _ = io::stdout().flush();
}

fn write_text_to_clipboard(text: &str) -> Result<(), String> {
    let utf16: Vec<u16> = text.encode_utf16().chain(std::iter::once(0)).collect();
    let bytes = utf16.len() * std::mem::size_of::<u16>();

    unsafe {
        let mut opened = false;
        for _ in 0..CLIPBOARD_OPEN_RETRIES {
            if OpenClipboard(0) != 0 {
                opened = true;
                break;
            }
            thread::sleep(Duration::from_millis(CLIPBOARD_OPEN_RETRY_DELAY_MS));
        }

        if !opened {
            return Err(last_os_error_message());
        }

        let _guard = ClipboardGuard;

        if EmptyClipboard() == 0 {
            return Err(last_os_error_message());
        }

        let handle = GlobalAlloc(GMEM_MOVEABLE, bytes);
        if handle.is_null() {
            return Err(last_os_error_message());
        }

        let locked = GlobalLock(handle) as *mut u16;
        if locked.is_null() {
            GlobalFree(handle);
            return Err(last_os_error_message());
        }

        std::ptr::copy_nonoverlapping(utf16.as_ptr(), locked, utf16.len());
        GlobalUnlock(handle);

        if SetClipboardData(CF_UNICODETEXT.into(), handle as isize) == 0 {
            GlobalFree(handle);
            return Err(last_os_error_message());
        }
    }

    Ok(())
}

fn paste_into_foreground() -> Result<(), String> {
    if post_paste_to_focused_edit()? {
        return Ok(());
    }

    send_ctrl_v_scancode()
}

fn post_paste_to_focused_edit() -> Result<bool, String> {
    unsafe {
        let mut info = GUITHREADINFO {
            cbSize: std::mem::size_of::<GUITHREADINFO>() as u32,
            ..std::mem::zeroed()
        };

        if GetGUIThreadInfo(0, &mut info) == 0 {
            return Err(last_os_error_message());
        }

        if info.hwndFocus == 0 {
            return Ok(false);
        }

        let class_name = get_window_class_name(info.hwndFocus)?;
        if !is_editable_window_class(&class_name) {
            return Ok(false);
        }

        if PostMessageW(info.hwndFocus, WM_PASTE, 0, 0) == 0 {
            return Err(last_os_error_message());
        }

        thread::sleep(Duration::from_millis(POST_PASTE_DELAY_MS));
        Ok(true)
    }
}

fn get_window_class_name(hwnd: isize) -> Result<String, String> {
    let mut class_buffer = [0u16; 128];
    let class_len = unsafe {
        GetClassNameW(
            hwnd,
            class_buffer.as_mut_ptr(),
            class_buffer.len() as i32,
        )
    };

    if class_len <= 0 {
        return Err(last_os_error_message());
    }

    Ok(String::from_utf16_lossy(&class_buffer[..class_len as usize]))
}

fn is_editable_window_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("edit") || lower.contains("textbox") || lower.contains("scintilla")
}

fn send_ctrl_v_scancode() -> Result<(), String> {
    send_input(&[keyboard_input(SCAN_LEFT_CONTROL, false)])?;
    thread::sleep(Duration::from_millis(KEYSTROKE_DELAY_MS));

    send_input(&[
        keyboard_input(SCAN_V, false),
        keyboard_input(SCAN_V, true),
    ])?;
    thread::sleep(Duration::from_millis(KEYSTROKE_DELAY_MS));

    send_input(&[keyboard_input(SCAN_LEFT_CONTROL, true)])
}

fn keyboard_input(scan_code: u16, key_up: bool) -> INPUT {
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

fn send_input(inputs: &[INPUT]) -> Result<(), String> {
    let sent = unsafe {
        SendInput(
            inputs.len() as u32,
            inputs.as_ptr(),
            std::mem::size_of::<INPUT>() as i32,
        )
    };

    if sent != inputs.len() as u32 {
        return Err(format!(
            "SendInput 只发送了 {}/{} 个键盘事件: {}",
            sent,
            inputs.len(),
            io::Error::last_os_error()
        ));
    }

    Ok(())
}

fn last_os_error_message() -> String {
    io::Error::last_os_error().to_string()
}
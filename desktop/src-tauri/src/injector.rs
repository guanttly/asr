use serde::{Deserialize, Serialize};

#[derive(Debug, Serialize, Deserialize)]
pub struct InjectResult {
    pub success: bool,
    pub message: String,
}

/// Write text to system clipboard and simulate Ctrl+V to paste at cursor position.
#[tauri::command]
pub fn inject_text(text: String) -> InjectResult {
    use arboard::Clipboard;
    use enigo::{Direction, Enigo, Key, Keyboard, Settings};
    use std::thread;
    use std::time::Duration;

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

    // Step 2: Small delay to ensure clipboard is ready
    thread::sleep(Duration::from_millis(50));

    // Step 3: Simulate Ctrl+V
    let mut enigo = match Enigo::new(&Settings::default()) {
        Ok(e) => e,
        Err(e) => {
            return InjectResult {
                success: false,
                message: format!("无法初始化按键模拟: {}", e),
            }
        }
    };

    if let Err(e) = enigo.key(Key::Control, Direction::Press) {
        return InjectResult {
            success: false,
            message: format!("模拟按键失败: {}", e),
        };
    }
    thread::sleep(Duration::from_millis(10));
    let _ = enigo.key(Key::Unicode('v'), Direction::Click);
    thread::sleep(Duration::from_millis(10));
    let _ = enigo.key(Key::Control, Direction::Release);

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

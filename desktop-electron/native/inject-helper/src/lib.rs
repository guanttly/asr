use serde::{Deserialize, Serialize};
use serde_json::Value;

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeTargetView {
    pub target_id: String,
    pub display_name: String,
    pub status: String,
    pub process_name: Option<String>,
    pub top_title: Option<String>,
    pub control_class_name: Option<String>,
    pub app_type: Option<String>,
    pub last_used_at: Option<i64>,
    pub use_count: Option<u32>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeStateView {
    pub supported: bool,
    pub state: String,
    pub locked_target: Option<BridgeTargetView>,
    pub candidate_target: Option<BridgeTargetView>,
    pub history: Vec<BridgeTargetView>,
    pub message: String,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeCommandResult {
    pub success: bool,
    pub message: String,
    pub target_id: Option<String>,
    pub display_name: Option<String>,
    pub state: Option<String>,
}

#[derive(Debug, Clone, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeRequest {
    pub id: Option<Value>,
    pub method: String,
    #[serde(default)]
    pub params: Value,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeError {
    pub code: String,
    pub message: String,
}

#[derive(Debug, Clone, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct BridgeResponse {
    pub id: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub result: Option<Value>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub error: Option<BridgeError>,
}

#[cfg(windows)]
mod windows_impl;

#[cfg(windows)]
pub use windows_impl::{
    delete_history_target, flash_locked_overlay, get_state, legacy_inject_text,
    lock_current_target, paste_text, read_clipboard_text, unlock_target, use_history_target,
};

#[cfg(not(windows))]
pub fn get_state() -> BridgeStateView {
    BridgeStateView {
        supported: false,
        state: "Unsupported".to_string(),
        locked_target: None,
        candidate_target: None,
        history: Vec::new(),
        message: "当前平台不支持 Windows 输入目标绑定，打包到 Windows 后生效。".to_string(),
    }
}

#[cfg(not(windows))]
pub fn lock_current_target() -> BridgeCommandResult {
    unsupported_result("当前平台不支持输入目标绑定")
}

#[cfg(not(windows))]
pub fn unlock_target() -> BridgeCommandResult {
    unsupported_result("当前平台不支持输入目标绑定")
}

#[cfg(not(windows))]
pub fn use_history_target(_target_id: &str) -> BridgeCommandResult {
    unsupported_result("当前平台不支持历史目标恢复")
}

#[cfg(not(windows))]
pub fn delete_history_target(_target_id: &str) -> BridgeCommandResult {
    unsupported_result("当前平台不支持历史目标管理")
}

#[cfg(not(windows))]
pub fn flash_locked_overlay(_duration_ms: u64) -> BridgeCommandResult {
    unsupported_result("当前平台不支持 Overlay 提示")
}

#[cfg(not(windows))]
pub fn paste_text(
    _text: &str,
    _source: Option<&str>,
    _segment_id: Option<&str>,
) -> BridgeCommandResult {
    unsupported_result("当前平台不支持自动粘贴，请手动复制文本")
}

#[cfg(not(windows))]
pub fn legacy_inject_text(_text: &str) -> BridgeCommandResult {
    unsupported_result("当前平台不支持自动粘贴，请手动复制文本")
}

#[cfg(not(windows))]
pub fn read_clipboard_text() -> Result<String, String> {
    Err("当前平台不支持读取系统剪贴板".to_string())
}

#[cfg(not(windows))]
fn unsupported_result(message: &str) -> BridgeCommandResult {
    BridgeCommandResult {
        success: false,
        message: message.to_string(),
        target_id: None,
        display_name: None,
        state: Some("Unsupported".to_string()),
    }
}

pub fn handle_request(request: BridgeRequest) -> BridgeResponse {
    let id = request.id.clone();
    let result = match request.method.as_str() {
        "state.get" => serde_json::to_value(get_state()),
        "target.lockCurrent" => serde_json::to_value(lock_current_target()),
        "target.unlock" => serde_json::to_value(unlock_target()),
        "target.useHistory" => {
            let target_id = request
                .params
                .get("targetId")
                .and_then(Value::as_str)
                .unwrap_or_default();
            serde_json::to_value(use_history_target(target_id))
        }
        "target.deleteHistory" => {
            let target_id = request
                .params
                .get("targetId")
                .and_then(Value::as_str)
                .unwrap_or_default();
            serde_json::to_value(delete_history_target(target_id))
        }
        "text.paste" => {
            let text = request
                .params
                .get("text")
                .and_then(Value::as_str)
                .unwrap_or_default();
            let source = request.params.get("source").and_then(Value::as_str);
            let segment_id = request.params.get("segmentId").and_then(Value::as_str);
            serde_json::to_value(paste_text(text, source, segment_id))
        }
        "overlay.flash" => {
            let duration_ms = request
                .params
                .get("durationMs")
                .and_then(Value::as_u64)
                .unwrap_or(2000);
            serde_json::to_value(flash_locked_overlay(duration_ms))
        }
        "clipboard.readText" => match read_clipboard_text() {
            Ok(text) => Ok(Value::String(text)),
            Err(message) => {
                return BridgeResponse {
                    id,
                    result: None,
                    error: Some(BridgeError {
                        code: "clipboard_error".to_string(),
                        message,
                    }),
                }
            }
        },
        _ => {
            return BridgeResponse {
                id,
                result: None,
                error: Some(BridgeError {
                    code: "method_not_found".to_string(),
                    message: format!("unsupported bridge method: {}", request.method),
                }),
            }
        }
    };

    match result {
        Ok(value) => BridgeResponse {
            id,
            result: Some(value),
            error: None,
        },
        Err(err) => BridgeResponse {
            id,
            result: None,
            error: Some(BridgeError {
                code: "internal_error".to_string(),
                message: err.to_string(),
            }),
        },
    }
}

pub use voice_input_bridge::{BridgeCommandResult, BridgeStateView};

#[tauri::command]
pub fn input_bridge_get_state() -> BridgeStateView {
    voice_input_bridge::get_state()
}

#[tauri::command]
pub fn input_bridge_lock_current() -> BridgeCommandResult {
    voice_input_bridge::lock_current_target()
}

#[tauri::command]
pub fn input_bridge_unlock() -> BridgeCommandResult {
    voice_input_bridge::unlock_target()
}

#[tauri::command]
pub fn input_bridge_use_history(target_id: String) -> BridgeCommandResult {
    voice_input_bridge::use_history_target(&target_id)
}

#[tauri::command]
pub fn input_bridge_delete_history(target_id: String) -> BridgeCommandResult {
    voice_input_bridge::delete_history_target(&target_id)
}

#[tauri::command]
pub fn input_bridge_flash_overlay(duration_ms: Option<u64>) -> BridgeCommandResult {
    voice_input_bridge::flash_locked_overlay(duration_ms.unwrap_or(2000))
}

#[tauri::command]
pub fn input_bridge_paste_text(
    text: String,
    source: Option<String>,
    segment_id: Option<String>,
) -> BridgeCommandResult {
    voice_input_bridge::paste_text(&text, source.as_deref(), segment_id.as_deref())
}

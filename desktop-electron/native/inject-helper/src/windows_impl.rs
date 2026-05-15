use crate::{BridgeCommandResult, BridgeStateView, BridgeTargetView};
use serde::{Deserialize, Serialize};
use std::cmp::Reverse;
use std::collections::hash_map::DefaultHasher;
use std::fs::{self, OpenOptions};
use std::hash::{Hash, Hasher};
use std::io::Write;
use std::path::PathBuf;
use std::sync::{Mutex, Once};
use std::thread;
use std::time::{Duration, SystemTime, UNIX_EPOCH};

use windows::Win32::Foundation::{POINT as UiaPoint, RECT as UiaRect};
use windows::Win32::System::Com::{
    CoCreateInstance, CoInitializeEx, CoUninitialize, CLSCTX_INPROC_SERVER,
    COINIT_APARTMENTTHREADED,
};
use windows::Win32::UI::Accessibility::{
    CUIAutomation, IUIAutomation, IUIAutomationElement, TreeScope_Descendants,
    UIA_ComboBoxControlTypeId, UIA_CustomControlTypeId, UIA_DocumentControlTypeId,
    UIA_EditControlTypeId, UIA_PaneControlTypeId, UIA_TextControlTypeId, UIA_TextPatternId,
    UIA_ValuePatternId, UIA_CONTROLTYPE_ID,
};
use windows_sys::Win32::Foundation::{
    CloseHandle, GlobalFree, BOOL, HWND, LPARAM, LRESULT, POINT as SysPoint, RECT, SIZE, WPARAM,
};
use windows_sys::Win32::Graphics::Gdi::{
    CreateFontW, CreatePen, CreateSolidBrush, DeleteObject, FillRect, GetDC, GetTextExtentPoint32W,
    ReleaseDC, RoundRect, SelectObject, SetBkMode, SetTextColor, TextOutW, UpdateWindow,
    ANSI_CHARSET, ANTIALIASED_QUALITY, CLIP_DEFAULT_PRECIS, DEFAULT_PITCH, FF_DONTCARE, FW_NORMAL,
    FW_SEMIBOLD, OUT_TT_PRECIS, PS_SOLID, TRANSPARENT,
};
use windows_sys::Win32::System::DataExchange::{
    CloseClipboard, EmptyClipboard, GetClipboardData, IsClipboardFormatAvailable, OpenClipboard,
    SetClipboardData,
};
use windows_sys::Win32::System::LibraryLoader::GetModuleHandleW;
use windows_sys::Win32::System::Memory::{GlobalAlloc, GlobalLock, GlobalUnlock, GMEM_MOVEABLE};
use windows_sys::Win32::System::Ole::CF_UNICODETEXT;
use windows_sys::Win32::System::Threading::{
    AttachThreadInput, GetCurrentThreadId, OpenProcess, QueryFullProcessImageNameW,
    PROCESS_QUERY_LIMITED_INFORMATION,
};
use windows_sys::Win32::UI::Accessibility::{SetWinEventHook, UnhookWinEvent, HWINEVENTHOOK};
use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
    SendInput, SetFocus, INPUT, INPUT_0, INPUT_KEYBOARD, KEYBDINPUT, KEYEVENTF_KEYUP,
    KEYEVENTF_SCANCODE,
};
use windows_sys::Win32::UI::WindowsAndMessaging::{
    BringWindowToTop, CreateWindowExW, DefWindowProcW, DestroyWindow, DispatchMessageW,
    EnumChildWindows, EnumWindows, GetAncestor, GetClassNameW, GetCursorPos, GetForegroundWindow,
    GetGUIThreadInfo, GetMessageW, GetParent, GetSystemMetrics, GetWindowRect,
    GetWindowTextLengthW, GetWindowTextW, GetWindowThreadProcessId, IsIconic, IsWindow,
    IsWindowVisible, PostMessageW, RegisterClassW, SetForegroundWindow, SetLayeredWindowAttributes,
    ShowWindow, TranslateMessage, EVENT_OBJECT_FOCUS, GA_ROOT, GUITHREADINFO, HTTRANSPARENT,
    LWA_ALPHA, LWA_COLORKEY, MSG, OBJID_CLIENT, OBJID_WINDOW, SM_CXVIRTUALSCREEN,
    SM_CYVIRTUALSCREEN, SM_XVIRTUALSCREEN, SM_YVIRTUALSCREEN, SW_RESTORE, SW_SHOWNOACTIVATE,
    WINEVENT_OUTOFCONTEXT, WINEVENT_SKIPOWNPROCESS, WM_NCHITTEST, WM_PASTE, WNDCLASSW,
    WS_EX_LAYERED, WS_EX_NOACTIVATE, WS_EX_TOOLWINDOW, WS_EX_TOPMOST, WS_EX_TRANSPARENT, WS_POPUP,
};

const HISTORY_VERSION: u32 = 1;
const MAX_HISTORY_ITEMS: usize = 20;
const RECOVER_SCORE_THRESHOLD: i32 = 80;
const CLIPBOARD_SETTLE_DELAY_MS: u64 = 70;
const POST_PASTE_DELAY_MS: u64 = 55;
const WEB_POST_PASTE_DELAY_MS: u64 = 180;
const KEYSTROKE_DELAY_MS: u64 = 18;
const CLIPBOARD_OPEN_RETRIES: usize = 10;
const CLIPBOARD_OPEN_RETRY_DELAY_MS: u64 = 40;
const SCAN_LEFT_CONTROL: u16 = 0x1D;
const SCAN_V: u16 = 0x2F;
const OVERLAY_CLASS_NAME: &str = "jushaVoiceInputBridgeOverlay";
const OVERLAY_COLOR_KEY: u32 = 0x00010101;
const OVERLAY_FADE_IN_MS: u64 = 180;
const OVERLAY_FADE_OUT_MS: u64 = 220;
const OVERLAY_MIN_HOLD_MS: u64 = 650;
const OVERLAY_DEFAULT_BOUND_MS: u64 = 2200;
const OVERLAY_DEFAULT_SUCCESS_MS: u64 = 1500;
const OVERLAY_DEFAULT_INVALID_MS: u64 = 1600;
const OVERLAY_BRACKET_LENGTH: i32 = 22;
const OVERLAY_BRACKET_THICKNESS: i32 = 3;
const OVERLAY_LABEL_PADDING_X: i32 = 14;
const OVERLAY_LABEL_PADDING_Y: i32 = 7;
const OVERLAY_LABEL_DOT_DIAMETER: i32 = 8;
const OVERLAY_LABEL_DOT_GAP: i32 = 10;
const OVERLAY_LABEL_RADIUS: i32 = 18;
const OVERLAY_LABEL_FONT_HEIGHT: i32 = 18;
const OVERLAY_LABEL_FACE_PRIMARY: &str = "Microsoft YaHei UI";
const OVERLAY_LABEL_FACE_FALLBACK: &str = "Segoe UI";
const MAX_ACCESSIBILITY_DESCENDANTS: i32 = 512;
const MAX_ACCESSIBILITY_ANCESTORS: usize = 12;

static OVERLAY_CLASS: Once = Once::new();

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RuntimeTarget {
    top_hwnd: isize,
    focus_hwnd: isize,
    process_id: u32,
    thread_id: u32,
}

#[derive(Debug, Clone, Copy, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct RectHint {
    left: i32,
    top: i32,
    right: i32,
    bottom: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct TargetSignature {
    process_name: String,
    exe_path: Option<String>,
    top_title: Option<String>,
    top_class_name: Option<String>,
    control_class_name: Option<String>,
    automation_id: Option<String>,
    control_name: Option<String>,
    control_type: Option<String>,
    rect_hint: Option<RectHint>,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BoundTargetHistoryItem {
    id: String,
    display_name: String,
    signature: TargetSignature,
    last_runtime: Option<RuntimeTarget>,
    last_bound_at: i64,
    last_used_at: i64,
    use_count: u32,
    app_type: String,
    priority: i32,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct HistoryFile {
    version: u32,
    locked_target_id: Option<String>,
    targets: Vec<BoundTargetHistoryItem>,
}

impl Default for HistoryFile {
    fn default() -> Self {
        Self {
            version: HISTORY_VERSION,
            locked_target_id: None,
            targets: Vec::new(),
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
struct BridgeConfig {
    allowed_apps: Vec<String>,
    blocked_apps: Vec<String>,
    fallback_to_current_focus: bool,
    require_whitelist_for_fallback: bool,
}

impl Default for BridgeConfig {
    fn default() -> Self {
        Self {
            allowed_apps: vec![
                "ris.exe".to_string(),
                "his.exe".to_string(),
                "chrome.exe".to_string(),
                "msedge.exe".to_string(),
                "firefox.exe".to_string(),
                "iexplore.exe".to_string(),
                "notepad.exe".to_string(),
                "winword.exe".to_string(),
                "excel.exe".to_string(),
                "powerpnt.exe".to_string(),
                "wps.exe".to_string(),
                "et.exe".to_string(),
                "wpp.exe".to_string(),
                "wechat.exe".to_string(),
                "dingtalk.exe".to_string(),
                "wework.exe".to_string(),
            ],
            blocked_apps: vec![
                "pacs.exe".to_string(),
                "qq.exe".to_string(),
                "explorer.exe".to_string(),
                "taskmgr.exe".to_string(),
                "cmd.exe".to_string(),
                "powershell.exe".to_string(),
                "windowsterminal.exe".to_string(),
            ],
            fallback_to_current_focus: true,
            require_whitelist_for_fallback: true,
        }
    }
}

#[derive(Debug, Clone)]
struct InputTarget {
    id: String,
    display_name: String,
    runtime: RuntimeTarget,
    signature: TargetSignature,
    app_type: String,
    priority: i32,
}

#[derive(Debug, Clone)]
struct AccessibilityFocusHint {
    rect_hint: RectHint,
    control_type: Option<String>,
    automation_id: Option<String>,
    control_name: Option<String>,
}

#[derive(Debug, Clone)]
struct AccessibilityCandidate {
    hint: AccessibilityFocusHint,
    score: i32,
    area: i64,
}

#[derive(Debug, Clone)]
struct ClipboardSnapshot {
    text: Option<String>,
}

struct ClipboardGuard;

impl Drop for ClipboardGuard {
    fn drop(&mut self) {
        unsafe {
            CloseClipboard();
        }
    }
}

pub fn get_state() -> BridgeStateView {
    ensure_focus_tracker();
    let history = load_history();
    let candidate = detect_current_focused_editable().ok();
    let locked_item = history
        .locked_target_id
        .as_ref()
        .and_then(|id| history.targets.iter().find(|item| &item.id == id));

    let locked_target = locked_item.map(history_item_to_view);
    let locked_valid = locked_item
        .and_then(|item| resolve_history_item(item).ok())
        .is_some();

    let state = if locked_valid {
        "Locked"
    } else if locked_target.is_some() {
        "Invalid"
    } else if candidate.is_some() {
        "CandidateReady"
    } else {
        "Idle"
    };

    let mut history_views: Vec<BridgeTargetView> =
        history.targets.iter().map(history_item_to_view).collect();
    history_views.sort_by_key(|item| Reverse(item.last_used_at.unwrap_or_default()));

    BridgeStateView {
        supported: true,
        state: state.to_string(),
        locked_target,
        candidate_target: candidate.as_ref().map(target_to_candidate_view),
        history: history_views,
        message: match state {
            "Locked" => "已锁定语音写入目标".to_string(),
            "Invalid" => "绑定目标已失效，请重新点击报告框并按绑定热键".to_string(),
            "CandidateReady" => "检测到候选输入框，可按绑定热键锁定".to_string(),
            _ => "尚未检测到可写入目标".to_string(),
        },
    }
}

pub fn lock_current_target() -> BridgeCommandResult {
    ensure_focus_tracker();
    match detect_current_focused_editable() {
        Ok(target) => {
            let config = load_config();
            if is_blocked_target(&target, &config) {
                return command_error(
                    "Invalid",
                    format!("{} 位于黑名单应用中，已拒绝绑定", target.display_name),
                );
            }

            let mut history = load_history();
            upsert_locked_target(&mut history, target.clone());
            if let Err(err) = save_history(&history) {
                return command_error("Invalid", format!("保存绑定历史失败: {err}"));
            }

            append_log(&format!(
                "target_locked target={} process={} title={}",
                target.id,
                target.signature.process_name,
                target.signature.top_title.clone().unwrap_or_default()
            ));
            flash_overlay_for_target(&target, "语音写入目标", OverlayKind::Bound, 2000);

            command_success(
                "Locked",
                format!("已绑定语音写入目标：{}", target.display_name),
                Some(target.id),
                Some(target.display_name),
            )
        }
        Err(err) => {
            append_log(&format!("target_lock_failed reason={err}"));
            command_error("Idle", format!("未检测到可绑定输入框：{err}"))
        }
    }
}

pub fn unlock_target() -> BridgeCommandResult {
    let mut history = load_history();
    history.locked_target_id = None;
    if let Err(err) = save_history(&history) {
        return command_error("Invalid", format!("解除绑定失败: {err}"));
    }
    append_log("target_unlocked");
    command_success("Idle", "已解除语音写入目标绑定".to_string(), None, None)
}

pub fn use_history_target(target_id: &str) -> BridgeCommandResult {
    if target_id.trim().is_empty() {
        return command_error("Invalid", "缺少历史目标 ID".to_string());
    }

    let mut history = load_history();
    let item = history
        .targets
        .iter()
        .find(|item| item.id == target_id)
        .cloned();
    let Some(item) = item else {
        return command_error("Invalid", "未找到指定历史目标".to_string());
    };

    match resolve_history_item(&item) {
        Ok(target) => {
            history.locked_target_id = Some(target.id.clone());
            update_history_runtime(&mut history, &target, false);
            if let Err(err) = save_history(&history) {
                return command_error("Invalid", format!("保存绑定历史失败: {err}"));
            }
            flash_overlay_for_target(&target, "已切换语音写入目标", OverlayKind::Bound, 1800);
            command_success(
                "Locked",
                format!("已使用历史目标：{}", target.display_name),
                Some(target.id),
                Some(target.display_name),
            )
        }
        Err(err) => command_error("Invalid", format!("历史目标不可用：{err}")),
    }
}

pub fn delete_history_target(target_id: &str) -> BridgeCommandResult {
    if target_id.trim().is_empty() {
        return command_error("Invalid", "缺少历史目标 ID".to_string());
    }

    let mut history = load_history();
    let before = history.targets.len();
    history.targets.retain(|item| item.id != target_id);
    if history.locked_target_id.as_deref() == Some(target_id) {
        history.locked_target_id = None;
    }

    if before == history.targets.len() {
        return command_error("Invalid", "未找到指定历史目标".to_string());
    }

    if let Err(err) = save_history(&history) {
        return command_error("Invalid", format!("删除历史目标失败: {err}"));
    }

    command_success("Idle", "已删除历史绑定目标".to_string(), None, None)
}

pub fn flash_locked_overlay(duration_ms: u64) -> BridgeCommandResult {
    let history = load_history();
    let target = history
        .locked_target_id
        .as_ref()
        .and_then(|id| history.targets.iter().find(|item| &item.id == id))
        .and_then(|item| resolve_history_item(item).ok());

    let Some(target) = target else {
        return command_error("Invalid", "当前没有可提示的绑定目标".to_string());
    };

    flash_overlay_for_target(&target, "语音写入目标", OverlayKind::Bound, duration_ms);
    command_success(
        "Locked",
        format!("已提示当前写入目标：{}", target.display_name),
        Some(target.id),
        Some(target.display_name),
    )
}

pub fn paste_text(
    text: &str,
    source: Option<&str>,
    segment_id: Option<&str>,
) -> BridgeCommandResult {
    ensure_focus_tracker();
    if text.is_empty() {
        return command_error("Invalid", "写入文本为空".to_string());
    }

    match resolve_target() {
        Ok((target, state)) => match paste_to_target(&target, text) {
            Ok(()) => {
                let mut history = load_history();
                update_history_runtime(&mut history, &target, true);
                history.locked_target_id = Some(target.id.clone());
                let _ = save_history(&history);

                append_log(&format!(
                    "paste_success target={} process={} source={} segment={} length={}",
                    target.id,
                    target.signature.process_name,
                    source.unwrap_or("unknown"),
                    segment_id.unwrap_or(""),
                    text.chars().count()
                ));
                flash_overlay_for_target(&target, "已写入", OverlayKind::Success, 650);

                command_success(
                    &state,
                    format!("已写入到 {}", target.display_name),
                    Some(target.id),
                    Some(target.display_name),
                )
            }
            Err(err) => {
                append_log(&format!(
                    "paste_failed target={} state={} error={}",
                    target.id, state, err
                ));
                flash_overlay_for_target(&target, "写入失败", OverlayKind::Invalid, 1200);
                command_error(&state, format!("写入失败: {err}"))
            }
        },
        Err(err) => {
            append_log(&format!("paste_no_target error={err}"));
            command_error("Invalid", err)
        }
    }
}

pub fn legacy_inject_text(text: &str) -> BridgeCommandResult {
    if text.is_empty() {
        return command_error("Invalid", "注入文本为空".to_string());
    }

    match detect_current_focused_editable()
        .and_then(|target| paste_to_target(&target, text).map(|_| target))
    {
        Ok(target) => command_success(
            "FallbackCurrentFocus",
            format!("已注入 {} 个字符", text.chars().count()),
            Some(target.id),
            Some(target.display_name),
        ),
        Err(err) => command_error("Invalid", format!("模拟按键失败: {err}")),
    }
}

pub fn read_clipboard_text() -> Result<String, String> {
    read_clipboard_unicode()?.ok_or_else(|| "剪贴板没有文本内容".to_string())
}

fn resolve_target() -> Result<(InputTarget, String), String> {
    let config = load_config();
    let mut history = load_history();

    if let Some(locked_id) = history.locked_target_id.clone() {
        if let Some(item) = history
            .targets
            .iter()
            .find(|item| item.id == locked_id)
            .cloned()
        {
            match resolve_history_item(&item) {
                Ok(target) => {
                    if is_blocked_target(&target, &config) {
                        return Err("绑定目标命中黑名单，已拒绝写入".to_string());
                    }
                    return Ok((target, "Locked".to_string()));
                }
                Err(err) => {
                    append_log(&format!(
                        "locked_target_invalid target={} error={err}",
                        item.id
                    ));
                }
            }
        }
    }

    let mut items = history.targets.clone();
    items.sort_by_key(|item| Reverse((item.priority, item.last_used_at, item.use_count)));
    for item in items {
        if let Ok(target) = resolve_history_item(&item) {
            if is_blocked_target(&target, &config) {
                continue;
            }
            history.locked_target_id = Some(target.id.clone());
            update_history_runtime(&mut history, &target, false);
            let _ = save_history(&history);
            append_log(&format!("target_recovered target={}", target.id));
            flash_overlay_for_target(&target, "已自动恢复最近绑定目标", OverlayKind::Bound, 1400);
            return Ok((target, "Recovering".to_string()));
        }
    }

    if config.fallback_to_current_focus {
        if let Ok(target) = detect_current_focused_editable() {
            if is_allowed_fallback_target(&target, &config) {
                append_log(&format!("target_fallback_current target={}", target.id));
                return Ok((target, "FallbackCurrentFocus".to_string()));
            }
        }
    }

    Err("未找到可用写入目标，请点击报告输入框并按绑定热键".to_string())
}

fn resolve_history_item(item: &BoundTargetHistoryItem) -> Result<InputTarget, String> {
    if let Some(runtime) = item.last_runtime.clone() {
        if is_runtime_target_alive(&runtime, &item.signature) {
            return Ok(InputTarget {
                id: item.id.clone(),
                display_name: item.display_name.clone(),
                runtime,
                signature: item.signature.clone(),
                app_type: item.app_type.clone(),
                priority: item.priority,
            });
        }
    }

    recover_by_signature(item).ok_or_else(|| "无法通过历史签名恢复目标".to_string())
}

fn recover_by_signature(item: &BoundTargetHistoryItem) -> Option<InputTarget> {
    let hwnds = enum_top_windows();
    let mut best: Option<(i32, InputTarget)> = None;

    for top_hwnd in hwnds {
        if unsafe { IsWindowVisible(top_hwnd) } == 0 {
            continue;
        }
        if is_own_window(top_hwnd) {
            continue;
        }

        let Some(focus_hwnd) = find_recoverable_focus_hwnd(top_hwnd, &item.signature) else {
            continue;
        };
        let Ok(target) = build_target(top_hwnd, focus_hwnd) else {
            continue;
        };
        let score = score_recovered_target(item, &target);
        if score >= RECOVER_SCORE_THRESHOLD {
            let replace = best
                .as_ref()
                .map(|(best_score, _)| score > *best_score)
                .unwrap_or(true);
            if replace {
                best = Some((
                    score,
                    InputTarget {
                        id: item.id.clone(),
                        display_name: item.display_name.clone(),
                        ..target
                    },
                ));
            }
        }
    }

    best.map(|(_, target)| target)
}

fn find_recoverable_focus_hwnd(top_hwnd: HWND, signature: &TargetSignature) -> Option<HWND> {
    let expected = signature
        .control_class_name
        .as_deref()
        .unwrap_or_default()
        .to_ascii_lowercase();

    if expected.is_empty() {
        return find_first_editable_child(top_hwnd);
    }

    if get_window_class_name(top_hwnd)
        .ok()
        .map(|name| class_matches(&name, &expected))
        .unwrap_or(false)
    {
        return Some(top_hwnd);
    }

    find_child_by_class(top_hwnd, &expected).or_else(|| find_first_editable_child(top_hwnd))
}

fn score_recovered_target(item: &BoundTargetHistoryItem, target: &InputTarget) -> i32 {
    let mut score = 0;
    if same_ci(&item.signature.process_name, &target.signature.process_name) {
        score += 40;
    }
    if same_opt_ci(
        item.signature.exe_path.as_deref(),
        target.signature.exe_path.as_deref(),
    ) {
        score += 30;
    }
    if same_opt_ci(
        item.signature.top_class_name.as_deref(),
        target.signature.top_class_name.as_deref(),
    ) {
        score += 15;
    }
    if title_similar(
        item.signature.top_title.as_deref(),
        target.signature.top_title.as_deref(),
    ) {
        score += 10;
    }
    if same_opt_ci(
        item.signature.control_class_name.as_deref(),
        target.signature.control_class_name.as_deref(),
    ) {
        score += 20;
    }
    if rect_close(
        item.signature.rect_hint.as_ref(),
        target.signature.rect_hint.as_ref(),
    ) {
        score += 5;
    }
    score += recency_score(item.last_used_at);
    score += (item.use_count.min(10)) as i32;
    score
}

fn recency_score(last_used_at: i64) -> i32 {
    let age_ms = now_ms().saturating_sub(last_used_at);
    let day_ms = 24 * 60 * 60 * 1000;
    if age_ms <= day_ms {
        20
    } else if age_ms <= 7 * day_ms {
        12
    } else if age_ms <= 30 * day_ms {
        6
    } else {
        0
    }
}

fn rect_close(a: Option<&RectHint>, b: Option<&RectHint>) -> bool {
    let (Some(a), Some(b)) = (a, b) else {
        return false;
    };
    let ax = (a.left + a.right) / 2;
    let ay = (a.top + a.bottom) / 2;
    let bx = (b.left + b.right) / 2;
    let by = (b.top + b.bottom) / 2;
    (ax - bx).abs() <= 80 && (ay - by).abs() <= 80
}

fn detect_current_focused_editable() -> Result<InputTarget, String> {
    let foreground = unsafe { GetForegroundWindow() };
    if foreground == 0 {
        return take_sticky_focus_target().ok_or_else(|| "当前没有前台窗口".to_string());
    }
    if unsafe { IsWindowVisible(foreground) } == 0 {
        return take_sticky_focus_target().ok_or_else(|| "当前前台窗口不可见".to_string());
    }
    if is_own_window(foreground) {
        // 自身窗口被点亮，回退到最近一次外部可编辑焦点
        return take_sticky_focus_target()
            .ok_or_else(|| "当前焦点位于语音助手自身窗口".to_string());
    }

    let mut foreground_pid = 0u32;
    let foreground_thread = unsafe { GetWindowThreadProcessId(foreground, &mut foreground_pid) };
    let mut info = GUITHREADINFO {
        cbSize: std::mem::size_of::<GUITHREADINFO>() as u32,
        ..unsafe { std::mem::zeroed() }
    };

    let active_focus = unsafe {
        if foreground_thread != 0
            && GetGUIThreadInfo(foreground_thread, &mut info) != 0
            && info.hwndFocus != 0
        {
            info.hwndFocus
        } else if GetGUIThreadInfo(0, &mut info) != 0 && info.hwndFocus != 0 {
            info.hwndFocus
        } else {
            foreground
        }
    };

    let top = unsafe {
        let root = GetAncestor(active_focus, GA_ROOT);
        if root != 0 {
            root
        } else {
            foreground
        }
    };

    if let Some(focus) = resolve_bindable_focus_hwnd(top, active_focus) {
        if let Ok(target) = build_target(top, focus) {
            return Ok(target);
        }
    }

    // 当前焦点不可绑定（如停留在 Ribbon / 字体下拉框） → 回退到 sticky
    if let Some(target) = take_sticky_focus_target() {
        append_log(&format!(
            "detect_focus_fallback_sticky target={} process={}",
            target.id, target.signature.process_name
        ));
        return Ok(target);
    }

    let class_name = get_window_class_name(active_focus).unwrap_or_else(|_| "unknown".to_string());
    Err(format!("当前控件不是可写入输入框: {class_name}"))
}

fn build_target(top_hwnd: HWND, focus_hwnd: HWND) -> Result<InputTarget, String> {
    if top_hwnd == 0 || focus_hwnd == 0 {
        return Err("窗口句柄无效".to_string());
    }

    let mut process_id = 0u32;
    let top_thread_id = unsafe { GetWindowThreadProcessId(top_hwnd, &mut process_id) };
    let mut focus_process_id = 0u32;
    let focus_thread_id = unsafe { GetWindowThreadProcessId(focus_hwnd, &mut focus_process_id) };
    if process_id == 0 {
        process_id = focus_process_id;
    }
    if process_id == std::process::id() {
        return Err("当前焦点位于语音助手自身进程".to_string());
    }

    let exe_path = get_process_path(process_id);
    let process_name = exe_path
        .as_deref()
        .and_then(|path| std::path::Path::new(path).file_name())
        .map(|name| name.to_string_lossy().to_string())
        .unwrap_or_else(|| format!("pid-{process_id}"));
    let top_title = non_empty(get_window_text(top_hwnd).unwrap_or_default());
    let top_class_name = non_empty(get_window_class_name(top_hwnd).unwrap_or_default());
    let control_class_name = non_empty(get_window_class_name(focus_hwnd).unwrap_or_default());
    let hwnd_rect_hint = get_rect_hint(focus_hwnd);
    let accessibility_hint =
        detect_accessibility_focus_hint(top_hwnd, focus_hwnd, hwnd_rect_hint.as_ref());
    let rect_hint = accessibility_hint
        .as_ref()
        .map(|hint| hint.rect_hint)
        .or(hwnd_rect_hint);
    let app_type = classify_app(&process_name, top_title.as_deref());
    let priority = match app_type.as_str() {
        "RIS" => 100,
        "HIS" => 90,
        "BrowserRIS" => 80,
        "ChromiumShell" => 70,
        "Office" => 60,
        "Chat" => 40,
        _ => 10,
    };

    let signature = TargetSignature {
        process_name: process_name.to_ascii_lowercase(),
        exe_path,
        top_title: top_title.clone(),
        top_class_name,
        control_class_name: control_class_name.clone(),
        automation_id: accessibility_hint
            .as_ref()
            .and_then(|hint| hint.automation_id.clone()),
        control_name: accessibility_hint
            .as_ref()
            .and_then(|hint| hint.control_name.clone()),
        control_type: accessibility_hint
            .as_ref()
            .and_then(|hint| hint.control_type.clone())
            .or_else(|| Some("Win32".to_string())),
        rect_hint,
    };
    let id = make_target_id(&signature);
    let display_name = make_display_name(
        &process_name,
        top_title.as_deref(),
        control_class_name.as_deref(),
    );

    Ok(InputTarget {
        id,
        display_name,
        runtime: RuntimeTarget {
            top_hwnd,
            focus_hwnd,
            process_id,
            thread_id: if focus_thread_id != 0 {
                focus_thread_id
            } else {
                top_thread_id
            },
        },
        signature,
        app_type,
        priority,
    })
}

fn is_runtime_target_alive(runtime: &RuntimeTarget, signature: &TargetSignature) -> bool {
    if runtime.top_hwnd == 0 || runtime.focus_hwnd == 0 {
        return false;
    }
    if unsafe { IsWindow(runtime.top_hwnd) } == 0 || unsafe { IsWindow(runtime.focus_hwnd) } == 0 {
        return false;
    }
    if unsafe { IsWindowVisible(runtime.top_hwnd) } == 0 {
        return false;
    }

    let mut pid = 0u32;
    unsafe {
        GetWindowThreadProcessId(runtime.top_hwnd, &mut pid);
    }
    if pid != 0 {
        let current_path = get_process_path(pid);
        let current_name = current_path
            .as_deref()
            .and_then(|path| std::path::Path::new(path).file_name())
            .map(|name| name.to_string_lossy().to_string().to_ascii_lowercase())
            .unwrap_or_default();
        if !current_name.is_empty() && current_name != signature.process_name {
            return false;
        }
    }

    if let Some(expected) = signature.control_class_name.as_deref() {
        if let Ok(actual) = get_window_class_name(runtime.focus_hwnd) {
            if !class_matches(&actual, &expected.to_ascii_lowercase()) {
                return false;
            }
        }
    }

    true
}

fn paste_to_target(target: &InputTarget, text: &str) -> Result<(), String> {
    if !is_runtime_target_alive(&target.runtime, &target.signature) {
        return Err("绑定目标已失效".to_string());
    }

    let config = load_config();
    if is_blocked_target(target, &config) {
        return Err("目标应用在黑名单中".to_string());
    }

    let previous_foreground = unsafe { GetForegroundWindow() };
    let clipboard = ClipboardSnapshot {
        text: read_clipboard_unicode().ok().flatten(),
    };

    focus_target(target)?;
    set_clipboard_unicode(text)?;
    thread::sleep(Duration::from_millis(CLIPBOARD_SETTLE_DELAY_MS));

    let control_class = target
        .signature
        .control_class_name
        .as_deref()
        .unwrap_or_default()
        .to_ascii_lowercase();
    let post_paste_delay_ms = if is_web_like_target(target) {
        WEB_POST_PASTE_DELAY_MS
    } else {
        POST_PASTE_DELAY_MS
    };
    if is_direct_paste_class(&control_class) {
        unsafe {
            if PostMessageW(target.runtime.focus_hwnd, WM_PASTE, 0, 0) == 0 {
                send_ctrl_v_scancode()?;
            }
        }
        thread::sleep(Duration::from_millis(post_paste_delay_ms));
    } else {
        send_ctrl_v_scancode()?;
        thread::sleep(Duration::from_millis(post_paste_delay_ms));
    }

    restore_clipboard(clipboard);

    if previous_foreground != 0 && previous_foreground != target.runtime.top_hwnd {
        unsafe {
            SetForegroundWindow(previous_foreground);
        }
    }

    Ok(())
}

fn focus_target(target: &InputTarget) -> Result<(), String> {
    let top = target.runtime.top_hwnd;
    let focus = target.runtime.focus_hwnd;
    let previous_foreground = unsafe { GetForegroundWindow() };
    if unsafe { IsIconic(top) } != 0 {
        unsafe {
            ShowWindow(top, SW_RESTORE);
        }
        thread::sleep(Duration::from_millis(80));
    }

    let current_thread = unsafe { GetCurrentThreadId() };
    let mut previous_foreground_pid = 0u32;
    let previous_foreground_thread = unsafe {
        if previous_foreground != 0 {
            GetWindowThreadProcessId(previous_foreground, &mut previous_foreground_pid)
        } else {
            0
        }
    };
    let target_thread = target.runtime.thread_id;
    let mut attached_threads = Vec::<u32>::new();

    unsafe {
        for thread_id in [previous_foreground_thread, target_thread] {
            if thread_id != 0
                && thread_id != current_thread
                && !attached_threads.contains(&thread_id)
                && AttachThreadInput(current_thread, thread_id, 1) != 0
            {
                attached_threads.push(thread_id);
            }
        }

        SetForegroundWindow(top);
        BringWindowToTop(top);

        if !is_web_like_target(target) {
            SetFocus(focus);
        } else {
            append_log(&format!(
                "focus_target_web_preserve_internal_focus target={} class={}",
                target.id,
                target
                    .signature
                    .control_class_name
                    .as_deref()
                    .unwrap_or_default()
            ));
        }

        for thread_id in attached_threads.into_iter().rev() {
            AttachThreadInput(current_thread, thread_id, 0);
        }
    }
    thread::sleep(Duration::from_millis(90));
    Ok(())
}

fn send_ctrl_v_scancode() -> Result<(), String> {
    send_input(&[keyboard_input(SCAN_LEFT_CONTROL, false)])?;
    thread::sleep(Duration::from_millis(KEYSTROKE_DELAY_MS));
    send_input(&[keyboard_input(SCAN_V, false), keyboard_input(SCAN_V, true)])?;
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
            std::io::Error::last_os_error()
        ));
    }
    Ok(())
}

fn read_clipboard_unicode() -> Result<Option<String>, String> {
    unsafe {
        if IsClipboardFormatAvailable(CF_UNICODETEXT.into()) == 0 {
            return Ok(None);
        }
        open_clipboard_with_retry()?;
        let _guard = ClipboardGuard;
        let handle = GetClipboardData(CF_UNICODETEXT.into());
        if handle == 0 {
            return Ok(None);
        }
        let locked = GlobalLock(handle as _) as *const u16;
        if locked.is_null() {
            return Ok(None);
        }

        let mut len = 0usize;
        while *locked.add(len) != 0 {
            len += 1;
        }
        let slice = std::slice::from_raw_parts(locked, len);
        let text = String::from_utf16_lossy(slice);
        GlobalUnlock(handle as _);
        Ok(Some(text))
    }
}

fn set_clipboard_unicode(text: &str) -> Result<(), String> {
    let utf16: Vec<u16> = text.encode_utf16().chain(std::iter::once(0)).collect();
    let bytes = utf16.len() * std::mem::size_of::<u16>();

    unsafe {
        open_clipboard_with_retry()?;
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

fn restore_clipboard(snapshot: ClipboardSnapshot) {
    if let Some(text) = snapshot.text {
        let _ = set_clipboard_unicode(&text);
        return;
    }

    unsafe {
        if open_clipboard_with_retry().is_ok() {
            let _guard = ClipboardGuard;
            EmptyClipboard();
        }
    }
}

unsafe fn open_clipboard_with_retry() -> Result<(), String> {
    for _ in 0..CLIPBOARD_OPEN_RETRIES {
        if OpenClipboard(0) != 0 {
            return Ok(());
        }
        thread::sleep(Duration::from_millis(CLIPBOARD_OPEN_RETRY_DELAY_MS));
    }
    Err(last_os_error_message())
}

#[derive(Debug, Clone, Copy)]
enum OverlayKind {
    Bound,
    Success,
    Invalid,
}

fn flash_overlay_for_target(
    target: &InputTarget,
    label: &str,
    kind: OverlayKind,
    duration_ms: u64,
) {
    let rect = target_overlay_rect(target).unwrap_or(RectHint {
        left: 20,
        top: 20,
        right: 460,
        bottom: 120,
    });
    let primary = label.to_string();
    let secondary = target.display_name.clone();
    let default_total = match kind {
        OverlayKind::Bound => OVERLAY_DEFAULT_BOUND_MS,
        OverlayKind::Success => OVERLAY_DEFAULT_SUCCESS_MS,
        OverlayKind::Invalid => OVERLAY_DEFAULT_INVALID_MS,
    };
    let total = if duration_ms == 0 {
        default_total
    } else {
        duration_ms.max(default_total)
    };

    thread::spawn(move || animate_overlay(rect, primary, secondary, kind, total));
}

fn target_overlay_rect(target: &InputTarget) -> Option<RectHint> {
    let hwnd_rect = get_rect_hint(target.runtime.focus_hwnd);
    let signature_rect = target.signature.rect_hint;
    let is_accessibility_rect = target
        .signature
        .control_type
        .as_deref()
        .map(|value| value.starts_with("UIAutomation"))
        .unwrap_or(false);

    if is_accessibility_rect {
        return signature_rect.or(hwnd_rect);
    }

    match (signature_rect, hwnd_rect) {
        (Some(signature), Some(hwnd))
            if rect_overlaps(&signature, &hwnd)
                && rect_substantially_smaller(&signature, &hwnd) =>
        {
            Some(signature)
        }
        (_, Some(hwnd)) => Some(hwnd),
        (Some(signature), None) => Some(signature),
        (None, None) => None,
    }
}

#[derive(Debug, Clone, Copy)]
struct OverlayTheme {
    accent: u32,
    accent_soft: u32,
    label_bg: u32,
    label_text: u32,
    dot_color: u32,
}

fn overlay_theme(kind: OverlayKind) -> OverlayTheme {
    match kind {
        // Bound: 翡翠绿
        OverlayKind::Bound => OverlayTheme {
            accent: rgb(16, 185, 129),
            accent_soft: rgb(110, 231, 183),
            label_bg: rgb(6, 95, 70),
            label_text: rgb(236, 253, 245),
            dot_color: rgb(110, 231, 183),
        },
        // Success: 天蓝
        OverlayKind::Success => OverlayTheme {
            accent: rgb(59, 130, 246),
            accent_soft: rgb(147, 197, 253),
            label_bg: rgb(30, 64, 175),
            label_text: rgb(239, 246, 255),
            dot_color: rgb(147, 197, 253),
        },
        // Invalid: 玫红
        OverlayKind::Invalid => OverlayTheme {
            accent: rgb(239, 68, 68),
            accent_soft: rgb(252, 165, 165),
            label_bg: rgb(153, 27, 27),
            label_text: rgb(254, 242, 242),
            dot_color: rgb(252, 165, 165),
        },
    }
}

fn animate_overlay(
    target_rect: RectHint,
    primary: String,
    secondary: String,
    kind: OverlayKind,
    total_ms: u64,
) {
    unsafe {
        ensure_overlay_class();

        let theme = overlay_theme(kind);
        let label_text = if secondary.trim().is_empty() {
            primary.clone()
        } else {
            format!("{}  ·  {}", primary, secondary)
        };

        let font = create_overlay_font();
        if font == 0 {
            // 字体创建失败时降级为不绘制
            return;
        }

        let (text_w, text_h) = measure_overlay_text(&label_text, font);
        let label_inner_w = OVERLAY_LABEL_DOT_DIAMETER + OVERLAY_LABEL_DOT_GAP + text_w.max(40);
        let label_w = label_inner_w + OVERLAY_LABEL_PADDING_X * 2;
        let label_h = text_h.max(OVERLAY_LABEL_FONT_HEIGHT) + OVERLAY_LABEL_PADDING_Y * 2;

        let vscreen_left = GetSystemMetrics(SM_XVIRTUALSCREEN);
        let vscreen_top = GetSystemMetrics(SM_YVIRTUALSCREEN);
        let vscreen_width = GetSystemMetrics(SM_CXVIRTUALSCREEN).max(640);
        let vscreen_height = GetSystemMetrics(SM_CYVIRTUALSCREEN).max(480);
        let vscreen_right = vscreen_left + vscreen_width;
        let vscreen_bottom = vscreen_top + vscreen_height;

        let target_left = target_rect.left.clamp(vscreen_left, vscreen_right - 1);
        let target_top = target_rect.top.clamp(vscreen_top, vscreen_bottom - 1);
        let target_right = target_rect.right.clamp(target_left + 24, vscreen_right);
        let target_bottom = target_rect.bottom.clamp(target_top + 24, vscreen_bottom);

        // 标签位置：优先放在目标矩形下方居中，溢出则放上方
        let mut label_x = (target_left + target_right) / 2 - label_w / 2;
        label_x = label_x.clamp(vscreen_left + 6, vscreen_right - label_w - 6);
        let mut label_y = target_bottom + 12;
        if label_y + label_h > vscreen_bottom - 6 {
            label_y = target_top - 12 - label_h;
        }
        if label_y < vscreen_top + 6 {
            // 上下都放不下：贴在目标矩形顶部右侧
            label_y = target_top.max(vscreen_top + 6);
        }

        let bracket_margin = 6;
        let bracket_left = (target_left - bracket_margin).max(vscreen_left);
        let bracket_top = (target_top - bracket_margin).max(vscreen_top);
        let bracket_right = (target_right + bracket_margin).min(vscreen_right);
        let bracket_bottom = (target_bottom + bracket_margin).min(vscreen_bottom);

        let win_left = bracket_left.min(label_x);
        let win_top = bracket_top.min(label_y);
        let win_right = bracket_right.max(label_x + label_w);
        let win_bottom = bracket_bottom.max(label_y + label_h);
        let win_w = (win_right - win_left).max(label_w + 12);
        let win_h = (win_bottom - win_top).max(label_h + 12);

        let class_name = wide_null(OVERLAY_CLASS_NAME);
        let hwnd = CreateWindowExW(
            WS_EX_LAYERED | WS_EX_TRANSPARENT | WS_EX_TOPMOST | WS_EX_TOOLWINDOW | WS_EX_NOACTIVATE,
            class_name.as_ptr(),
            std::ptr::null(),
            WS_POPUP,
            win_left,
            win_top,
            win_w,
            win_h,
            0,
            0,
            GetModuleHandleW(std::ptr::null()),
            std::ptr::null(),
        );

        if hwnd == 0 {
            DeleteObject(font as isize);
            return;
        }

        // 初始 alpha = 0
        SetLayeredWindowAttributes(hwnd, OVERLAY_COLOR_KEY, 0, LWA_COLORKEY | LWA_ALPHA);
        ShowWindow(hwnd, SW_SHOWNOACTIVATE);

        let bracket_local = RECT {
            left: bracket_left - win_left,
            top: bracket_top - win_top,
            right: bracket_right - win_left,
            bottom: bracket_bottom - win_top,
        };
        let label_local = RECT {
            left: label_x - win_left,
            top: label_y - win_top,
            right: label_x + label_w - win_left,
            bottom: label_y + label_h - win_top,
        };
        paint_overlay_contents(
            hwnd,
            win_w,
            win_h,
            &bracket_local,
            &label_local,
            &label_text,
            font,
            theme,
        );
        UpdateWindow(hwnd);

        // 淡入
        let fade_in_steps = 14u32;
        let fade_in_delay = (OVERLAY_FADE_IN_MS / fade_in_steps as u64).max(8);
        for i in 1..=fade_in_steps {
            let alpha = ((i as u32 * 255) / fade_in_steps) as u8;
            SetLayeredWindowAttributes(hwnd, OVERLAY_COLOR_KEY, alpha, LWA_COLORKEY | LWA_ALPHA);
            thread::sleep(Duration::from_millis(fade_in_delay));
        }

        let hold = total_ms
            .saturating_sub(OVERLAY_FADE_IN_MS + OVERLAY_FADE_OUT_MS)
            .max(OVERLAY_MIN_HOLD_MS);
        thread::sleep(Duration::from_millis(hold));

        // 淡出
        let fade_out_steps = 16u32;
        let fade_out_delay = (OVERLAY_FADE_OUT_MS / fade_out_steps as u64).max(8);
        for i in 1..=fade_out_steps {
            let alpha = (255 - (i as u32 * 255) / fade_out_steps) as u8;
            SetLayeredWindowAttributes(hwnd, OVERLAY_COLOR_KEY, alpha, LWA_COLORKEY | LWA_ALPHA);
            thread::sleep(Duration::from_millis(fade_out_delay));
        }

        DestroyWindow(hwnd);
        DeleteObject(font as isize);
    }
}

unsafe fn create_overlay_font() -> isize {
    let face = wide_null(OVERLAY_LABEL_FACE_PRIMARY);
    let font = CreateFontW(
        OVERLAY_LABEL_FONT_HEIGHT,
        0,
        0,
        0,
        FW_SEMIBOLD as i32,
        0,
        0,
        0,
        ANSI_CHARSET as u32,
        OUT_TT_PRECIS as u32,
        CLIP_DEFAULT_PRECIS as u32,
        ANTIALIASED_QUALITY as u32,
        (DEFAULT_PITCH | FF_DONTCARE) as u32,
        face.as_ptr(),
    );
    if font != 0 {
        return font as isize;
    }
    let fallback = wide_null(OVERLAY_LABEL_FACE_FALLBACK);
    let font2 = CreateFontW(
        OVERLAY_LABEL_FONT_HEIGHT,
        0,
        0,
        0,
        FW_NORMAL as i32,
        0,
        0,
        0,
        ANSI_CHARSET as u32,
        OUT_TT_PRECIS as u32,
        CLIP_DEFAULT_PRECIS as u32,
        ANTIALIASED_QUALITY as u32,
        (DEFAULT_PITCH | FF_DONTCARE) as u32,
        fallback.as_ptr(),
    );
    font2 as isize
}

unsafe fn measure_overlay_text(text: &str, font: isize) -> (i32, i32) {
    let utf16: Vec<u16> = text.encode_utf16().collect();
    if utf16.is_empty() {
        return (12, OVERLAY_LABEL_FONT_HEIGHT);
    }
    let hdc = GetDC(0);
    if hdc == 0 {
        return ((text.chars().count() as i32) * 9, OVERLAY_LABEL_FONT_HEIGHT);
    }
    let old_font = SelectObject(hdc, font);
    let mut size = SIZE { cx: 0, cy: 0 };
    let ok = GetTextExtentPoint32W(hdc, utf16.as_ptr(), utf16.len() as i32, &mut size);
    SelectObject(hdc, old_font);
    ReleaseDC(0, hdc);
    if ok == 0 || size.cx <= 0 {
        return ((text.chars().count() as i32) * 9, OVERLAY_LABEL_FONT_HEIGHT);
    }
    (size.cx, size.cy.max(OVERLAY_LABEL_FONT_HEIGHT))
}

unsafe fn paint_overlay_contents(
    hwnd: HWND,
    width: i32,
    height: i32,
    bracket_rect: &RECT,
    label_rect: &RECT,
    label_text: &str,
    font: isize,
    theme: OverlayTheme,
) {
    let hdc = GetDC(hwnd);
    if hdc == 0 {
        return;
    }

    // 透明背景
    let bg_brush = CreateSolidBrush(OVERLAY_COLOR_KEY);
    let fill = RECT {
        left: 0,
        top: 0,
        right: width,
        bottom: height,
    };
    FillRect(hdc, &fill, bg_brush);
    DeleteObject(bg_brush as isize);

    SetBkMode(hdc, TRANSPARENT as i32);

    draw_corner_brackets(hdc, bracket_rect, theme.accent, theme.accent_soft);
    draw_pill_label(hdc, label_rect, label_text, font, theme);

    ReleaseDC(hwnd, hdc);
}

unsafe fn draw_corner_brackets(hdc: isize, rect: &RECT, accent: u32, accent_soft: u32) {
    let len = OVERLAY_BRACKET_LENGTH;
    let thickness = OVERLAY_BRACKET_THICKNESS;

    let brush = CreateSolidBrush(accent);
    let soft_brush = CreateSolidBrush(accent_soft);

    // 四角：每角两条短矩形（水平 + 垂直）
    let corners: [(i32, i32, i32, i32, i32, i32, i32, i32); 4] = [
        // top-left: horiz + vert
        (
            rect.left,
            rect.top,
            rect.left + len,
            rect.top + thickness,
            rect.left,
            rect.top,
            rect.left + thickness,
            rect.top + len,
        ),
        // top-right
        (
            rect.right - len,
            rect.top,
            rect.right,
            rect.top + thickness,
            rect.right - thickness,
            rect.top,
            rect.right,
            rect.top + len,
        ),
        // bottom-left
        (
            rect.left,
            rect.bottom - thickness,
            rect.left + len,
            rect.bottom,
            rect.left,
            rect.bottom - len,
            rect.left + thickness,
            rect.bottom,
        ),
        // bottom-right
        (
            rect.right - len,
            rect.bottom - thickness,
            rect.right,
            rect.bottom,
            rect.right - thickness,
            rect.bottom - len,
            rect.right,
            rect.bottom,
        ),
    ];

    for (h_l, h_t, h_r, h_b, v_l, v_t, v_r, v_b) in corners {
        let h_rect = RECT {
            left: h_l,
            top: h_t,
            right: h_r,
            bottom: h_b,
        };
        let v_rect = RECT {
            left: v_l,
            top: v_t,
            right: v_r,
            bottom: v_b,
        };
        FillRect(hdc, &h_rect, brush);
        FillRect(hdc, &v_rect, brush);

        // 端点高亮：在 L 形拐角处叠加一个 2px 软色方块
        let glow = RECT {
            left: h_l.min(v_l),
            top: h_t.min(v_t),
            right: h_l.min(v_l) + thickness,
            bottom: h_t.min(v_t) + thickness,
        };
        FillRect(hdc, &glow, soft_brush);
    }

    DeleteObject(brush as isize);
    DeleteObject(soft_brush as isize);
}

unsafe fn draw_pill_label(hdc: isize, rect: &RECT, text: &str, font: isize, theme: OverlayTheme) {
    let radius = OVERLAY_LABEL_RADIUS.min((rect.bottom - rect.top) / 2 + 2);

    // 阴影层（在右下偏移 2px，颜色稍暗）
    let shadow_color = scale_color(theme.label_bg, 0.55);
    let shadow_brush = CreateSolidBrush(shadow_color);
    let shadow_pen = CreatePen(PS_SOLID, 1, shadow_color);
    let old_pen = SelectObject(hdc, shadow_pen as isize);
    let old_brush = SelectObject(hdc, shadow_brush as isize);
    RoundRect(
        hdc,
        rect.left + 2,
        rect.top + 2,
        rect.right + 2,
        rect.bottom + 2,
        radius,
        radius,
    );
    SelectObject(hdc, old_pen);
    SelectObject(hdc, old_brush);
    DeleteObject(shadow_pen as isize);
    DeleteObject(shadow_brush as isize);

    // 主体胶囊
    let bg_brush = CreateSolidBrush(theme.label_bg);
    let bg_pen = CreatePen(PS_SOLID, 1, theme.accent);
    let old_pen = SelectObject(hdc, bg_pen as isize);
    let old_brush = SelectObject(hdc, bg_brush as isize);
    RoundRect(
        hdc,
        rect.left,
        rect.top,
        rect.right,
        rect.bottom,
        radius,
        radius,
    );
    SelectObject(hdc, old_pen);
    SelectObject(hdc, old_brush);
    DeleteObject(bg_pen as isize);
    DeleteObject(bg_brush as isize);

    // 状态点
    let dot_brush = CreateSolidBrush(theme.dot_color);
    let dot_pen = CreatePen(PS_SOLID, 1, theme.dot_color);
    let old_pen = SelectObject(hdc, dot_pen as isize);
    let old_brush = SelectObject(hdc, dot_brush as isize);
    let dot_d = OVERLAY_LABEL_DOT_DIAMETER;
    let dot_left = rect.left + OVERLAY_LABEL_PADDING_X;
    let dot_top = (rect.top + rect.bottom) / 2 - dot_d / 2;
    RoundRect(
        hdc,
        dot_left,
        dot_top,
        dot_left + dot_d,
        dot_top + dot_d,
        dot_d,
        dot_d,
    );
    SelectObject(hdc, old_pen);
    SelectObject(hdc, old_brush);
    DeleteObject(dot_pen as isize);
    DeleteObject(dot_brush as isize);

    // 文本
    let old_font = SelectObject(hdc, font);
    SetTextColor(hdc, theme.label_text);
    let text_x = dot_left + dot_d + OVERLAY_LABEL_DOT_GAP;
    let text_h = OVERLAY_LABEL_FONT_HEIGHT;
    let text_y = (rect.top + rect.bottom) / 2 - text_h / 2;
    let utf16: Vec<u16> = text.encode_utf16().take(200).collect();
    TextOutW(hdc, text_x, text_y, utf16.as_ptr(), utf16.len() as i32);
    SelectObject(hdc, old_font);
}

fn scale_color(color: u32, factor: f32) -> u32 {
    let r = (color & 0xFF) as f32;
    let g = ((color >> 8) & 0xFF) as f32;
    let b = ((color >> 16) & 0xFF) as f32;
    let r = (r * factor).clamp(0.0, 255.0) as u32;
    let g = (g * factor).clamp(0.0, 255.0) as u32;
    let b = (b * factor).clamp(0.0, 255.0) as u32;
    r | (g << 8) | (b << 16)
}

unsafe fn ensure_overlay_class() {
    OVERLAY_CLASS.call_once(|| {
        let class_name = wide_null(OVERLAY_CLASS_NAME);
        let window_class = WNDCLASSW {
            style: 0,
            lpfnWndProc: Some(overlay_wnd_proc),
            cbClsExtra: 0,
            cbWndExtra: 0,
            hInstance: GetModuleHandleW(std::ptr::null()),
            hIcon: 0,
            hCursor: 0,
            hbrBackground: 0,
            lpszMenuName: std::ptr::null(),
            lpszClassName: class_name.as_ptr(),
        };
        RegisterClassW(&window_class);
    });
}

unsafe extern "system" fn overlay_wnd_proc(
    hwnd: HWND,
    msg: u32,
    wparam: WPARAM,
    lparam: LPARAM,
) -> LRESULT {
    if msg == WM_NCHITTEST {
        return HTTRANSPARENT as LRESULT;
    }
    DefWindowProcW(hwnd, msg, wparam, lparam)
}

fn wide_null(value: &str) -> Vec<u16> {
    value.encode_utf16().chain(std::iter::once(0)).collect()
}

fn rgb(r: u8, g: u8, b: u8) -> u32 {
    u32::from(r) | (u32::from(g) << 8) | (u32::from(b) << 16)
}

fn history_item_to_view(item: &BoundTargetHistoryItem) -> BridgeTargetView {
    let status = if resolve_history_item(item).is_ok() {
        "valid"
    } else {
        "invalid"
    };
    BridgeTargetView {
        target_id: item.id.clone(),
        display_name: item.display_name.clone(),
        status: status.to_string(),
        process_name: Some(item.signature.process_name.clone()),
        top_title: item.signature.top_title.clone(),
        control_class_name: item.signature.control_class_name.clone(),
        app_type: Some(item.app_type.clone()),
        last_used_at: Some(item.last_used_at),
        use_count: Some(item.use_count),
    }
}

fn target_to_candidate_view(target: &InputTarget) -> BridgeTargetView {
    BridgeTargetView {
        target_id: target.id.clone(),
        display_name: target.display_name.clone(),
        status: "candidate".to_string(),
        process_name: Some(target.signature.process_name.clone()),
        top_title: target.signature.top_title.clone(),
        control_class_name: target.signature.control_class_name.clone(),
        app_type: Some(target.app_type.clone()),
        last_used_at: None,
        use_count: None,
    }
}

fn upsert_locked_target(history: &mut HistoryFile, target: InputTarget) {
    let now = now_ms();
    let existing = history
        .targets
        .iter()
        .find(|item| item.id == target.id)
        .cloned();
    let use_count = existing.as_ref().map(|item| item.use_count).unwrap_or(0);
    let last_bound_at = existing
        .as_ref()
        .map(|item| item.last_bound_at)
        .unwrap_or(now);

    history.targets.retain(|item| item.id != target.id);
    history.targets.insert(
        0,
        BoundTargetHistoryItem {
            id: target.id.clone(),
            display_name: target.display_name.clone(),
            signature: target.signature.clone(),
            last_runtime: Some(target.runtime.clone()),
            last_bound_at,
            last_used_at: now,
            use_count,
            app_type: target.app_type.clone(),
            priority: target.priority,
        },
    );
    history.locked_target_id = Some(target.id);
    history
        .targets
        .sort_by_key(|item| Reverse((item.priority, item.last_used_at, item.use_count)));
    history.targets.truncate(MAX_HISTORY_ITEMS);
}

fn update_history_runtime(history: &mut HistoryFile, target: &InputTarget, mark_used: bool) {
    let now = now_ms();
    let mut found = false;
    for item in &mut history.targets {
        if item.id == target.id {
            item.display_name = target.display_name.clone();
            item.signature = target.signature.clone();
            item.last_runtime = Some(target.runtime.clone());
            if mark_used {
                item.last_used_at = now;
                item.use_count = item.use_count.saturating_add(1);
            }
            found = true;
            break;
        }
    }
    if !found {
        upsert_locked_target(history, target.clone());
    }
    history
        .targets
        .sort_by_key(|item| Reverse((item.priority, item.last_used_at, item.use_count)));
    history.targets.truncate(MAX_HISTORY_ITEMS);
}

fn load_history() -> HistoryFile {
    let path = history_path();
    let Ok(raw) = fs::read_to_string(path) else {
        return HistoryFile::default();
    };
    serde_json::from_str::<HistoryFile>(&raw).unwrap_or_default()
}

fn save_history(history: &HistoryFile) -> Result<(), String> {
    let path = history_path();
    if let Some(parent) = path.parent() {
        fs::create_dir_all(parent).map_err(|err| err.to_string())?;
    }
    let raw = serde_json::to_string_pretty(history).map_err(|err| err.to_string())?;
    fs::write(path, raw).map_err(|err| err.to_string())
}

fn load_config() -> BridgeConfig {
    let path = config_path();
    let Ok(raw) = fs::read_to_string(path) else {
        return BridgeConfig::default();
    };
    let mut config = serde_json::from_str::<BridgeConfig>(&raw).unwrap_or_default();
    config.allowed_apps = normalize_app_list(config.allowed_apps);
    config.blocked_apps = normalize_app_list(config.blocked_apps);
    config
}

fn normalize_app_list(items: Vec<String>) -> Vec<String> {
    items
        .into_iter()
        .map(|item| item.trim().to_ascii_lowercase())
        .filter(|item| !item.is_empty())
        .collect()
}

fn app_root_dir() -> PathBuf {
    if let Some(appdata) = std::env::var_os("APPDATA") {
        return PathBuf::from(appdata)
            .join("jusha")
            .join("VoiceInputBridge");
    }
    if let Some(local_appdata) = std::env::var_os("LOCALAPPDATA") {
        return PathBuf::from(local_appdata)
            .join("jusha")
            .join("VoiceInputBridge");
    }
    std::env::temp_dir().join("jusha").join("VoiceInputBridge")
}

fn history_path() -> PathBuf {
    app_root_dir().join("target-history.json")
}

fn config_path() -> PathBuf {
    app_root_dir().join("config.json")
}

fn log_path() -> PathBuf {
    app_root_dir().join("logs").join("bridge.log")
}

fn append_log(message: &str) {
    let path = log_path();
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }
    let timestamp = (now_ms() as f64 / 1000.0).to_string();
    if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) {
        let _ = writeln!(file, "[{timestamp} pid={}] {message}", std::process::id());
    }
}

fn make_target_id(signature: &TargetSignature) -> String {
    let mut hasher = DefaultHasher::new();
    signature.process_name.hash(&mut hasher);
    signature.exe_path.hash(&mut hasher);
    signature.top_title.hash(&mut hasher);
    signature.top_class_name.hash(&mut hasher);
    signature.control_class_name.hash(&mut hasher);
    format!("target-{:016x}", hasher.finish())
}

fn make_display_name(
    process_name: &str,
    title: Option<&str>,
    control_class: Option<&str>,
) -> String {
    let app = if process_name.eq_ignore_ascii_case("chrome.exe")
        || process_name.eq_ignore_ascii_case("msedge.exe")
        || process_name.eq_ignore_ascii_case("firefox.exe")
        || process_name.eq_ignore_ascii_case("iexplore.exe")
    {
        "浏览器 RIS".to_string()
    } else if process_name.eq_ignore_ascii_case("winword.exe") {
        "Word".to_string()
    } else if process_name.eq_ignore_ascii_case("excel.exe") {
        "Excel".to_string()
    } else if process_name.eq_ignore_ascii_case("powerpnt.exe") {
        "PowerPoint".to_string()
    } else if is_wechat_process_name(process_name) {
        "微信".to_string()
    } else if process_name.eq_ignore_ascii_case("dingtalk.exe") {
        "钉钉".to_string()
    } else if process_name.to_ascii_lowercase().contains("ris") {
        "RIS".to_string()
    } else if process_name.to_ascii_lowercase().contains("his") {
        "HIS".to_string()
    } else {
        process_name.to_string()
    };
    let title = title
        .filter(|value| !value.trim().is_empty())
        .unwrap_or("输入窗口");
    let control = control_class
        .filter(|value| !value.trim().is_empty())
        .unwrap_or("输入框");
    format!("{app} - {title} - {control}")
}

fn classify_app(process_name: &str, title: Option<&str>) -> String {
    let process = process_name.to_ascii_lowercase();
    let title = title.unwrap_or_default().to_ascii_lowercase();
    if process.contains("ris") || title.contains("ris") || title.contains("报告") {
        "RIS".to_string()
    } else if process.contains("his") || title.contains("his") || title.contains("病历") {
        "HIS".to_string()
    } else if ["chrome.exe", "msedge.exe", "firefox.exe", "iexplore.exe"]
        .contains(&process.as_str())
    {
        "BrowserRIS".to_string()
    } else if [
        "dingtalk.exe",
        "wework.exe",
        "wxwork.exe",
        "feishu.exe",
        "lark.exe",
    ]
    .contains(&process.as_str())
    {
        "ChromiumShell".to_string()
    } else if [
        "winword.exe",
        "excel.exe",
        "powerpnt.exe",
        "wps.exe",
        "et.exe",
        "wpp.exe",
    ]
    .contains(&process.as_str())
    {
        "Office".to_string()
    } else if is_wechat_process_name(&process) {
        "Chat".to_string()
    } else {
        "Other".to_string()
    }
}

fn is_allowed_fallback_target(target: &InputTarget, config: &BridgeConfig) -> bool {
    if is_blocked_target(target, config) {
        return false;
    }
    if !config.require_whitelist_for_fallback {
        return true;
    }
    config
        .allowed_apps
        .iter()
        .any(|app| same_ci(app, &target.signature.process_name))
}

fn is_blocked_target(target: &InputTarget, config: &BridgeConfig) -> bool {
    config
        .blocked_apps
        .iter()
        .any(|app| same_ci(app, &target.signature.process_name))
}

fn is_own_window(hwnd: HWND) -> bool {
    let mut pid = 0u32;
    unsafe {
        GetWindowThreadProcessId(hwnd, &mut pid);
    }
    pid == std::process::id()
}

fn resolve_bindable_focus_hwnd(top_hwnd: HWND, focus_hwnd: HWND) -> Option<HWND> {
    let top_class = get_window_class_name(top_hwnd).unwrap_or_default();

    // ---- Office (Excel / PPT / Word / WPS) ----
    // Office 的文本插入点经常不暴露为独立 HWND；Ribbon 里的字体下拉框反而是 Edit。
    // 只要顶层是 Office，就优先绑定文档/幻灯片画布，找不到 canvas 时绑定顶层窗口作为 Ctrl+V 宿主。
    if is_office_top_class(&top_class) {
        if focus_hwnd != 0 {
            let focus_class = get_window_class_name(focus_hwnd).unwrap_or_default();
            if is_office_canvas_class(&focus_class) && !is_focus_in_ribbon_or_toolbar(focus_hwnd) {
                return Some(focus_hwnd);
            }
        }
        if let Some(canvas) = find_office_canvas_hwnd(top_hwnd) {
            if focus_hwnd != 0
                && is_descendant_of(focus_hwnd, canvas)
                && !is_focus_in_ribbon_or_toolbar(focus_hwnd)
            {
                return Some(focus_hwnd);
            }
            return Some(canvas);
        }
        return Some(top_hwnd);
    }

    // ---- Chromium / CEF / 微信：直接使用渲染宿主或可粘贴宿主 ----
    if focus_hwnd != 0 {
        let focus_class = get_window_class_name(focus_hwnd).unwrap_or_default();
        if is_paste_host_target_class(&focus_class) && !is_focus_in_ribbon_or_toolbar(focus_hwnd) {
            return Some(focus_hwnd);
        }
    }
    if let Some(host) = find_paste_host_hwnd(top_hwnd) {
        return Some(host);
    }
    if is_paste_host_target_class(&top_class) {
        return Some(top_hwnd);
    }

    // ---- 默认 Win32 解析 ----
    if focus_hwnd != 0 {
        let focus_class = get_window_class_name(focus_hwnd).unwrap_or_default();
        // Ribbon/字体下拉框等编辑控件应被跳过
        if is_bindable_window(focus_hwnd) && !is_focus_in_ribbon_or_toolbar(focus_hwnd) {
            return Some(focus_hwnd);
        }
        if let Some(child) = find_first_editable_child(focus_hwnd) {
            return Some(child);
        }
        if is_paste_host_window(focus_hwnd) {
            return Some(focus_hwnd);
        }
        // 回退：用类名只判定 paste host
        if is_paste_host_target_class(&focus_class) {
            return Some(focus_hwnd);
        }
    }

    if top_hwnd != 0 && top_hwnd != focus_hwnd {
        if let Some(child) = find_first_editable_child(top_hwnd) {
            return Some(child);
        }
        if is_paste_host_window(top_hwnd) {
            return Some(top_hwnd);
        }
    }

    None
}

fn is_office_canvas_class(class_name: &str) -> bool {
    office_canvas_class_priority(class_name) > 0
}

fn is_office_top_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower == "xlmain"
        || lower == "pptframeclass"
        || lower == "opusapp"
        || lower.starts_with("kingsoft")
        || lower.starts_with("wpsoffice")
        || lower.contains("etmain")
        || lower.contains("wpsapp")
        || lower.contains("wppapp")
}

fn office_canvas_class_priority(class_name: &str) -> i32 {
    let lower = class_name.to_ascii_lowercase();
    if lower == "excel7" {
        110
    } else if lower == "_wwg" || lower == "_wwb" || lower == "_wwf" {
        105
    } else if lower == "paneclassdc" || lower.contains("paneclassdc") {
        100
    } else if lower.contains("screenclass") {
        90
    } else if lower.contains("ppt") && lower.contains("view") {
        85
    } else if lower.contains("slide") && lower.contains("view") {
        80
    } else if lower == "mdiclient" || lower == "mdiclass" {
        70
    } else if lower.starts_with("etmain")
        || lower.starts_with("kspread")
        || lower.starts_with("kingsoft")
        || lower.contains("officeart")
    {
        60
    } else {
        0
    }
}

fn is_ribbon_or_toolbar_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("netuihwnd")
        || lower.contains("netui")
        || lower.contains("msocommandbar")
        || lower.contains("msoworkpane")
        || lower.contains("msodock")
        || lower.contains("mso") && lower.contains("command")
        || lower.contains("ribbon")
        || lower.contains("nuipane")
        || lower.contains("commandbars")
        || lower.starts_with("toolbarwindow")
        || lower == "comboboxex32"
        || lower == "comboboxex"
}

fn is_focus_in_ribbon_or_toolbar(hwnd: HWND) -> bool {
    let mut current = hwnd;
    for _ in 0..16 {
        if current == 0 {
            break;
        }
        if let Ok(class_name) = get_window_class_name(current) {
            if is_ribbon_or_toolbar_class(&class_name) {
                return true;
            }
        }
        let parent = unsafe { GetParent(current) };
        if parent == 0 || parent == current {
            break;
        }
        current = parent;
    }
    false
}

fn is_chromium_render_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("chrome_renderwidgethosthwnd")
        || lower.contains("intermediate d3d window")
        || lower.contains("legacyrendertarget")
        || lower.contains("cef-osr-widget")
        || lower.contains("cefbrowserwindow")
        || lower.contains("cefwebview")
        || lower.contains("cefwindow")
        || lower.contains("cefclient")
}

fn is_wechat_process_name(process_name: &str) -> bool {
    let lower = process_name.to_ascii_lowercase();
    lower == "wechat.exe" || lower == "weixin.exe" || lower == "wxwork.exe" || lower == "wework.exe"
}

fn is_wechat_window_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("wechat")
        || lower.contains("weixin")
        || lower.contains("wxwork")
        || lower.contains("wechatmainwnd")
        || lower.contains("wechatbrowserwnd")
}

fn is_descendant_of(child: HWND, ancestor: HWND) -> bool {
    if child == 0 || ancestor == 0 {
        return false;
    }
    let mut current = child;
    for _ in 0..32 {
        if current == ancestor {
            return true;
        }
        let parent = unsafe { GetParent(current) };
        if parent == 0 || parent == current {
            return false;
        }
        current = parent;
    }
    false
}

struct CanvasSearch {
    found: HWND,
    found_class_priority: i32,
    found_area: i64,
}

fn find_office_canvas_hwnd(top_hwnd: HWND) -> Option<HWND> {
    if top_hwnd == 0 {
        return None;
    }
    // 仅在顶层窗口类看起来像 Office 时启用
    let top_class = get_window_class_name(top_hwnd).unwrap_or_default();
    if !is_office_top_class(&top_class) {
        return None;
    }

    let mut search = CanvasSearch {
        found: 0,
        found_class_priority: 0,
        found_area: 0,
    };
    unsafe {
        EnumChildWindows(
            top_hwnd,
            Some(enum_office_canvas_proc),
            &mut search as *mut _ as LPARAM,
        );
    }
    if search.found != 0 {
        Some(search.found)
    } else {
        None
    }
}

unsafe extern "system" fn enum_office_canvas_proc(hwnd: HWND, lparam: LPARAM) -> BOOL {
    let search = &mut *(lparam as *mut CanvasSearch);
    if IsWindowVisible(hwnd) == 0 {
        return 1;
    }
    let Ok(class_name) = get_window_class_name(hwnd) else {
        return 1;
    };
    let lower = class_name.to_ascii_lowercase();
    let priority = office_canvas_class_priority(&lower);
    if priority <= 0 {
        return 1;
    };
    let mut rect: RECT = std::mem::zeroed();
    if GetWindowRect(hwnd, &mut rect) == 0 {
        return 1;
    }
    let area = (rect.right - rect.left) as i64 * (rect.bottom - rect.top) as i64;
    if area > 2_500
        && (priority > search.found_class_priority
            || (priority == search.found_class_priority && area > search.found_area))
    {
        search.found = hwnd;
        search.found_class_priority = priority;
        search.found_area = area;
    }
    1
}

fn find_paste_host_hwnd(top_hwnd: HWND) -> Option<HWND> {
    if top_hwnd == 0 {
        return None;
    }
    let mut search = CanvasSearch {
        found: 0,
        found_class_priority: 0,
        found_area: 0,
    };
    unsafe {
        EnumChildWindows(
            top_hwnd,
            Some(enum_paste_host_proc),
            &mut search as *mut _ as LPARAM,
        );
    }
    if search.found != 0 {
        Some(search.found)
    } else {
        None
    }
}

unsafe extern "system" fn enum_paste_host_proc(hwnd: HWND, lparam: LPARAM) -> BOOL {
    let search = &mut *(lparam as *mut CanvasSearch);
    if IsWindowVisible(hwnd) == 0 {
        return 1;
    }
    let Ok(class_name) = get_window_class_name(hwnd) else {
        return 1;
    };
    if is_focus_in_ribbon_or_toolbar(hwnd) {
        return 1;
    }
    let priority = paste_host_class_priority(&class_name);
    if priority <= 0 {
        return 1;
    }
    let mut rect: RECT = std::mem::zeroed();
    if GetWindowRect(hwnd, &mut rect) == 0 {
        return 1;
    }
    let area = (rect.right - rect.left) as i64 * (rect.bottom - rect.top) as i64;
    if area > 2_500
        && (priority > search.found_class_priority
            || (priority == search.found_class_priority && area > search.found_area))
    {
        search.found = hwnd;
        search.found_class_priority = priority;
        search.found_area = area;
    }
    1
}

fn paste_host_class_priority(class_name: &str) -> i32 {
    let lower = class_name.to_ascii_lowercase();
    if lower.contains("chrome_renderwidgethosthwnd") {
        110
    } else if lower.contains("cefbrowserwindow")
        || lower.contains("cefwebview")
        || lower.contains("cef-osr-widget")
        || lower.contains("cefclient")
    {
        100
    } else if lower.contains("chrome_widgetwin") {
        90
    } else if is_wechat_window_class(class_name) {
        80
    } else if lower.contains("internet explorer_server") {
        70
    } else if lower.contains("webview") {
        60
    } else {
        0
    }
}

fn is_bindable_window(hwnd: HWND) -> bool {
    get_window_class_name(hwnd)
        .ok()
        .map(|class_name| is_editable_target_class(&class_name))
        .unwrap_or(false)
}

fn is_paste_host_window(hwnd: HWND) -> bool {
    get_window_class_name(hwnd)
        .ok()
        .map(|class_name| is_paste_host_target_class(&class_name))
        .unwrap_or(false)
}

fn is_editable_target_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("edit")
        || lower.contains("textbox")
        || lower.contains("richedit")
        || lower.contains("scintilla")
        || lower.contains("thunderrt6textbox")
        || lower.contains("internet explorer_server")
        || lower.contains("chrome_renderwidgethosthwnd")
        || lower.contains("cef")
        || lower.contains("webview")
        || lower == "_wwg"
        || lower == "_wwb"
        || lower == "_wwf"
        || lower.contains("wps")
}

fn is_paste_host_target_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    lower.contains("chrome_widgetwin")
        || lower.contains("mozillawindowclass")
        || lower.contains("pptframeclass")
        || lower.contains("xlmain")
        || lower.contains("opusapp")
        || is_wechat_window_class(class_name)
        || lower.contains("internet explorer_server")
        || lower.contains("cef")
        || lower.contains("webview")
        || lower == "excel7"
        || lower == "paneclassdc"
        || lower == "_wwg"
        || lower == "_wwb"
        || lower == "_wwf"
        || lower.starts_with("etmain")
        || is_chromium_render_class(class_name)
}

fn is_web_like_target(target: &InputTarget) -> bool {
    matches!(target.app_type.as_str(), "BrowserRIS" | "ChromiumShell")
        || target
            .signature
            .control_class_name
            .as_deref()
            .map(|class_name| is_web_accessibility_host_class(class_name))
            .unwrap_or(false)
}

fn is_direct_paste_class(lower_class_name: &str) -> bool {
    lower_class_name.contains("edit")
        || lower_class_name.contains("textbox")
        || lower_class_name.contains("richedit")
        || lower_class_name.contains("scintilla")
}

fn class_matches(actual: &str, expected_lower: &str) -> bool {
    let actual_lower = actual.to_ascii_lowercase();
    actual_lower == expected_lower
        || actual_lower.contains(expected_lower)
        || expected_lower.contains(&actual_lower)
}

fn enum_top_windows() -> Vec<HWND> {
    let mut hwnds = Vec::<HWND>::new();
    unsafe {
        EnumWindows(
            Some(enum_windows_proc),
            &mut hwnds as *mut Vec<HWND> as LPARAM,
        );
    }
    hwnds
}

unsafe extern "system" fn enum_windows_proc(hwnd: HWND, lparam: LPARAM) -> BOOL {
    let hwnds = &mut *(lparam as *mut Vec<HWND>);
    hwnds.push(hwnd);
    1
}

fn find_child_by_class(parent: HWND, expected_lower: &str) -> Option<HWND> {
    let mut search = ChildSearch {
        expected_lower: expected_lower.to_string(),
        result: 0,
        first_editable: 0,
    };
    unsafe {
        EnumChildWindows(
            parent,
            Some(enum_child_class_proc),
            &mut search as *mut ChildSearch as LPARAM,
        );
    }
    if search.result != 0 {
        Some(search.result)
    } else if search.first_editable != 0 {
        Some(search.first_editable)
    } else {
        None
    }
}

fn find_first_editable_child(parent: HWND) -> Option<HWND> {
    let mut search = ChildSearch {
        expected_lower: String::new(),
        result: 0,
        first_editable: 0,
    };
    unsafe {
        EnumChildWindows(
            parent,
            Some(enum_child_class_proc),
            &mut search as *mut ChildSearch as LPARAM,
        );
    }
    if search.first_editable != 0 {
        Some(search.first_editable)
    } else {
        None
    }
}

struct ChildSearch {
    expected_lower: String,
    result: HWND,
    first_editable: HWND,
}

unsafe extern "system" fn enum_child_class_proc(hwnd: HWND, lparam: LPARAM) -> BOOL {
    let search = &mut *(lparam as *mut ChildSearch);
    if let Ok(class_name) = get_window_class_name(hwnd) {
        if search.first_editable == 0 && is_editable_target_class(&class_name) {
            search.first_editable = hwnd;
        }
        if !search.expected_lower.is_empty() && class_matches(&class_name, &search.expected_lower) {
            search.result = hwnd;
            return 0;
        }
    }
    1
}

fn get_window_class_name(hwnd: HWND) -> Result<String, String> {
    let mut class_buffer = [0u16; 256];
    let class_len =
        unsafe { GetClassNameW(hwnd, class_buffer.as_mut_ptr(), class_buffer.len() as i32) };
    if class_len <= 0 {
        return Err(last_os_error_message());
    }
    Ok(String::from_utf16_lossy(
        &class_buffer[..class_len as usize],
    ))
}

fn get_window_text(hwnd: HWND) -> Result<String, String> {
    let len = unsafe { GetWindowTextLengthW(hwnd) };
    if len <= 0 {
        return Ok(String::new());
    }
    let mut buffer = vec![0u16; len as usize + 1];
    let copied = unsafe { GetWindowTextW(hwnd, buffer.as_mut_ptr(), buffer.len() as i32) };
    if copied <= 0 {
        return Ok(String::new());
    }
    Ok(String::from_utf16_lossy(&buffer[..copied as usize]))
}

fn get_rect_hint(hwnd: HWND) -> Option<RectHint> {
    let mut rect: RECT = unsafe { std::mem::zeroed() };
    let ok = unsafe { GetWindowRect(hwnd, &mut rect) };
    if ok == 0 {
        return None;
    }
    Some(RectHint {
        left: rect.left,
        top: rect.top,
        right: rect.right,
        bottom: rect.bottom,
    })
}

fn detect_accessibility_focus_hint(
    top_hwnd: HWND,
    focus_hwnd: HWND,
    host_rect_hint: Option<&RectHint>,
) -> Option<AccessibilityFocusHint> {
    if focus_hwnd == 0 || !is_web_accessibility_host_hwnd(focus_hwnd) {
        return None;
    }

    let host_rect = resolve_accessibility_host_rect(top_hwnd, focus_hwnd, host_rect_hint)?;
    let top_rect = get_rect_hint(top_hwnd).unwrap_or(host_rect);

    unsafe {
        let _guard = ComInitGuard::new()?;
        let automation: IUIAutomation =
            CoCreateInstance(&CUIAutomation, None, CLSCTX_INPROC_SERVER).ok()?;

        let cursor = get_cursor_point();
        let cursor_in_host = cursor
            .as_ref()
            .map(|point| point_in_rect(point, &host_rect))
            .unwrap_or(false);

        if cursor_in_host {
            if let Some(cursor) = cursor.as_ref() {
                if let Some(hint) =
                    accessibility_hint_from_point(&automation, cursor, &host_rect, &top_rect)
                {
                    return Some(hint);
                }
            }
        }

        if let Ok(element) = automation.GetFocusedElement() {
            let focused_cursor = if cursor_in_host {
                cursor.as_ref()
            } else {
                None
            };
            if let Some(hint) = accessibility_hint_from_element_or_related(
                &automation,
                element,
                &host_rect,
                &top_rect,
                AccessibilityProbeMode::Focused,
                focused_cursor,
            ) {
                if rect_overlaps(&hint.rect_hint, &host_rect) {
                    return Some(hint);
                }
            }
        }

        None
    }
}

fn resolve_accessibility_host_rect(
    top_hwnd: HWND,
    focus_hwnd: HWND,
    host_rect_hint: Option<&RectHint>,
) -> Option<RectHint> {
    let fallback = host_rect_hint
        .copied()
        .or_else(|| get_rect_hint(focus_hwnd))?;

    if let Some(host_hwnd) = find_paste_host_hwnd(top_hwnd) {
        if let Some(host_rect) = get_rect_hint(host_hwnd) {
            if rect_area(&host_rect) > 0
                && (host_hwnd == focus_hwnd
                    || rect_area(&host_rect) < rect_area(&fallback)
                    || rect_overlaps(&host_rect, &fallback))
            {
                return Some(host_rect);
            }
        }
    }

    Some(fallback)
}

unsafe fn accessibility_hint_from_point(
    automation: &IUIAutomation,
    cursor: &SysPoint,
    host_rect: &RectHint,
    top_rect: &RectHint,
) -> Option<AccessibilityFocusHint> {
    let element = automation
        .ElementFromPoint(UiaPoint {
            x: cursor.x,
            y: cursor.y,
        })
        .ok()?;
    accessibility_hint_from_element_or_related(
        automation,
        element,
        host_rect,
        top_rect,
        AccessibilityProbeMode::Point,
        Some(cursor),
    )
}

#[derive(Clone, Copy)]
enum AccessibilityProbeMode {
    Focused,
    Point,
}

unsafe fn accessibility_hint_from_element_or_related(
    automation: &IUIAutomation,
    element: IUIAutomationElement,
    host_rect: &RectHint,
    top_rect: &RectHint,
    mode: AccessibilityProbeMode,
    cursor: Option<&SysPoint>,
) -> Option<AccessibilityFocusHint> {
    let mut best =
        accessibility_candidate_from_element(element.clone(), host_rect, top_rect, mode, cursor);
    let needs_more_specific = matches!(mode, AccessibilityProbeMode::Point)
        || best
            .as_ref()
            .map(|candidate| !accessibility_hint_is_precise(&candidate.hint, host_rect))
            .unwrap_or(true);

    if needs_more_specific {
        if let Some(candidate) = best_accessibility_ancestor_candidate(
            automation,
            element.clone(),
            host_rect,
            top_rect,
            mode,
            cursor,
        ) {
            update_best_accessibility_candidate(&mut best, candidate);
        }

        if let Some(candidate) = best_accessibility_descendant_candidate(
            automation, element, host_rect, top_rect, mode, cursor,
        ) {
            update_best_accessibility_candidate(&mut best, candidate);
        }
    }

    best.map(|candidate| candidate.hint)
}

unsafe fn best_accessibility_ancestor_candidate(
    automation: &IUIAutomation,
    element: IUIAutomationElement,
    host_rect: &RectHint,
    top_rect: &RectHint,
    mode: AccessibilityProbeMode,
    cursor: Option<&SysPoint>,
) -> Option<AccessibilityCandidate> {
    let walker = automation
        .RawViewWalker()
        .or_else(|_| automation.ControlViewWalker())
        .ok()?;
    let mut best = None;
    let mut current = element;

    for _ in 0..MAX_ACCESSIBILITY_ANCESTORS {
        let Ok(parent) = walker.GetParentElement(&current) else {
            break;
        };
        if let Some(candidate) =
            accessibility_candidate_from_element(parent.clone(), host_rect, top_rect, mode, cursor)
        {
            update_best_accessibility_candidate(&mut best, candidate);
        }
        current = parent;
    }

    best
}

unsafe fn best_accessibility_descendant_candidate(
    automation: &IUIAutomation,
    element: IUIAutomationElement,
    host_rect: &RectHint,
    top_rect: &RectHint,
    mode: AccessibilityProbeMode,
    cursor: Option<&SysPoint>,
) -> Option<AccessibilityCandidate> {
    let condition = automation.CreateTrueCondition().ok()?;
    let elements = element.FindAll(TreeScope_Descendants, &condition).ok()?;
    let length = elements
        .Length()
        .ok()
        .map(|value| value.min(MAX_ACCESSIBILITY_DESCENDANTS))?;
    let mut best = None;

    for index in 0..length {
        let Ok(child) = elements.GetElement(index) else {
            continue;
        };
        if let Some(candidate) =
            accessibility_candidate_from_element(child, host_rect, top_rect, mode, cursor)
        {
            update_best_accessibility_candidate(&mut best, candidate);
        }
    }

    best
}

unsafe fn accessibility_candidate_from_element(
    element: IUIAutomationElement,
    host_rect: &RectHint,
    top_rect: &RectHint,
    mode: AccessibilityProbeMode,
    cursor: Option<&SysPoint>,
) -> Option<AccessibilityCandidate> {
    let uia_rect = element.CurrentBoundingRectangle().ok()?;
    let rect_hint = rect_hint_from_uia_rect(uia_rect)?;
    if !is_usable_accessibility_rect(&rect_hint, host_rect, top_rect) {
        return None;
    }

    let contains_cursor = cursor
        .map(|point| point_in_rect(point, &rect_hint))
        .unwrap_or(false);
    if matches!(mode, AccessibilityProbeMode::Point) && !contains_cursor {
        return None;
    }

    let control_type = element.CurrentControlType().ok();
    let has_keyboard_focus = element
        .CurrentHasKeyboardFocus()
        .map(|value| value.0 != 0)
        .unwrap_or(false);
    let has_value_pattern = element.GetCurrentPattern(UIA_ValuePatternId).is_ok();
    let has_text_pattern = element.GetCurrentPattern(UIA_TextPatternId).is_ok();

    if !is_web_input_accessibility_focus(
        control_type,
        has_keyboard_focus,
        has_value_pattern,
        has_text_pattern,
        &rect_hint,
        host_rect,
        mode,
        contains_cursor,
    ) {
        return None;
    }

    let automation_id = element
        .CurrentAutomationId()
        .ok()
        .and_then(|value| non_empty(value.to_string()));
    let control_name = element
        .CurrentName()
        .ok()
        .and_then(|value| non_empty(value.to_string()));
    let score = accessibility_candidate_score(
        control_type,
        has_keyboard_focus,
        has_value_pattern,
        has_text_pattern,
        contains_cursor,
        &rect_hint,
        host_rect,
        mode,
    );
    let control_type_label = control_type.map(uia_control_type_label);
    let hint = AccessibilityFocusHint {
        rect_hint,
        control_type: control_type_label.clone(),
        automation_id,
        control_name,
    };
    let area = rect_area(&hint.rect_hint);

    Some(AccessibilityCandidate { hint, score, area })
}

fn accessibility_hint_is_precise(hint: &AccessibilityFocusHint, host_rect: &RectHint) -> bool {
    let precise_type = hint
        .control_type
        .as_deref()
        .map(|value| value == "UIAutomation:Edit" || value == "UIAutomation:ComboBox")
        .unwrap_or(false);
    precise_type || rect_substantially_smaller(&hint.rect_hint, host_rect)
}

fn update_best_accessibility_candidate(
    best: &mut Option<AccessibilityCandidate>,
    candidate: AccessibilityCandidate,
) {
    let replace = best
        .as_ref()
        .map(|current| {
            candidate.score > current.score
                || (candidate.score == current.score && candidate.area < current.area)
        })
        .unwrap_or(true);

    if replace {
        *best = Some(candidate);
    }
}

fn accessibility_candidate_score(
    control_type: Option<UIA_CONTROLTYPE_ID>,
    has_keyboard_focus: bool,
    has_value_pattern: bool,
    has_text_pattern: bool,
    contains_cursor: bool,
    rect: &RectHint,
    host_rect: &RectHint,
    mode: AccessibilityProbeMode,
) -> i32 {
    let mut score = 0;
    if has_keyboard_focus {
        score += 80;
    }
    if contains_cursor {
        score += 60;
    }
    if has_value_pattern {
        score += 45;
    }
    if has_text_pattern {
        score += 25;
    }

    if let Some(value) = control_type {
        if value == UIA_EditControlTypeId {
            score += 70;
        } else if value == UIA_ComboBoxControlTypeId {
            score += 55;
        } else if value == UIA_DocumentControlTypeId {
            score += 25;
        } else if value == UIA_CustomControlTypeId || value == UIA_PaneControlTypeId {
            score += 18;
        } else if value == UIA_TextControlTypeId {
            score += 6;
        }
    }

    if rect_substantially_smaller(rect, host_rect) {
        score += 30;
    } else {
        score -= 25;
    }

    let area = rect_area(rect);
    let host_area = rect_area(host_rect).max(1);
    if area <= host_area / 8 {
        score += 25;
    } else if area <= host_area / 3 {
        score += 18;
    } else if area <= host_area * 2 / 3 {
        score += 8;
    }

    if matches!(mode, AccessibilityProbeMode::Focused) && !has_keyboard_focus && !contains_cursor {
        score -= 35;
    }

    score
}

struct ComInitGuard;

impl ComInitGuard {
    unsafe fn new() -> Option<Self> {
        let hr = CoInitializeEx(None, COINIT_APARTMENTTHREADED);
        if hr.is_err() {
            return None;
        }
        Some(Self)
    }
}

impl Drop for ComInitGuard {
    fn drop(&mut self) {
        unsafe {
            CoUninitialize();
        }
    }
}

fn is_web_accessibility_host_hwnd(hwnd: HWND) -> bool {
    get_window_class_name(hwnd)
        .ok()
        .map(|class_name| is_web_accessibility_host_class(&class_name))
        .unwrap_or(false)
}

fn is_web_accessibility_host_class(class_name: &str) -> bool {
    let lower = class_name.to_ascii_lowercase();
    is_chromium_render_class(class_name)
        || is_wechat_window_class(class_name)
        || lower.contains("chrome_widgetwin")
        || lower.contains("internet explorer_server")
        || lower.contains("webview")
}

fn rect_hint_from_uia_rect(rect: UiaRect) -> Option<RectHint> {
    let hint = RectHint {
        left: rect.left,
        top: rect.top,
        right: rect.right,
        bottom: rect.bottom,
    };
    if rect_area(&hint) <= 0 {
        None
    } else {
        Some(hint)
    }
}

fn is_usable_accessibility_rect(
    rect: &RectHint,
    host_rect: &RectHint,
    top_rect: &RectHint,
) -> bool {
    if rect_area(rect) <= 16 {
        return false;
    }
    if !rect_overlaps(rect, host_rect) && !rect_overlaps(rect, top_rect) {
        return false;
    }
    true
}

fn is_web_input_accessibility_focus(
    control_type: Option<UIA_CONTROLTYPE_ID>,
    has_keyboard_focus: bool,
    has_value_pattern: bool,
    has_text_pattern: bool,
    rect: &RectHint,
    host_rect: &RectHint,
    mode: AccessibilityProbeMode,
    contains_cursor: bool,
) -> bool {
    let type_accepts_text_without_focus = control_type
        .map(|value| value == UIA_EditControlTypeId || value == UIA_ComboBoxControlTypeId)
        .unwrap_or(false);
    let type_can_host_text_focus = control_type
        .map(|value| {
            value == UIA_DocumentControlTypeId
                || value == UIA_TextControlTypeId
                || value == UIA_CustomControlTypeId
                || value == UIA_PaneControlTypeId
        })
        .unwrap_or(false);
    let type_can_host_point_text = control_type
        .map(|value| {
            value == UIA_DocumentControlTypeId
                || value == UIA_CustomControlTypeId
                || value == UIA_PaneControlTypeId
        })
        .unwrap_or(false);
    let smaller_than_host = rect_substantially_smaller(rect, host_rect);
    let point_text_target = matches!(mode, AccessibilityProbeMode::Point)
        && contains_cursor
        && smaller_than_host
        && has_text_pattern
        && type_can_host_point_text;

    if !smaller_than_host && !has_value_pattern {
        return false;
    }

    if has_value_pattern || type_accepts_text_without_focus {
        return true;
    }

    if matches!(mode, AccessibilityProbeMode::Focused) && !has_keyboard_focus {
        return false;
    }

    type_can_host_text_focus && has_text_pattern && (has_keyboard_focus || point_text_target)
}

fn get_cursor_point() -> Option<SysPoint> {
    let mut point = SysPoint { x: 0, y: 0 };
    let ok = unsafe { GetCursorPos(&mut point) };
    if ok == 0 {
        None
    } else {
        Some(point)
    }
}

fn point_in_rect(point: &SysPoint, rect: &RectHint) -> bool {
    point.x >= rect.left && point.x < rect.right && point.y >= rect.top && point.y < rect.bottom
}

fn uia_control_type_label(control_type: UIA_CONTROLTYPE_ID) -> String {
    let name = if control_type == UIA_EditControlTypeId {
        "Edit"
    } else if control_type == UIA_ComboBoxControlTypeId {
        "ComboBox"
    } else if control_type == UIA_DocumentControlTypeId {
        "Document"
    } else if control_type == UIA_TextControlTypeId {
        "Text"
    } else if control_type == UIA_CustomControlTypeId {
        "Custom"
    } else if control_type == UIA_PaneControlTypeId {
        "Pane"
    } else {
        return format!("UIAutomation:{}", control_type.0);
    };
    format!("UIAutomation:{name}")
}

fn rect_area(rect: &RectHint) -> i64 {
    let width = (rect.right - rect.left).max(0) as i64;
    let height = (rect.bottom - rect.top).max(0) as i64;
    width * height
}

fn rect_overlaps(a: &RectHint, b: &RectHint) -> bool {
    a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top
}

fn rect_substantially_smaller(inner: &RectHint, outer: &RectHint) -> bool {
    let inner_area = rect_area(inner);
    let outer_area = rect_area(outer);
    if inner_area <= 0 || outer_area <= 0 {
        return false;
    }
    inner_area * 100 < outer_area * 80
        || (outer.right - outer.left) - (inner.right - inner.left) > 48
        || (outer.bottom - outer.top) - (inner.bottom - inner.top) > 48
}

fn get_process_path(process_id: u32) -> Option<String> {
    if process_id == 0 {
        return None;
    }
    unsafe {
        let handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, process_id);
        if handle == 0 {
            return None;
        }
        let mut buffer = vec![0u16; 4096];
        let mut size = buffer.len() as u32;
        let ok = QueryFullProcessImageNameW(handle, 0, buffer.as_mut_ptr(), &mut size);
        CloseHandle(handle);
        if ok == 0 || size == 0 {
            return None;
        }
        Some(String::from_utf16_lossy(&buffer[..size as usize]))
    }
}

fn non_empty(value: String) -> Option<String> {
    let trimmed = value.trim();
    if trimmed.is_empty() {
        None
    } else {
        Some(trimmed.to_string())
    }
}

fn same_ci(a: &str, b: &str) -> bool {
    a.eq_ignore_ascii_case(b)
}

fn same_opt_ci(a: Option<&str>, b: Option<&str>) -> bool {
    match (a, b) {
        (Some(a), Some(b)) => a.eq_ignore_ascii_case(b),
        _ => false,
    }
}

fn title_similar(a: Option<&str>, b: Option<&str>) -> bool {
    let (Some(a), Some(b)) = (a, b) else {
        return false;
    };
    let a = a.trim().to_ascii_lowercase();
    let b = b.trim().to_ascii_lowercase();
    if a.is_empty() || b.is_empty() {
        return false;
    }
    a == b || a.contains(&b) || b.contains(&a)
}

fn now_ms() -> i64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| duration.as_millis() as i64)
        .unwrap_or_default()
}

fn last_os_error_message() -> String {
    std::io::Error::last_os_error().to_string()
}

fn command_success(
    state: &str,
    message: String,
    target_id: Option<String>,
    display_name: Option<String>,
) -> BridgeCommandResult {
    BridgeCommandResult {
        success: true,
        message,
        target_id,
        display_name,
        state: Some(state.to_string()),
    }
}

fn command_error(state: &str, message: String) -> BridgeCommandResult {
    BridgeCommandResult {
        success: false,
        message,
        target_id: None,
        display_name: None,
        state: Some(state.to_string()),
    }
}

// =========================================================================
// 持续焦点追踪：通过 SetWinEventHook(EVENT_OBJECT_FOCUS) 记录最近一次可用的
// 可编辑焦点，绑定热键触发时如果当前焦点失效（被自身 UI 抢占或落在 Ribbon
// / 字体下拉框上），就回退到最近一次"真实"输入框焦点。
// =========================================================================

#[derive(Clone, Copy)]
struct StickyFocus {
    top: HWND,
    focus: HWND,
    captured_at_ms: i64,
}

const STICKY_FOCUS_TTL_MS: i64 = 45_000;

static FOCUS_TRACKER_ONCE: Once = Once::new();
static STICKY_FOCUS: Mutex<Option<StickyFocus>> = Mutex::new(None);

fn ensure_focus_tracker() {
    FOCUS_TRACKER_ONCE.call_once(|| {
        let _ = thread::Builder::new()
            .name("voice-bridge-focus".to_string())
            .spawn(focus_tracker_loop);
    });
}

fn focus_tracker_loop() {
    unsafe {
        let hook = SetWinEventHook(
            EVENT_OBJECT_FOCUS,
            EVENT_OBJECT_FOCUS,
            0,
            Some(on_focus_event),
            0,
            0,
            WINEVENT_OUTOFCONTEXT | WINEVENT_SKIPOWNPROCESS,
        );
        if hook == 0 {
            append_log("focus_tracker_hook_failed");
            return;
        }
        append_log("focus_tracker_started");
        let mut msg: MSG = std::mem::zeroed();
        while GetMessageW(&mut msg, 0, 0, 0) > 0 {
            TranslateMessage(&msg);
            DispatchMessageW(&msg);
        }
        UnhookWinEvent(hook);
    }
}

unsafe extern "system" fn on_focus_event(
    _hook: HWINEVENTHOOK,
    _event: u32,
    hwnd: HWND,
    id_object: i32,
    _id_child: i32,
    _thread_id: u32,
    _event_time: u32,
) {
    if hwnd == 0 {
        return;
    }
    // 只关心客户区/窗口对象，忽略菜单、光标、声音等
    if id_object != OBJID_CLIENT && id_object != OBJID_WINDOW {
        return;
    }
    if IsWindow(hwnd) == 0 {
        return;
    }

    let mut pid = 0u32;
    GetWindowThreadProcessId(hwnd, &mut pid);
    if pid == 0 || pid == std::process::id() {
        return;
    }

    let top = {
        let root = GetAncestor(hwnd, GA_ROOT);
        if root != 0 {
            root
        } else {
            hwnd
        }
    };
    if is_own_window(top) {
        return;
    }
    if IsWindowVisible(top) == 0 {
        return;
    }

    // Ribbon / 字体下拉等控件不参与 sticky 记忆
    if is_focus_in_ribbon_or_toolbar(hwnd) {
        return;
    }

    let Some(focus) = resolve_bindable_focus_hwnd(top, hwnd) else {
        return;
    };
    if is_focus_in_ribbon_or_toolbar(focus) {
        return;
    }

    let sticky = StickyFocus {
        top,
        focus,
        captured_at_ms: now_ms(),
    };
    if let Ok(mut guard) = STICKY_FOCUS.lock() {
        *guard = Some(sticky);
    }
}

fn take_sticky_focus_target() -> Option<InputTarget> {
    let sticky = {
        let guard = STICKY_FOCUS.lock().ok()?;
        (*guard)?
    };
    if now_ms() - sticky.captured_at_ms > STICKY_FOCUS_TTL_MS {
        return None;
    }
    unsafe {
        if IsWindow(sticky.top) == 0 || IsWindow(sticky.focus) == 0 {
            return None;
        }
        if IsWindowVisible(sticky.top) == 0 {
            return None;
        }
    }
    build_target(sticky.top, sticky.focus).ok()
}

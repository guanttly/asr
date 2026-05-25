mod hotkeys;
mod injector;
mod input_bridge;
mod tray;

pub use injector::{inject_text, read_clipboard};

use base64::{engine::general_purpose::STANDARD as BASE64_STANDARD, Engine as _};
use serde::{Deserialize, Serialize};
use std::{
    collections::BTreeSet,
    fs::{self, OpenOptions},
    io::Write,
    path::{Path, PathBuf},
    process::Command,
    time::{SystemTime, UNIX_EPOCH},
};
use tauri::Manager;

#[cfg(target_os = "windows")]
fn install_windows_permission_handler<R: tauri::Runtime>(window: &tauri::WebviewWindow<R>) {
    use webview2_com::{
        Microsoft::Web::WebView2::Win32::{
            ICoreWebView2, ICoreWebView2PermissionRequestedEventArgs, COREWEBVIEW2_PERMISSION_KIND,
            COREWEBVIEW2_PERMISSION_KIND_CAMERA, COREWEBVIEW2_PERMISSION_KIND_CLIPBOARD_READ,
            COREWEBVIEW2_PERMISSION_KIND_MICROPHONE, COREWEBVIEW2_PERMISSION_STATE_ALLOW,
        },
        PermissionRequestedEventHandler,
    };

    if let Err(err) = window.with_webview(|webview| unsafe {
        let controller = webview.controller();
        let core = match controller.CoreWebView2() {
            Ok(core) => core,
            Err(err) => {
                log_runtime(&format!("failed to get CoreWebView2: {err}"));
                return;
            }
        };

        let mut token = 0i64;
        if let Err(err) = core.add_PermissionRequested(
            &PermissionRequestedEventHandler::create(Box::new(
                |_: Option<ICoreWebView2>,
                 args: Option<ICoreWebView2PermissionRequestedEventArgs>| {
                    let Some(args) = args else {
                        return Ok(());
                    };

                    let mut kind = COREWEBVIEW2_PERMISSION_KIND::default();
                    args.PermissionKind(&mut kind)?;

                    if kind == COREWEBVIEW2_PERMISSION_KIND_MICROPHONE
                        || kind == COREWEBVIEW2_PERMISSION_KIND_CAMERA
                        || kind == COREWEBVIEW2_PERMISSION_KIND_CLIPBOARD_READ
                    {
                        args.SetState(COREWEBVIEW2_PERMISSION_STATE_ALLOW)?;
                    }

                    Ok(())
                },
            )),
            &mut token,
        ) {
            log_runtime(&format!("failed to add PermissionRequested handler: {err}"));
            return;
        }

        log_runtime("installed windows PermissionRequested handler");
    }) {
        log_runtime(&format!("failed to access native webview: {err}"));
    }
}

#[cfg(target_os = "windows")]
fn install_windows_certificate_handler<R: tauri::Runtime>(window: &tauri::WebviewWindow<R>) {
    use webview2_com::{
        Microsoft::Web::WebView2::Win32::{
            ICoreWebView2, ICoreWebView2ServerCertificateErrorDetectedEventArgs, ICoreWebView2_14,
            COREWEBVIEW2_SERVER_CERTIFICATE_ERROR_ACTION_ALWAYS_ALLOW,
        },
        ServerCertificateErrorDetectedEventHandler,
    };
    use windows::core::Interface as _;

    if let Err(err) = window.with_webview(|webview| unsafe {
        let controller = webview.controller();
        let core = match controller.CoreWebView2() {
            Ok(core) => core,
            Err(err) => {
                log_runtime(&format!(
                    "failed to get CoreWebView2 for certificate handler: {err}"
                ));
                return;
            }
        };

        let core14: ICoreWebView2_14 = match core.cast() {
            Ok(core14) => core14,
            Err(err) => {
                log_runtime(&format!(
                    "failed to cast CoreWebView2 to ICoreWebView2_14: {err}"
                ));
                return;
            }
        };

        let mut token = 0i64;
        if let Err(err) = core14.add_ServerCertificateErrorDetected(
            &ServerCertificateErrorDetectedEventHandler::create(Box::new(
                |_: Option<ICoreWebView2>,
                 args: Option<ICoreWebView2ServerCertificateErrorDetectedEventArgs>| {
                    let Some(args) = args else {
                        return Ok(());
                    };

                    args.SetAction(COREWEBVIEW2_SERVER_CERTIFICATE_ERROR_ACTION_ALWAYS_ALLOW)?;
                    Ok(())
                },
            )),
            &mut token,
        ) {
            log_runtime(&format!("failed to add ServerCertificateErrorDetected handler: {err}"));
            return;
        }

        log_runtime("installed windows ServerCertificateErrorDetected handler");
    }) {
        log_runtime(&format!(
            "failed to access native webview for certificate handler: {err}"
        ));
    }
}

#[cfg(target_os = "windows")]
fn install_windows_webview_handlers<R: tauri::Runtime>(window: &tauri::WebviewWindow<R>) {
    install_windows_permission_handler(window);
    install_windows_certificate_handler(window);
}

fn runtime_log_path() -> PathBuf {
    let root_dir = runtime_root_dir();
    root_dir.join("logs").join("startup.log")
}

fn runtime_root_dir() -> PathBuf {
    #[cfg(target_os = "windows")]
    {
        if let Some(local_app_data) = std::env::var_os("LOCALAPPDATA") {
            return PathBuf::from(local_app_data).join("asr-desktop");
        }
    }

    std::env::temp_dir().join("asr-desktop")
}

fn main_window_state_path() -> PathBuf {
    runtime_root_dir().join("main-window-state.json")
}

fn log_runtime(message: &str) {
    let path = runtime_log_path();
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }

    let timestamp = SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|duration| format!("{}.{:03}", duration.as_secs(), duration.subsec_millis()))
        .unwrap_or_else(|_| "0.000".to_string());

    if let Ok(mut file) = OpenOptions::new().create(true).append(true).open(path) {
        let _ = writeln!(file, "[{timestamp} pid={}] {message}", std::process::id());
    }
}

fn reveal_in_file_manager(path: &Path) -> Result<(), String> {
    #[cfg(target_os = "windows")]
    {
        std::process::Command::new("explorer")
            .arg("/select,")
            .arg(path)
            .spawn()
            .map_err(|err| format!("failed to open Explorer: {err}"))?;
        return Ok(());
    }

    #[cfg(target_os = "macos")]
    {
        std::process::Command::new("open")
            .arg("-R")
            .arg(path)
            .spawn()
            .map_err(|err| format!("failed to reveal file in Finder: {err}"))?;
        return Ok(());
    }

    #[cfg(all(unix, not(target_os = "macos")))]
    {
        let target_dir = path.parent().unwrap_or(path);
        std::process::Command::new("xdg-open")
            .arg(target_dir)
            .spawn()
            .map_err(|err| format!("failed to open file manager: {err}"))?;
        return Ok(());
    }

    #[allow(unreachable_code)]
    Err("opening the file manager is not supported on this platform".to_string())
}

#[derive(Clone, Copy, Deserialize, Serialize)]
struct MainWindowState {
    x: i32,
    y: i32,
}

fn load_main_window_state() -> Option<MainWindowState> {
    let raw = fs::read_to_string(main_window_state_path()).ok()?;
    serde_json::from_str(&raw).ok()
}

fn persist_main_window_state(state: MainWindowState) {
    let path = main_window_state_path();
    if let Some(parent) = path.parent() {
        let _ = fs::create_dir_all(parent);
    }

    match serde_json::to_vec(&state) {
        Ok(bytes) => {
            if let Err(err) = fs::write(path, bytes) {
                log_runtime(&format!("failed to persist main window state: {err}"));
            }
        }
        Err(err) => log_runtime(&format!("failed to serialize main window state: {err}")),
    }
}

fn persist_main_window_position(position: &tauri::PhysicalPosition<i32>) {
    persist_main_window_state(MainWindowState {
        x: position.x,
        y: position.y,
    });
}

fn persist_main_window_position_from_window<R: tauri::Runtime>(window: &tauri::WebviewWindow<R>) {
    match window.outer_position() {
        Ok(position) => persist_main_window_position(&position),
        Err(err) => log_runtime(&format!("failed to read main window position: {err}")),
    }
}

fn restore_main_window_position<R: tauri::Runtime>(window: &tauri::WebviewWindow<R>) {
    let Some(state) = load_main_window_state() else {
        return;
    };

    let position = tauri::Position::Physical(tauri::PhysicalPosition::new(state.x, state.y));
    if let Err(err) = window.set_position(position) {
        log_runtime(&format!("failed to restore main window position: {err}"));
    }
}

fn tray_enabled() -> bool {
    true
}

#[derive(Serialize)]
struct MachineIdentityPayload {
    machine_code: String,
    hostname: String,
    platform: String,
    ip_addresses: Vec<String>,
    mac_addresses: Vec<String>,
}

fn normalize_machine_seed(value: &str) -> Option<String> {
    let normalized = value.trim().trim_matches('\0').to_ascii_lowercase();
    if normalized.is_empty()
        || normalized == "none"
        || normalized == "unknown"
        || normalized == "to be filled by o.e.m."
        || normalized.chars().all(|ch| ch == '0' || ch == '-')
        || normalized.chars().all(|ch| ch == 'f' || ch == '-')
    {
        return None;
    }
    Some(normalized)
}

fn command_output(program: &str, args: &[&str]) -> Option<String> {
    let output = Command::new(program).args(args).output().ok()?;
    if !output.status.success() {
        return None;
    }
    let text = String::from_utf8_lossy(&output.stdout).trim().to_string();
    if text.is_empty() {
        None
    } else {
        Some(text)
    }
}

#[cfg(target_os = "windows")]
fn platform_machine_uuid() -> Option<String> {
    if let Some(output) = command_output("wmic", &["csproduct", "get", "uuid"]) {
        for line in output.lines() {
            let value = line.trim();
            if value.eq_ignore_ascii_case("uuid") {
                continue;
            }
            if let Some(uuid) = normalize_machine_seed(value) {
                return Some(uuid);
            }
        }
    }

    if let Some(output) = command_output("reg", &["query", r"HKLM\SOFTWARE\Microsoft\Cryptography", "/v", "MachineGuid"]) {
        for line in output.lines() {
            if !line.contains("MachineGuid") {
                continue;
            }
            if let Some(value) = line.split_whitespace().last().and_then(normalize_machine_seed) {
                return Some(value);
            }
        }
    }

    None
}

#[cfg(target_os = "linux")]
fn platform_machine_uuid() -> Option<String> {
    for path in ["/sys/class/dmi/id/product_uuid", "/etc/machine-id"] {
        if let Ok(value) = fs::read_to_string(path) {
            if let Some(uuid) = normalize_machine_seed(&value) {
                return Some(uuid);
            }
        }
    }
    None
}

#[cfg(target_os = "macos")]
fn platform_machine_uuid() -> Option<String> {
    let output = command_output("ioreg", &["-rd1", "-c", "IOPlatformExpertDevice"])?;
    for line in output.lines() {
        if !line.contains("IOPlatformUUID") {
            continue;
        }
        if let Some(raw) = line.split('=').nth(1) {
            let value = raw.trim().trim_matches('"');
            if let Some(uuid) = normalize_machine_seed(value) {
                return Some(uuid);
            }
        }
    }
    None
}

#[cfg(not(any(target_os = "windows", target_os = "linux", target_os = "macos")))]
fn platform_machine_uuid() -> Option<String> {
    None
}

fn build_machine_code(hostname: &str, platform: &str, ip_list: &[String], mac_list: &[String]) -> String {
    use sha2::{Digest, Sha256};

    let stable_seed = platform_machine_uuid()
        .map(|value| format!("hardware:{value}"))
        .or_else(|| mac_list.first().and_then(|value| normalize_machine_seed(value)).map(|value| format!("mac:{value}")));
    let fingerprint = match stable_seed {
        Some(stable_id) => serde_json::json!({
            "version": 2,
            "stable_id": stable_id,
        }),
        None => serde_json::json!({
            "version": 1,
            "hostname": hostname,
            "platform": platform,
            "ip_addresses": ip_list,
            "mac_addresses": mac_list,
        }),
    };

    let mut hasher = Sha256::new();
    hasher.update(fingerprint.to_string().as_bytes());
    hex::encode(hasher.finalize())
}

#[tauri::command]
async fn get_machine_identity() -> Result<MachineIdentityPayload, String> {
    // 必须是 async——Tauri 2 同步 command 在主线程执行，
    // hostname/network 系统调用在 Windows 复杂网络环境下可阻塞数秒导致 UI "未响应"。
    let hostname = hostname::get()
        .map_err(|err| err.to_string())?
        .to_string_lossy()
        .trim()
        .to_string();

    let platform = format!("{}-{}", std::env::consts::OS, std::env::consts::ARCH);

    let mut ip_addresses = BTreeSet::new();
    if let Ok(interfaces) = local_ip_address::list_afinet_netifas() {
        for (_, ip_addr) in interfaces {
            if !ip_addr.is_loopback() {
                ip_addresses.insert(ip_addr.to_string());
            }
        }
    }

    let mut mac_addresses = BTreeSet::new();
    if let Ok(Some(address)) = mac_address::get_mac_address() {
        mac_addresses.insert(address.to_string());
    }

    let ip_list: Vec<String> = ip_addresses.into_iter().collect();
    let mac_list: Vec<String> = mac_addresses.into_iter().collect();
    let machine_code = build_machine_code(&hostname, &platform, &ip_list, &mac_list);

    Ok(MachineIdentityPayload {
        machine_code,
        hostname,
        platform,
        ip_addresses: ip_list,
        mac_addresses: mac_list,
    })
}

#[tauri::command]
fn append_runtime_log(scope: String, message: String) {
    log_runtime(&format!("[{scope}] {message}"));
}

#[tauri::command]
fn read_runtime_log_tail(lines: Option<usize>) -> Result<String, String> {
    let max_lines = lines.unwrap_or(120).clamp(1, 400);
    let content = match fs::read_to_string(runtime_log_path()) {
        Ok(content) => content,
        Err(err) if err.kind() == std::io::ErrorKind::NotFound => return Ok(String::new()),
        Err(err) => return Err(err.to_string()),
    };

    let tail = content
        .lines()
        .rev()
        .take(max_lines)
        .collect::<Vec<_>>()
        .into_iter()
        .rev()
        .collect::<Vec<_>>()
        .join("\n");

    Ok(tail)
}

#[tauri::command]
fn get_runtime_log_path() -> String {
    runtime_log_path().to_string_lossy().to_string()
}

#[tauri::command]
async fn save_pdf_file(suggested_name: String, pdf_base64: String) -> Result<bool, String> {
    let Some(file_handle) = rfd::AsyncFileDialog::new()
        .add_filter("PDF", &["pdf"])
        .set_file_name(&suggested_name)
        .save_file()
        .await
    else {
        return Ok(false);
    };

    let pdf_bytes = BASE64_STANDARD
        .decode(pdf_base64)
        .map_err(|err| format!("failed to decode pdf bytes: {err}"))?;

    let mut target_path = file_handle.path().to_path_buf();
    if target_path.extension().is_none() {
        target_path.set_extension("pdf");
    }

    fs::write(&target_path, pdf_bytes).map_err(|err| format!("failed to write pdf file: {err}"))?;

    log_runtime(&format!("saved pdf file to {}", target_path.display()));

    if let Err(err) = reveal_in_file_manager(&target_path) {
        log_runtime(&format!(
            "saved pdf file but failed to open containing folder for {}: {}",
            target_path.display(),
            err
        ));
    }

    Ok(true)
}

#[tauri::command]
fn open_devtools(app: tauri::AppHandle, window: tauri::WebviewWindow) {
    // 优先打开调用者所在窗口的 DevTools
    window.open_devtools();
    // 同时打开另一个窗口的 DevTools（调试两个窗口通信问题时很有用）
    let other_label = if window.label() == "settings" {
        "main"
    } else {
        "settings"
    };
    if let Some(other) = app.get_webview_window(other_label) {
        other.open_devtools();
    }
}

pub(crate) fn show_main_window(app: &tauri::AppHandle) -> Result<(), String> {
    log_runtime("[window] showing main window");
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "main window not found".to_string())?;

    restore_main_window_position(&window);
    window.show().map_err(|err| err.to_string())?;
    window.unminimize().map_err(|err| err.to_string())?;
    window.set_focus().map_err(|err| err.to_string())?;
    Ok(())
}

#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
pub(crate) fn toggle_main_window_internal(app: &tauri::AppHandle) -> Result<(), String> {
    let window = app
        .get_webview_window("main")
        .ok_or_else(|| "main window not found".to_string())?;
    let visible = window.is_visible().map_err(|err| err.to_string())?;
    if visible {
        persist_main_window_position_from_window(&window);
        window.hide().map_err(|err| err.to_string())?;
        return Ok(());
    }
    show_main_window(app)
}

pub(crate) fn open_settings_window_internal(app: &tauri::AppHandle) -> Result<(), String> {
    if let Some(window) = app.get_webview_window("settings") {
        log_runtime("[window] focusing existing settings window");
        window.show().map_err(|err| err.to_string())?;
        window.unminimize().map_err(|err| err.to_string())?;
        window.set_focus().map_err(|err| err.to_string())?;
        return Ok(());
    }

    let mut builder = tauri::WebviewWindowBuilder::new(
        app,
        "settings",
        // 不要在路径中加查询参数，Windows WebView2 的 asset 协议
        // 会把查询参数作为文件名的一部分来查找，导致 404 白屏。
        tauri::WebviewUrl::App("index.html".into()),
    )
    // 在任何页面 JS 之前注入窗口标记——比 getCurrentWindow() 更可靠，
    // 因为 __TAURI_INTERNALS__ 在动态创建的窗口中可能存在初始化时序问题。
    .initialization_script("Object.defineProperty(window,'__ASR_WINDOW__',{value:'settings'})")
    .title("巨鲨语音助手设置")
    .inner_size(440.0, 680.0)
    .min_inner_size(400.0, 560.0)
    .resizable(true)
    .center()
    .decorations(true);

    if let Some(icon) = app.default_window_icon().cloned() {
        builder = builder.icon(icon).map_err(|err| err.to_string())?;
    }

    let window = builder.build().map_err(|err| err.to_string())?;
    log_runtime("[window] created settings window");

    // 注意：Windows WebView2 handlers 必须在 build() 之后调用，
    // 不可省略——设置页面的权限授权和 HTTPS 证书放行都依赖这里。
    #[cfg(target_os = "windows")]
    install_windows_webview_handlers(&window);

    window.show().map_err(|err| err.to_string())?;
    window.set_focus().map_err(|err| err.to_string())?;
    Ok(())
}

#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
pub(crate) fn toggle_settings_window_internal(app: &tauri::AppHandle) -> Result<(), String> {
    if let Some(window) = app.get_webview_window("settings") {
        let visible = window.is_visible().map_err(|err| err.to_string())?;
        if visible {
            window.hide().map_err(|err| err.to_string())?;
            return Ok(());
        }
    }
    open_settings_window_internal(app)
}

#[tauri::command]
async fn open_settings_window(app: tauri::AppHandle) -> Result<(), String> {
    // 必须是 async——同步命令在主线程执行，而 builder.build() 创建的新窗口
    // 会立刻通过 IPC 请求主线程（如 getCurrentWindow()），导致死锁白屏。
    open_settings_window_internal(&app)
}

#[tauri::command]
async fn configure_hotkeys(
    app: tauri::AppHandle,
    bindings: Vec<hotkeys::HotkeyBindingPayload>,
) -> Result<hotkeys::HotkeyConfigureResult, String> {
    hotkeys::configure_hotkeys(&app, bindings)
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    std::panic::set_hook(Box::new(|panic_info| {
        log_runtime(&format!("panic: {panic_info}"));
    }));

    let exe_path = std::env::current_exe()
        .map(|path| path.display().to_string())
        .unwrap_or_else(|_| "unknown".to_string());
    log_runtime(&format!("starting tauri app exe={exe_path}"));

    let enable_tray = tray_enabled();
    log_runtime(&format!("tray enabled: {enable_tray}"));

    let app = tauri::Builder::default()
        .plugin(tauri_plugin_global_shortcut::Builder::new().build())
        .plugin(tauri_plugin_store::Builder::new().build())
        .invoke_handler(tauri::generate_handler![
            inject_text,
            read_clipboard,
            input_bridge::input_bridge_get_state,
            input_bridge::input_bridge_lock_current,
            input_bridge::input_bridge_unlock,
            input_bridge::input_bridge_use_history,
            input_bridge::input_bridge_delete_history,
            input_bridge::input_bridge_flash_overlay,
            input_bridge::input_bridge_paste_text,
            get_machine_identity,
            open_settings_window,
            configure_hotkeys,
            open_devtools,
            append_runtime_log,
            read_runtime_log_tail,
            get_runtime_log_path,
            save_pdf_file
        ])
        .setup(move |app| {
            let window = app.get_webview_window("main").ok_or_else(|| {
                log_runtime("main window not found in tauri config");
                std::io::Error::new(std::io::ErrorKind::Other, "main window missing")
            })?;

            if enable_tray {
                if let Err(err) = tray::setup_tray(&app.handle()) {
                    log_runtime(&format!("tray setup failed: {err}"));
                }
            } else {
                log_runtime("tray setup skipped on this platform");
            }

            if let Err(err) = hotkeys::start_hotkey_service(&app.handle()) {
                log_runtime(&format!("hotkey service setup failed: {err}"));
            }

            // Hide on close only when tray support is enabled; otherwise allow normal exit.
            #[cfg(target_os = "windows")]
            install_windows_webview_handlers(&window);

            let _ = window.set_always_on_top(true);
            restore_main_window_position(&window);

            if let Some(icon) = app.default_window_icon().cloned() {
                let _ = window.set_icon(icon);
            }

            let window_clone = window.clone();
            window.on_window_event(move |event| {
                if let tauri::WindowEvent::Moved(position) = event {
                    persist_main_window_position(position);
                }

                if enable_tray && matches!(event, tauri::WindowEvent::CloseRequested { .. }) {
                    let tauri::WindowEvent::CloseRequested { api, .. } = event else {
                        return;
                    };
                    api.prevent_close();
                    persist_main_window_position_from_window(&window_clone);
                    let _ = window_clone.hide();
                }
            });

            if let Err(err) = show_main_window(&app.handle()) {
                log_runtime(&format!("failed to show main window: {err}"));
            }

            log_runtime("tauri setup completed");

            Ok(())
        })
        .run(tauri::generate_context!());

    if let Err(err) = app {
        log_runtime(&format!("tauri run failed: {err}"));
    }
}

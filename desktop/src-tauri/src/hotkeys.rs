use serde::{Deserialize, Serialize};

#[cfg(not(target_os = "windows"))]
use tauri::AppHandle;

#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HotkeyModifiersPayload {
    pub ctrl: bool,
    pub alt: bool,
    pub shift: bool,
    pub meta: bool,
}

#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
#[derive(Clone, Debug, Deserialize)]
#[serde(tag = "type", rename_all = "camelCase")]
pub enum HotkeyTriggerPayload {
    Keyboard { code: String },
    Mouse { button: String },
}

#[cfg_attr(not(target_os = "windows"), allow(dead_code))]
#[derive(Clone, Debug, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct HotkeyBindingPayload {
    pub action: String,
    pub enabled: bool,
    pub modifiers: HotkeyModifiersPayload,
    pub trigger: HotkeyTriggerPayload,
}

#[derive(Clone, Debug, Serialize)]
#[serde(rename_all = "camelCase")]
pub struct HotkeyConfigureResult {
    pub supported: bool,
    pub registered: usize,
    pub message: String,
}

#[cfg(not(target_os = "windows"))]
pub fn start_hotkey_service(_app: &AppHandle) -> Result<(), String> {
    Ok(())
}

#[cfg(not(target_os = "windows"))]
pub fn configure_hotkeys(
    _app: &AppHandle,
    _bindings: Vec<HotkeyBindingPayload>,
) -> Result<HotkeyConfigureResult, String> {
    Ok(HotkeyConfigureResult {
        supported: false,
        registered: 0,
        message: "当前平台不支持 Windows 全局热键注册，打包到 Windows 后生效。".to_string(),
    })
}

#[cfg(target_os = "windows")]
mod windows_impl {
    use super::{HotkeyBindingPayload, HotkeyConfigureResult, HotkeyModifiersPayload, HotkeyTriggerPayload};
    use std::collections::{BTreeSet, HashMap};
    use std::ptr::null_mut;
    use std::sync::atomic::{AtomicU32, Ordering};
    use std::sync::mpsc::{self, Receiver, Sender};
    use std::sync::{Mutex, OnceLock};
    use std::thread;
    use tauri::{AppHandle, Emitter};
    use windows_sys::Win32::Foundation::{LPARAM, LRESULT, WPARAM};
    use windows_sys::Win32::System::Threading::GetCurrentThreadId;
    use windows_sys::Win32::UI::Input::KeyboardAndMouse::{
        GetAsyncKeyState, RegisterHotKey, UnregisterHotKey, MOD_ALT, MOD_CONTROL,
        MOD_NOREPEAT, MOD_SHIFT, MOD_WIN, VK_BACK, VK_CONTROL, VK_DELETE, VK_DOWN,
        VK_END, VK_ESCAPE, VK_HOME, VK_INSERT, VK_LEFT, VK_MENU, VK_NEXT, VK_PRIOR,
        VK_RETURN, VK_RIGHT, VK_SHIFT, VK_SPACE, VK_TAB, VK_UP,
    };
    use windows_sys::Win32::UI::WindowsAndMessaging::{
        CallNextHookEx, DispatchMessageW, GetMessageW, HC_ACTION, HHOOK, MSG,
        MSLLHOOKSTRUCT, PM_NOREMOVE, PeekMessageW, PostThreadMessageW,
        SetWindowsHookExW, TranslateMessage, UnhookWindowsHookEx, WH_MOUSE_LL,
        WM_APP, WM_HOTKEY, WM_XBUTTONDOWN, XBUTTON1, XBUTTON2,
    };

    const HOTKEY_ACTION_EVENT: &str = "desktop-hotkey-action";
    const HOTKEY_WAKE_MESSAGE: u32 = WM_APP + 41;
    const VK_LWIN: i32 = 0x5B;
    const VK_RWIN: i32 = 0x5C;
    const VK_OEM_1: u32 = 0xBA;
    const VK_OEM_PLUS: u32 = 0xBB;
    const VK_OEM_COMMA: u32 = 0xBC;
    const VK_OEM_MINUS: u32 = 0xBD;
    const VK_OEM_PERIOD: u32 = 0xBE;
    const VK_OEM_2: u32 = 0xBF;
    const VK_OEM_3: u32 = 0xC0;
    const VK_OEM_4: u32 = 0xDB;
    const VK_OEM_5: u32 = 0xDC;
    const VK_OEM_6: u32 = 0xDD;
    const VK_OEM_7: u32 = 0xDE;

    static HOTKEY_SENDER: OnceLock<Sender<ServiceCommand>> = OnceLock::new();
    static HOTKEY_RUNTIME: OnceLock<Mutex<HotkeyRuntime>> = OnceLock::new();
    static HOTKEY_THREAD_ID: AtomicU32 = AtomicU32::new(0);

    #[derive(Clone, Copy, Debug, Eq, PartialEq)]
    enum MouseButtonTrigger {
        Mouse4,
        Mouse5,
    }

    #[derive(Clone, Debug)]
    struct ResolvedMouseBinding {
        action: String,
        modifiers: HotkeyModifiersPayload,
        button: MouseButtonTrigger,
    }

    struct HotkeyRuntime {
        app: AppHandle,
        keyboard_actions: HashMap<i32, String>,
        mouse_bindings: Vec<ResolvedMouseBinding>,
        next_id: i32,
    }

    enum ServiceCommand {
        Configure {
            bindings: Vec<HotkeyBindingPayload>,
            responder: Sender<Result<HotkeyConfigureResult, String>>,
        },
    }

    pub fn start_hotkey_service(app: &AppHandle) -> Result<(), String> {
        if HOTKEY_SENDER.get().is_some() {
            return Ok(());
        }

        let (command_tx, command_rx) = mpsc::channel::<ServiceCommand>();
        let (ready_tx, ready_rx) = mpsc::channel::<Result<(), String>>();
        let app_handle = app.clone();

        thread::Builder::new()
            .name("desktop-hotkeys".to_string())
            .spawn(move || run_hotkey_thread(app_handle, command_rx, ready_tx))
            .map_err(|err| format!("failed to spawn hotkey thread: {err}"))?;

        let ready = ready_rx
            .recv()
            .map_err(|err| format!("failed to initialize hotkey thread: {err}"))?;
        ready?;

        HOTKEY_SENDER
            .set(command_tx)
            .map_err(|_| "hotkey service already initialized".to_string())?;

        Ok(())
    }

    pub fn configure_hotkeys(
        _app: &AppHandle,
        bindings: Vec<HotkeyBindingPayload>,
    ) -> Result<HotkeyConfigureResult, String> {
        let sender = HOTKEY_SENDER
            .get()
            .ok_or_else(|| "Windows 热键服务尚未初始化".to_string())?;
        let (response_tx, response_rx) = mpsc::channel();
        sender
            .send(ServiceCommand::Configure {
                bindings,
                responder: response_tx,
            })
            .map_err(|err| format!("failed to send hotkey config command: {err}"))?;

        let thread_id = HOTKEY_THREAD_ID.load(Ordering::SeqCst);
        if thread_id != 0 {
            unsafe {
                PostThreadMessageW(thread_id, HOTKEY_WAKE_MESSAGE, 0, 0);
            }
        }

        response_rx
            .recv()
            .map_err(|err| format!("failed to receive hotkey config result: {err}"))?
    }

    fn run_hotkey_thread(
        app: AppHandle,
        receiver: Receiver<ServiceCommand>,
        ready_tx: Sender<Result<(), String>>,
    ) {
        let mut bootstrap_msg = MSG::default();
        unsafe {
            PeekMessageW(&mut bootstrap_msg, null_mut(), 0, 0, PM_NOREMOVE);
        }

        let thread_id = unsafe { GetCurrentThreadId() };
        HOTKEY_THREAD_ID.store(thread_id, Ordering::SeqCst);

        let mouse_hook = unsafe { SetWindowsHookExW(WH_MOUSE_LL, Some(mouse_hook_proc), null_mut(), 0) };
        if mouse_hook.is_null() {
            let _ = ready_tx.send(Err("failed to install low-level mouse hook".to_string()));
            return;
        }

        if HOTKEY_RUNTIME
            .set(Mutex::new(HotkeyRuntime {
                app,
                keyboard_actions: HashMap::new(),
                mouse_bindings: Vec::new(),
                next_id: 1,
            }))
            .is_err()
        {
            let _ = ready_tx.send(Err("hotkey runtime already initialized".to_string()));
            unsafe {
                UnhookWindowsHookEx(mouse_hook);
            }
            return;
        }

        let _ = ready_tx.send(Ok(()));
        crate::log_runtime("[hotkey] windows hotkey service started");

        let mut msg = MSG::default();
        loop {
            let status = unsafe { GetMessageW(&mut msg, null_mut(), 0, 0) };
            if status == -1 {
                crate::log_runtime("[hotkey] GetMessageW returned -1");
                break;
            }
            if status == 0 {
                break;
            }

            match msg.message {
                WM_HOTKEY => handle_registered_hotkey(msg.wParam as i32),
                HOTKEY_WAKE_MESSAGE => drain_commands(&receiver),
                _ => unsafe {
                    TranslateMessage(&msg);
                    DispatchMessageW(&msg);
                },
            }
        }

        cleanup_hotkeys(mouse_hook);
    }

    fn drain_commands(receiver: &Receiver<ServiceCommand>) {
        while let Ok(command) = receiver.try_recv() {
            match command {
                ServiceCommand::Configure { bindings, responder } => {
                    let result = configure_hotkeys_internal(bindings);
                    let _ = responder.send(result);
                }
            }
        }
    }

    fn configure_hotkeys_internal(
        bindings: Vec<HotkeyBindingPayload>,
    ) -> Result<HotkeyConfigureResult, String> {
        let runtime_lock = HOTKEY_RUNTIME
            .get()
            .ok_or_else(|| "Windows 热键运行时尚未初始化".to_string())?;
        let mut runtime = runtime_lock
            .lock()
            .map_err(|_| "failed to lock hotkey runtime".to_string())?;

        let filtered: Vec<HotkeyBindingPayload> = bindings
            .into_iter()
            .filter(|binding| binding.enabled)
            .collect();

        let mut signatures = BTreeSet::new();
        for binding in &filtered {
            let signature = binding_signature(binding);
            if !signature.is_empty() && !signatures.insert(signature) {
                return Err(format!("热键 {} 与其他动作重复", describe_binding(binding)));
            }
        }

        unregister_all_keyboard_hotkeys(&mut runtime);
        runtime.mouse_bindings.clear();

        let mut registered_ids = Vec::new();
        let mut registered_total = 0usize;

        for binding in filtered {
            match binding.trigger.clone() {
                HotkeyTriggerPayload::Keyboard { code } => {
                    let vk = keyboard_code_to_vk(&code)
                        .ok_or_else(|| format!("暂不支持将 {} 注册为 Windows 全局热键", code))?;
                    let id = runtime.next_id;
                    runtime.next_id += 1;
                    let modifiers = modifiers_to_flag(&binding.modifiers);

                    let ok = unsafe { RegisterHotKey(null_mut(), id, modifiers, vk) };
                    if ok == 0 {
                        for registered_id in &registered_ids {
                            unsafe {
                                UnregisterHotKey(null_mut(), *registered_id);
                            }
                        }
                        runtime.keyboard_actions.clear();
                        runtime.mouse_bindings.clear();
                        return Err(format!(
                            "Windows 无法注册热键 {}，它可能已被系统或其他应用占用",
                            describe_binding(&binding)
                        ));
                    }

                    registered_ids.push(id);
                    runtime.keyboard_actions.insert(id, binding.action.clone());
                    registered_total += 1;
                }
                HotkeyTriggerPayload::Mouse { button } => {
                    let resolved_button = parse_mouse_button(&button).ok_or_else(|| {
                        format!("暂不支持将 {} 注册为 Windows 全局鼠标热键", button)
                    })?;
                    runtime.mouse_bindings.push(ResolvedMouseBinding {
                        action: binding.action.clone(),
                        modifiers: binding.modifiers.clone(),
                        button: resolved_button,
                    });
                    registered_total += 1;
                }
            }
        }

        crate::log_runtime(&format!(
            "[hotkey] configured {} windows global hotkeys",
            registered_total
        ));

        Ok(HotkeyConfigureResult {
            supported: true,
            registered: registered_total,
            message: format!("已同步 {} 个 Windows 全局热键", registered_total),
        })
    }

    fn unregister_all_keyboard_hotkeys(runtime: &mut HotkeyRuntime) {
        let ids: Vec<i32> = runtime.keyboard_actions.keys().copied().collect();
        for id in ids {
            unsafe {
                UnregisterHotKey(null_mut(), id);
            }
        }
        runtime.keyboard_actions.clear();
    }

    fn cleanup_hotkeys(mouse_hook: HHOOK) {
        if let Some(runtime_lock) = HOTKEY_RUNTIME.get() {
            if let Ok(mut runtime) = runtime_lock.lock() {
                unregister_all_keyboard_hotkeys(&mut runtime);
            }
        }
        if !mouse_hook.is_null() {
            unsafe {
                UnhookWindowsHookEx(mouse_hook);
            }
        }
    }

    fn handle_registered_hotkey(id: i32) {
        let action = HOTKEY_RUNTIME
            .get()
            .and_then(|runtime_lock| runtime_lock.lock().ok())
            .and_then(|runtime| runtime.keyboard_actions.get(&id).cloned());

        if let Some(action) = action {
            dispatch_action(&action);
        }
    }

    fn dispatch_action(action: &str) {
        let app = HOTKEY_RUNTIME
            .get()
            .and_then(|runtime_lock| runtime_lock.lock().ok())
            .map(|runtime| runtime.app.clone());

        let Some(app) = app else {
            return;
        };

        match action {
            "toggleSettingsWindow" => {
                if let Err(err) = crate::toggle_settings_window_internal(&app) {
                    crate::log_runtime(&format!("[hotkey] toggle settings window failed: {err}"));
                }
            }
            "toggleFloatingWindow" => {
                if let Err(err) = crate::toggle_main_window_internal(&app) {
                    crate::log_runtime(&format!("[hotkey] toggle main window failed: {err}"));
                }
            }
            _ => {
                if let Err(err) = app.emit(HOTKEY_ACTION_EVENT, action.to_string()) {
                    crate::log_runtime(&format!("[hotkey] emit hotkey action failed: {err}"));
                }
            }
        }
    }

    fn binding_signature(binding: &HotkeyBindingPayload) -> String {
        match &binding.trigger {
            HotkeyTriggerPayload::Keyboard { code } => format!(
                "keyboard:{}:{}:{}:{}:{}",
                binding.modifiers.ctrl,
                binding.modifiers.alt,
                binding.modifiers.shift,
                binding.modifiers.meta,
                code.trim().to_ascii_lowercase(),
            ),
            HotkeyTriggerPayload::Mouse { button } => format!(
                "mouse:{}:{}:{}:{}:{}",
                binding.modifiers.ctrl,
                binding.modifiers.alt,
                binding.modifiers.shift,
                binding.modifiers.meta,
                button.trim().to_ascii_lowercase(),
            ),
        }
    }

    fn describe_binding(binding: &HotkeyBindingPayload) -> String {
        let mut parts = Vec::new();
        if binding.modifiers.ctrl {
            parts.push("Ctrl".to_string());
        }
        if binding.modifiers.alt {
            parts.push("Alt".to_string());
        }
        if binding.modifiers.shift {
            parts.push("Shift".to_string());
        }
        if binding.modifiers.meta {
            parts.push("Win".to_string());
        }
        match &binding.trigger {
            HotkeyTriggerPayload::Keyboard { code } => parts.push(code.clone()),
            HotkeyTriggerPayload::Mouse { button } => parts.push(button.clone()),
        }
        parts.join("+")
    }

    fn modifiers_to_flag(modifiers: &HotkeyModifiersPayload) -> u32 {
        let mut flags = MOD_NOREPEAT;
        if modifiers.ctrl {
            flags |= MOD_CONTROL;
        }
        if modifiers.alt {
            flags |= MOD_ALT;
        }
        if modifiers.shift {
            flags |= MOD_SHIFT;
        }
        if modifiers.meta {
            flags |= MOD_WIN;
        }
        flags
    }

    fn keyboard_code_to_vk(code: &str) -> Option<u32> {
        let trimmed = code.trim();
        if let Some(rest) = trimmed.strip_prefix("Key") {
            if rest.len() == 1 {
                let ch = rest.as_bytes()[0];
                if ch.is_ascii_uppercase() {
                    return Some(ch as u32);
                }
            }
        }
        if let Some(rest) = trimmed.strip_prefix("Digit") {
            if rest.len() == 1 {
                let ch = rest.as_bytes()[0];
                if ch.is_ascii_digit() {
                    return Some(ch as u32);
                }
            }
        }
        if let Some(rest) = trimmed.strip_prefix('F') {
            if let Ok(index) = rest.parse::<u32>() {
                if (1..=24).contains(&index) {
                    return Some(0x6F + index);
                }
            }
        }

        match trimmed {
            "Space" => Some(VK_SPACE as u32),
            "Escape" => Some(VK_ESCAPE as u32),
            "Enter" => Some(VK_RETURN as u32),
            "Tab" => Some(VK_TAB as u32),
            "Backspace" => Some(VK_BACK as u32),
            "ArrowUp" => Some(VK_UP as u32),
            "ArrowDown" => Some(VK_DOWN as u32),
            "ArrowLeft" => Some(VK_LEFT as u32),
            "ArrowRight" => Some(VK_RIGHT as u32),
            "Insert" => Some(VK_INSERT as u32),
            "Delete" => Some(VK_DELETE as u32),
            "Home" => Some(VK_HOME as u32),
            "End" => Some(VK_END as u32),
            "PageUp" => Some(VK_PRIOR as u32),
            "PageDown" => Some(VK_NEXT as u32),
            "Minus" => Some(VK_OEM_MINUS),
            "Equal" => Some(VK_OEM_PLUS),
            "BracketLeft" => Some(VK_OEM_4),
            "BracketRight" => Some(VK_OEM_6),
            "Semicolon" => Some(VK_OEM_1),
            "Quote" => Some(VK_OEM_7),
            "Comma" => Some(VK_OEM_COMMA),
            "Period" => Some(VK_OEM_PERIOD),
            "Slash" => Some(VK_OEM_2),
            "Backslash" => Some(VK_OEM_5),
            "Backquote" => Some(VK_OEM_3),
            _ => None,
        }
    }

    fn parse_mouse_button(button: &str) -> Option<MouseButtonTrigger> {
        match button.trim().to_ascii_lowercase().as_str() {
            "mouse4" => Some(MouseButtonTrigger::Mouse4),
            "mouse5" => Some(MouseButtonTrigger::Mouse5),
            _ => None,
        }
    }

    fn current_modifier_state() -> HotkeyModifiersPayload {
        HotkeyModifiersPayload {
            ctrl: unsafe { GetAsyncKeyState(i32::from(VK_CONTROL)) } < 0,
            alt: unsafe { GetAsyncKeyState(i32::from(VK_MENU)) } < 0,
            shift: unsafe { GetAsyncKeyState(i32::from(VK_SHIFT)) } < 0,
            meta: unsafe { GetAsyncKeyState(VK_LWIN) } < 0 || unsafe { GetAsyncKeyState(VK_RWIN) } < 0,
        }
    }

    fn mouse_button_from_xbutton(mouse_data: u32) -> Option<MouseButtonTrigger> {
        match hiword(mouse_data) as u16 {
            value if value == XBUTTON1 as u16 => Some(MouseButtonTrigger::Mouse4),
            value if value == XBUTTON2 as u16 => Some(MouseButtonTrigger::Mouse5),
            _ => None,
        }
    }

    fn hiword(value: u32) -> u16 {
        ((value >> 16) & 0xFFFF) as u16
    }

    unsafe extern "system" fn mouse_hook_proc(
        code: i32,
        wparam: WPARAM,
        lparam: LPARAM,
    ) -> LRESULT {
        if code == HC_ACTION as i32 && wparam as u32 == WM_XBUTTONDOWN {
            let info = *(lparam as *const MSLLHOOKSTRUCT);
            if let Some(button) = mouse_button_from_xbutton(info.mouseData) {
                let modifiers = current_modifier_state();
                let action = HOTKEY_RUNTIME
                    .get()
                    .and_then(|runtime_lock| runtime_lock.lock().ok())
                    .and_then(|runtime| {
                        runtime
                            .mouse_bindings
                            .iter()
                            .find(|binding| {
                                binding.button == button
                                    && binding.modifiers.ctrl == modifiers.ctrl
                                    && binding.modifiers.alt == modifiers.alt
                                    && binding.modifiers.shift == modifiers.shift
                                    && binding.modifiers.meta == modifiers.meta
                            })
                            .map(|binding| binding.action.clone())
                    });

                if let Some(action) = action {
                    dispatch_action(&action);
                }
            }
        }

        CallNextHookEx(null_mut(), code, wparam, lparam)
    }
}

#[cfg(target_os = "windows")]
pub use windows_impl::{configure_hotkeys, start_hotkey_service};
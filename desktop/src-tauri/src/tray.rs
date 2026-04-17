use tauri::{
    menu::{Menu, MenuItem},
    tray::TrayIconBuilder,
    AppHandle,
};

use crate::{open_settings_window_internal, show_main_window};

pub fn setup_tray(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let show = MenuItem::with_id(app, "show", "显示悬浮球", true, None::<&str>)?;
    let settings = MenuItem::with_id(app, "settings", "打开设置", true, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "退出", true, None::<&str>)?;

    let menu = Menu::with_items(app, &[&show, &settings, &quit])?;

    let mut builder = TrayIconBuilder::new()
        .tooltip("语音速录助手")
        .menu(&menu)
        .on_menu_event(|app, event| match event.id.as_ref() {
            "show" => {
                let _ = show_main_window(app);
            }
            "settings" => {
                let _ = open_settings_window_internal(app);
            }
            "quit" => {
                app.exit(0);
            }
            _ => {}
        })
        .show_menu_on_left_click(false)
        .on_tray_icon_event(|tray, event| {
            if let tauri::tray::TrayIconEvent::Click {
                button: tauri::tray::MouseButton::Left,
                ..
            } = event
            {
                let app = tray.app_handle();
                let _ = show_main_window(&app);
            }
        });

    if let Some(icon) = app.default_window_icon().cloned() {
        builder = builder.icon(icon);
    }

    builder.build(app)?;

    Ok(())
}

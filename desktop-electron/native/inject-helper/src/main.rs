use std::io::{self, BufRead, Read, Write};

use voice_input_bridge::{handle_request, legacy_inject_text, BridgeRequest};

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if args.iter().any(|arg| arg == "--stdio") {
        run_stdio();
        return;
    }

    run_single_request();
}

fn run_stdio() {
    let stdin = io::stdin();
    let mut stdout = io::stdout();

    for line in stdin.lock().lines() {
        let Ok(line) = line else {
            break;
        };
        let trimmed = line.trim();
        if trimmed.is_empty() {
            continue;
        }

        let response = match serde_json::from_str::<BridgeRequest>(trimmed) {
            Ok(request) => handle_request(request),
            Err(err) => voice_input_bridge::BridgeResponse {
                id: None,
                result: None,
                error: Some(voice_input_bridge::BridgeError {
                    code: "bad_request".to_string(),
                    message: err.to_string(),
                }),
            },
        };

        if let Ok(raw) = serde_json::to_string(&response) {
            let _ = writeln!(stdout, "{raw}");
            let _ = stdout.flush();
        }
    }
}

fn run_single_request() {
    let mut input = String::new();
    if let Err(err) = io::stdin().read_to_string(&mut input) {
        emit_legacy_result(false, &format!("读取输入失败: {err}"));
        return;
    }

    let trimmed = input.trim();
    if trimmed.starts_with('{') {
        match serde_json::from_str::<BridgeRequest>(trimmed) {
            Ok(request) => {
                let response = handle_request(request);
                match serde_json::to_string(&response) {
                    Ok(raw) => println!("{raw}"),
                    Err(err) => emit_legacy_result(false, &format!("序列化响应失败: {err}")),
                }
            }
            Err(err) => emit_legacy_result(false, &format!("解析请求失败: {err}")),
        }
        return;
    }

    let result = legacy_inject_text(&input);
    emit_legacy_result(result.success, &result.message);
}

fn emit_legacy_result(success: bool, message: &str) {
    let status = if success { '1' } else { '0' };
    let sanitized = message.replace(['\r', '\n', '\t'], " ");
    let _ = writeln!(io::stdout(), "{status}\t{sanitized}");
    let _ = io::stdout().flush();
}

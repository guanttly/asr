#[cfg(not(windows))]
fn main() {
    eprintln!("process-killer only supports Windows targets");
    std::process::exit(1);
}

#[cfg(windows)]
fn main() {
    std::process::exit(windows_main());
}

#[cfg(windows)]
fn windows_main() -> i32 {
    let config = match Config::parse() {
        Ok(config) => config,
        Err(message) => {
            eprintln!("{message}");
            eprintln!("usage: process-killer --name <process.exe> [--path <full-exe-path>] [--wait-ms <ms>]");
            return 1;
        }
    };

    match terminate_matching_processes(&config) {
        Ok(summary) => {
            println!(
                "matched={} terminated={} failed={} remaining={}",
                summary.matched, summary.terminated, summary.failed, summary.remaining
            );
            if summary.remaining == 0 { 0 } else { 2 }
        }
        Err(message) => {
            eprintln!("{message}");
            1
        }
    }
}

#[cfg(windows)]
use std::ffi::OsString;
#[cfg(windows)]
use std::os::windows::ffi::OsStringExt;
#[cfg(windows)]
use std::thread;
#[cfg(windows)]
use std::time::{Duration, Instant};
#[cfg(windows)]
use windows_sys::Win32::Foundation::{CloseHandle, INVALID_HANDLE_VALUE};
#[cfg(windows)]
use windows_sys::Win32::System::Diagnostics::ToolHelp::{
    CreateToolhelp32Snapshot, Process32FirstW, Process32NextW, PROCESSENTRY32W,
    TH32CS_SNAPPROCESS,
};
#[cfg(windows)]
use windows_sys::Win32::System::Threading::{
    OpenProcess, QueryFullProcessImageNameW, TerminateProcess, WaitForSingleObject,
    PROCESS_QUERY_LIMITED_INFORMATION, PROCESS_TERMINATE,
};

#[cfg(windows)]
const SYNCHRONIZE_ACCESS: u32 = 0x0010_0000;

#[cfg(windows)]
#[derive(Debug)]
struct Config {
    process_name: String,
    target_path: Option<String>,
    wait_ms: u64,
}

#[cfg(windows)]
impl Config {
    fn parse() -> Result<Self, String> {
        let mut process_name = None;
        let mut target_path = None;
        let mut wait_ms = 5000u64;
        let mut args = std::env::args().skip(1);

        while let Some(arg) = args.next() {
            match arg.as_str() {
                "--name" | "-n" => {
                    process_name = Some(next_value(&mut args, &arg)?);
                }
                "--path" => {
                    target_path = Some(next_value(&mut args, &arg)?);
                }
                "--wait-ms" => {
                    let raw = next_value(&mut args, &arg)?;
                    wait_ms = raw
                        .parse::<u64>()
                        .map_err(|_| format!("invalid --wait-ms value: {raw}"))?;
                }
                "--help" | "-h" => {
                    return Err("process-killer terminates matching Windows processes".to_string());
                }
                value if !value.starts_with('-') && process_name.is_none() => {
                    process_name = Some(value.to_string());
                }
                _ => return Err(format!("unknown argument: {arg}")),
            }
        }

        let process_name = process_name
            .filter(|value| !value.trim().is_empty())
            .ok_or_else(|| "missing --name".to_string())?;

        Ok(Self {
            process_name: process_name.to_ascii_lowercase(),
            target_path: target_path.map(|path| normalize_path(&path)),
            wait_ms,
        })
    }
}

#[cfg(windows)]
#[derive(Debug, Default)]
struct TerminateSummary {
    matched: usize,
    terminated: usize,
    failed: usize,
    remaining: usize,
}

#[cfg(windows)]
#[derive(Debug)]
struct ProcessMatch {
    pid: u32,
}

#[cfg(windows)]
fn next_value(args: &mut impl Iterator<Item = String>, name: &str) -> Result<String, String> {
    args.next()
        .filter(|value| !value.trim().is_empty())
        .ok_or_else(|| format!("missing value for {name}"))
}

#[cfg(windows)]
fn terminate_matching_processes(config: &Config) -> Result<TerminateSummary, String> {
    let deadline = Instant::now() + Duration::from_millis(config.wait_ms);
    let mut summary = TerminateSummary::default();

    loop {
        let matches = collect_matches(config)?;

        if matches.is_empty() {
            summary.remaining = 0;
            return Ok(summary);
        }

        summary.matched += matches.len();

        for process in matches {
            match terminate_process(process.pid, config.wait_ms.min(2000) as u32) {
                Ok(()) => summary.terminated += 1,
                Err(message) => {
                    summary.failed += 1;
                    eprintln!("{message}");
                }
            }
        }

        if Instant::now() >= deadline {
            summary.remaining = collect_matches(config)?.len();
            return Ok(summary);
        }

        thread::sleep(Duration::from_millis(200));
    }
}

#[cfg(windows)]
fn collect_matches(config: &Config) -> Result<Vec<ProcessMatch>, String> {
    let mut matches = Vec::new();
    unsafe {
        let snapshot = CreateToolhelp32Snapshot(TH32CS_SNAPPROCESS, 0);
        if snapshot == INVALID_HANDLE_VALUE {
            return Err("CreateToolhelp32Snapshot failed".to_string());
        }

        let mut entry: PROCESSENTRY32W = std::mem::zeroed();
        entry.dwSize = std::mem::size_of::<PROCESSENTRY32W>() as u32;

        if Process32FirstW(snapshot, &mut entry) == 0 {
            CloseHandle(snapshot);
            return Ok(matches);
        }

        loop {
            let exe_name = utf16z_to_string(&entry.szExeFile);
            if exe_name.to_ascii_lowercase() == config.process_name {
                let image_path = query_process_image_path(entry.th32ProcessID);
                if process_path_matches(image_path.as_deref(), config.target_path.as_deref()) {
                    matches.push(ProcessMatch {
                        pid: entry.th32ProcessID,
                    });
                }
            }

            if Process32NextW(snapshot, &mut entry) == 0 {
                break;
            }
        }

        CloseHandle(snapshot);
    }
    Ok(matches)
}

#[cfg(windows)]
fn terminate_process(pid: u32, wait_ms: u32) -> Result<(), String> {
    unsafe {
        let handle = OpenProcess(
            PROCESS_TERMINATE | PROCESS_QUERY_LIMITED_INFORMATION | SYNCHRONIZE_ACCESS,
            0,
            pid,
        );
        if handle == 0 {
            return Err(format!("OpenProcess failed for pid {pid}"));
        }

        let result = if TerminateProcess(handle, 1) == 0 {
            Err(format!("TerminateProcess failed for pid {pid}"))
        } else {
            WaitForSingleObject(handle, wait_ms);
            Ok(())
        };

        CloseHandle(handle);
        result
    }
}

#[cfg(windows)]
fn query_process_image_path(pid: u32) -> Option<String> {
    unsafe {
        let handle = OpenProcess(PROCESS_QUERY_LIMITED_INFORMATION, 0, pid);
        if handle == 0 {
            return None;
        }

        let mut buffer = vec![0u16; 32768];
        let mut size = buffer.len() as u32;
        let ok = QueryFullProcessImageNameW(handle, 0, buffer.as_mut_ptr(), &mut size);
        CloseHandle(handle);

        if ok == 0 || size == 0 {
            return None;
        }

        Some(OsString::from_wide(&buffer[..size as usize]).to_string_lossy().into_owned())
    }
}

#[cfg(windows)]
fn process_path_matches(image_path: Option<&str>, target_path: Option<&str>) -> bool {
    let Some(target_path) = target_path else {
        return true;
    };

    image_path
        .map(|path| normalize_path(path) == target_path)
        .unwrap_or(true)
}

#[cfg(windows)]
fn utf16z_to_string(value: &[u16]) -> String {
    let end = value.iter().position(|ch| *ch == 0).unwrap_or(value.len());
    OsString::from_wide(&value[..end]).to_string_lossy().into_owned()
}

#[cfg(windows)]
fn normalize_path(value: &str) -> String {
    let mut normalized = value.trim().trim_matches('"').replace('/', "\\");
    if normalized.starts_with(r"\\?\") {
        normalized = normalized[4..].to_string();
    }
    normalized.to_lowercase()
}
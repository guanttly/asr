fn main() {
    println!("cargo:rerun-if-env-changed=ASR_DESKTOP_IGNORE_CERT_ERRORS");
    tauri_build::build()
}

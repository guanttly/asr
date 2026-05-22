import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const scriptPath = fileURLToPath(import.meta.url)
const projectDir = path.resolve(path.dirname(scriptPath), '..')
const helperDir = path.join(projectDir, 'native', 'inject-helper')
const helperManifestPath = path.join(helperDir, 'Cargo.toml')
const helperTargetDir = path.join(helperDir, 'target')
const helperOutputDir = path.join(projectDir, 'build', 'native', 'win32-x64')
const helperNames = ['voice-input-bridge.exe', 'process-killer.exe']
const helperToolchain = '1.77.2'
const helperTarget = 'x86_64-pc-windows-msvc'

function run(command, args, envOverrides = {}) {
  return spawnSync(command, args, {
    cwd: projectDir,
    stdio: 'inherit',
    env: {
      ...process.env,
      ...envOverrides,
      XWIN_ACCEPT_LICENSE: process.env.XWIN_ACCEPT_LICENSE ?? '1',
    },
  })
}

function runOrExit(command, args, label, envOverrides = {}) {
  const result = run(command, args, envOverrides)

  if (result.error)
    throw result.error

  if (result.status !== 0) {
    console.error(`${label}失败。`)
    process.exit(result.status ?? 1)
  }
}

const toolchainInstallArgs = ['toolchain', 'install', helperToolchain, '--profile', 'minimal', '--target', helperTarget]
const toolchainInstall = run('rustup', toolchainInstallArgs)

if (toolchainInstall.error)
  throw toolchainInstall.error

if (toolchainInstall.status !== 0) {
  console.warn('默认 Rust 分发镜像安装旧 Win7 helper toolchain 失败，回退到官方 Rust 分发源。')
  runOrExit('rustup', toolchainInstallArgs, '安装 Win7 helper Rust toolchain', {
    RUSTUP_DIST_SERVER: 'https://static.rust-lang.org',
    RUSTUP_UPDATE_ROOT: 'https://static.rust-lang.org/rustup',
  })
}

runOrExit('cargo', [`+${helperToolchain}`, 'xwin', 'build', '--release', '--target', helperTarget, '--manifest-path', helperManifestPath, '--target-dir', helperTargetDir], '构建 Win7 注入 helper')

fs.mkdirSync(helperOutputDir, { recursive: true })
for (const helperName of helperNames) {
  const builtHelperPath = path.join(helperTargetDir, helperTarget, 'release', helperName)
  const packagedHelperPath = path.join(helperOutputDir, helperName)

  if (!fs.existsSync(builtHelperPath)) {
    console.error(`未找到已构建的 helper: ${builtHelperPath}`)
    process.exit(1)
  }

  fs.copyFileSync(builtHelperPath, packagedHelperPath)
  console.log(`已生成 Win7 helper: ${packagedHelperPath}`)
}
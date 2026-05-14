import { spawnSync } from 'node:child_process'
import fs from 'node:fs'
import os from 'node:os'
import path from 'node:path'
import process from 'node:process'
import { fileURLToPath } from 'node:url'

const scriptPath = fileURLToPath(import.meta.url)
const projectDir = path.resolve(path.dirname(scriptPath), '..')
const repoRoot = path.resolve(projectDir, '..')
const builderEntrypoint = path.join(projectDir, 'node_modules', '.bin', 'electron-builder')
const builderArgs = ['--win', '--x64', '--config', 'electron-builder.yml']
const dockerBuilderBaseImage = 'electronuserland/builder:wine'
const dockerBuilderHeadlessImage = 'asr-electron-builder:wine-xvfb'
const hostRuntimeDir = path.join(os.tmpdir(), 'asr-electron-xdg-runtime')
const hostWinePrefixDir = path.join(os.homedir(), '.cache', 'asr-electron-host-wine-prefix')
const dockerHomeCacheDir = path.join(os.homedir(), '.cache', 'asr-electron-builder-home')
const dockerWinePrefixCacheDir = path.join(os.homedir(), '.cache', 'asr-electron-wine-prefix')
const dockerWinePrefixDir = '/tmp/electron-wine-prefix'
const xvfbServerArgs = '-screen 0 1024x768x24 -nolisten tcp -ac'
const winebootTimeoutSeconds = readTimeoutSeconds('ASR_ELECTRON_WIN_WINEBOOT_TIMEOUT', 180)
const builderTimeoutSeconds = readTimeoutSeconds('ASR_ELECTRON_WIN_BUILDER_TIMEOUT', 3600)
const builderShellCommand = `./node_modules/.bin/electron-builder ${builderArgs.map(arg => JSON.stringify(arg)).join(' ')}`
const buildMode = normalizeBuildMode(process.env.ASR_ELECTRON_WIN_BUILD_MODE)

function readTimeoutSeconds(name, fallback) {
  const value = process.env[name]

  if (!value)
    return fallback

  const seconds = Number.parseInt(value, 10)

  if (Number.isFinite(seconds) && seconds >= 0)
    return seconds

  console.warn(`${name}=${value} 无效，已回退到 ${fallback} 秒。`)
  return fallback
}

function shellQuote(value) {
  return `'${String(value).replaceAll('\'', `'\\''`)}'`
}

function withTimeout(command, seconds) {
  return seconds > 0 ? `timeout ${seconds}s ${command}` : command
}

function normalizeBuildMode(value) {
  const mode = (value || 'auto').trim().toLowerCase()

  if (['auto', 'docker', 'host'].includes(mode))
    return mode

  console.warn(`ASR_ELECTRON_WIN_BUILD_MODE=${value} 无效，已回退到 auto。可选值: auto, docker, host。`)
  return 'auto'
}

function hasCommand(command) {
  const result = spawnSync('sh', ['-lc', `command -v ${command}`], { stdio: 'ignore' })
  return result.status === 0
}

function isHeadlessLinux() {
  return process.platform === 'linux' && !process.env.DISPLAY && !process.env.WAYLAND_DISPLAY
}

function canAutoUseHostWine() {
  if (!hasCommand('wine'))
    return false

  if (process.platform !== 'linux')
    return true

  if (!isHeadlessLinux())
    return true

  if (process.env.ASR_ELECTRON_WIN_HOST_XVFB === '0')
    return false

  return hasCommand('xvfb-run')
}

function ensureHostRuntimeDir() {
  fs.mkdirSync(hostRuntimeDir, { recursive: true, mode: 0o700 })
  fs.chmodSync(hostRuntimeDir, 0o700)
}

function ensureDir(directory, mode) {
  fs.mkdirSync(directory, { recursive: true, mode })

  if (mode)
    fs.chmodSync(directory, mode)
}

function createWineEnv(prefix) {
  return {
    WINEARCH: process.env.WINEARCH ?? 'win64',
    WINEDEBUG: process.env.WINEDEBUG ?? '-all',
    WINEDLLOVERRIDES: process.env.WINEDLLOVERRIDES ?? 'mscoree,mshtml=',
    WINEPREFIX: prefix,
  }
}

function appendDockerEnv(args, env) {
  for (const [name, value] of Object.entries(env))
    args.push('--env', `${name}=${value}`)
}

function hasDockerImage(image) {
  const result = spawnSync('docker', ['image', 'inspect', image], { stdio: 'ignore' })

  if (result.error)
    throw result.error

  return result.status === 0
}

function ensureDockerHeadlessImage() {
  if (hasDockerImage(dockerBuilderHeadlessImage))
    return dockerBuilderHeadlessImage

  console.warn('首次构建本地 xvfb 版 builder:wine 辅助镜像。')

  const dockerfile = `FROM ${dockerBuilderBaseImage}
RUN apt-get update \\
  && apt-get install -y --no-install-recommends xvfb xauth \\
  && rm -rf /var/lib/apt/lists/*
`
  const result = spawnSync('docker', ['build', '--tag', dockerBuilderHeadlessImage, '-'], {
    cwd: repoRoot,
    input: dockerfile,
    stdio: ['pipe', 'inherit', 'inherit'],
  })

  if (result.error)
    throw result.error

  if (result.status !== 0)
    process.exit(result.status ?? 1)

  return dockerBuilderHeadlessImage
}

function runOrExit(command, args, options = {}) {
  const result = spawnSync(command, args, {
    cwd: projectDir,
    stdio: 'inherit',
    ...options,
  })

  if (result.error)
    throw result.error

  process.exit(result.status ?? 1)
}

function buildWithHostWine() {
  if (process.platform === 'linux' && process.env.ASR_ELECTRON_WIN_HOST_XVFB !== '0') {
    const winePrefix = process.env.WINEPREFIX ?? hostWinePrefixDir
    ensureDir(winePrefix, 0o700)

    if (hasCommand('xvfb-run')) {
      ensureHostRuntimeDir()
      runOrExit('xvfb-run', ['-a', builderEntrypoint, ...builderArgs], {
        env: {
          ...process.env,
          ...createWineEnv(winePrefix),
          XDG_RUNTIME_DIR: hostRuntimeDir,
        },
      })
      return
    }

    if (isHeadlessLinux()) {
      console.error('检测到无图形环境；宿主机 Wine 打包需要 xvfb-run。请安装 xvfb，或使用 ASR_ELECTRON_WIN_BUILD_MODE=docker。')
      process.exit(1)
    }
  }

  runOrExit(builderEntrypoint, builderArgs)
}

function assertDockerAvailable() {
  if (hasCommand('docker'))
    return

  console.error('未检测到 Docker，无法使用容器化 Wine 构建 Windows NSIS 安装包。')
  process.exit(1)
}

function buildWithDocker() {
  assertDockerAvailable()

  const electronCache = path.join(os.homedir(), '.cache', 'electron')
  const electronBuilderCache = path.join(os.homedir(), '.cache', 'electron-builder')
  ensureDir(electronCache)
  ensureDir(electronBuilderCache)
  ensureDir(dockerHomeCacheDir, 0o700)
  ensureDir(dockerWinePrefixCacheDir, 0o700)

  const uid = typeof process.getuid === 'function' ? process.getuid() : 1000
  const gid = typeof process.getgid === 'function' ? process.getgid() : 1000
  const useVirtualDisplay = process.platform === 'linux'
  const dockerImage = useVirtualDisplay ? ensureDockerHeadlessImage() : dockerBuilderBaseImage
  const shellSegments = ['mkdir -p /tmp/electron-home /tmp/electron-wine-prefix']
  const wineBuildCommand = [
    withTimeout('wineboot --init', winebootTimeoutSeconds),
    'wineserver -w',
    withTimeout(builderShellCommand, builderTimeoutSeconds),
  ].join(' && ')
  const wineBuildCommandWithCleanup = `${wineBuildCommand}; status=$?; wineserver -w || true; exit $status`

  if (useVirtualDisplay) {
    shellSegments.push('mkdir -p /tmp/runtime-builder', 'chmod 700 /tmp/runtime-builder', 'export XDG_RUNTIME_DIR=/tmp/runtime-builder')
  }

  shellSegments.push(
    useVirtualDisplay
      ? `xvfb-run -a -s ${shellQuote(xvfbServerArgs)} /bin/bash -lc ${shellQuote(wineBuildCommandWithCleanup)}`
      : wineBuildCommandWithCleanup,
  )

  const dockerCommand = [
    'run',
    '--rm',
    '--user', `${uid}:${gid}`,
    '--workdir', '/repo/desktop-electron',
    '--env', 'ELECTRON_CACHE=/tmp/electron-cache',
    '--env', 'ELECTRON_BUILDER_CACHE=/tmp/electron-builder-cache',
    '--env', 'HOME=/tmp/electron-home',
  ]

  appendDockerEnv(dockerCommand, createWineEnv(dockerWinePrefixDir))

  dockerCommand.push(
    '--volume', `${repoRoot}:/repo`,
    '--volume', `${dockerHomeCacheDir}:/tmp/electron-home`,
    '--volume', `${dockerWinePrefixCacheDir}:${dockerWinePrefixDir}`,
    '--volume', `${electronCache}:/tmp/electron-cache`,
    '--volume', `${electronBuilderCache}:/tmp/electron-builder-cache`,
    dockerImage,
    '/bin/bash',
    '-lc',
    shellSegments.join(' && '),
  )

  runOrExit('docker', dockerCommand)
}

if (process.platform === 'linux') {
  const dockerAvailable = hasCommand('docker')
  const wineAvailable = hasCommand('wine')
  const hostWineReady = canAutoUseHostWine()

  if (buildMode === 'docker') {
    console.warn('已选择 Docker Wine + xvfb 打包 Win7 兼容包。')
    buildWithDocker()
  }
  else if (buildMode === 'host') {
    if (!wineAvailable) {
      console.error('ASR_ELECTRON_WIN_BUILD_MODE=host 需要宿主机安装 wine。')
      process.exit(1)
    }
    console.warn('已选择宿主机 Wine 打包 Win7 兼容包。')
    buildWithHostWine()
  }
  else if (hostWineReady) {
    console.warn('Linux 环境默认优先使用宿主机 Wine 打包 Win7 兼容包，仅在宿主机 Wine 不可用时回退到 Docker Wine + xvfb。')
    buildWithHostWine()
  }
  else if (dockerAvailable) {
    console.warn('宿主机 Wine 不可用或缺少无头运行前置，已回退到 Docker Wine + xvfb 打包 Win7 兼容包。')
    buildWithDocker()
  }
  else if (wineAvailable) {
    console.warn('未检测到 Docker，回退到宿主机 Wine 打包 Win7 兼容包。')
    buildWithHostWine()
  }
  else {
    console.error('未检测到 Docker 或 Wine，无法在 Linux 上构建 Windows NSIS 安装包。请安装 Docker 后重试。')
    process.exit(1)
  }
}
else {
  buildWithHostWine()
}
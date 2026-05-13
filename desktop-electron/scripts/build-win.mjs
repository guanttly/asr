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

function hasCommand(command) {
  const result = spawnSync('sh', ['-lc', `command -v ${command}`], { stdio: 'ignore' })
  return result.status === 0
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
  runOrExit(builderEntrypoint, builderArgs)
}

function buildWithDocker() {
  if (!hasCommand('docker')) {
    console.error('未检测到 Wine；同时本机也没有 Docker，无法在 Linux 上构建 Windows NSIS 安装包。')
    process.exit(1)
  }

  const electronCache = path.join(os.homedir(), '.cache', 'electron')
  const electronBuilderCache = path.join(os.homedir(), '.cache', 'electron-builder')
  fs.mkdirSync(electronCache, { recursive: true })
  fs.mkdirSync(electronBuilderCache, { recursive: true })

  const uid = typeof process.getuid === 'function' ? process.getuid() : 1000
  const gid = typeof process.getgid === 'function' ? process.getgid() : 1000
  const dockerCommand = [
    'run',
    '--rm',
    '--user', `${uid}:${gid}`,
    '--workdir', '/repo/desktop-electron',
    '--env', 'ELECTRON_CACHE=/tmp/electron-cache',
    '--env', 'ELECTRON_BUILDER_CACHE=/tmp/electron-builder-cache',
    '--env', 'HOME=/tmp/electron-home',
    '--env', 'WINEPREFIX=/tmp/electron-home/.wine',
    '--volume', `${repoRoot}:/repo`,
    '--volume', `${electronCache}:/tmp/electron-cache`,
    '--volume', `${electronBuilderCache}:/tmp/electron-builder-cache`,
    'electronuserland/builder:wine',
    '/bin/bash',
    '-lc',
    `mkdir -p /tmp/electron-home && ./node_modules/.bin/electron-builder ${builderArgs.join(' ')}`,
  ]

  runOrExit('docker', dockerCommand)
}

if (process.platform === 'linux' && !hasCommand('wine')) {
  console.warn('未检测到宿主机 Wine，自动切换到 electronuserland/builder:wine 进行 Windows 打包。')
  buildWithDocker()
}
else {
  buildWithHostWine()
}
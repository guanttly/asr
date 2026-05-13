import { context } from 'esbuild'
import { spawn } from 'node:child_process'
import { fileURLToPath } from 'node:url'
import path from 'node:path'
import process from 'node:process'

const scriptPath = fileURLToPath(import.meta.url)
const projectDir = path.resolve(path.dirname(scriptPath), '..')
const distDir = path.join(projectDir, 'dist-electron')

const RENDERER_DEV_URL = 'http://127.0.0.1:1430'
process.env.ELECTRON_RENDERER_URL = RENDERER_DEV_URL

const sharedOptions = {
  bundle: true,
  platform: 'node',
  format: 'cjs',
  target: 'node16',
  external: ['electron'],
  outExtension: { '.js': '.cjs' },
  sourcemap: true,
  logLevel: 'info',
}

const mainCtx = await context({
  ...sharedOptions,
  entryPoints: [path.join(projectDir, 'electron', 'main', 'index.ts')],
  outfile: path.join(distDir, 'main', 'index.cjs'),
})
const preloadCtx = await context({
  ...sharedOptions,
  entryPoints: [path.join(projectDir, 'electron', 'preload', 'index.ts')],
  outfile: path.join(distDir, 'preload', 'index.cjs'),
})

await mainCtx.rebuild()
await preloadCtx.rebuild()

const vite = spawn('vite', ['--config', path.join(projectDir, 'vite.config.ts')], {
  stdio: 'inherit',
  cwd: projectDir,
  shell: true,
})

const electronModulePath = (await import('electron')).default
const electron = spawn(electronModulePath, [path.join(distDir, 'main', 'index.cjs')], {
  stdio: 'inherit',
  cwd: projectDir,
})

const cleanup = () => {
  void mainCtx.dispose()
  void preloadCtx.dispose()
  if (!vite.killed)
    vite.kill()
  if (!electron.killed)
    electron.kill()
  process.exit(0)
}

electron.on('exit', cleanup)
process.on('SIGINT', cleanup)
process.on('SIGTERM', cleanup)

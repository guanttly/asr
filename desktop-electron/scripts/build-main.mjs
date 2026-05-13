import { build } from 'esbuild'
import { fileURLToPath } from 'node:url'
import path from 'node:path'

const scriptPath = fileURLToPath(import.meta.url)
const projectDir = path.resolve(path.dirname(scriptPath), '..')
const distDir = path.join(projectDir, 'dist-electron')

const sharedOptions = {
  bundle: true,
  platform: 'node',
  format: 'cjs',
  target: 'node16',
  external: ['electron'],
  outExtension: { '.js': '.cjs' },
  logLevel: 'info',
}

await build({
  ...sharedOptions,
  entryPoints: [path.join(projectDir, 'electron', 'main', 'index.ts')],
  outfile: path.join(distDir, 'main', 'index.cjs'),
})

await build({
  ...sharedOptions,
  entryPoints: [path.join(projectDir, 'electron', 'preload', 'index.ts')],
  outfile: path.join(distDir, 'preload', 'index.cjs'),
})

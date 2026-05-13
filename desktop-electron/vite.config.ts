import { fileURLToPath, URL } from 'node:url'
import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import { defineConfig } from 'vite'
import { resolveBuildMeta } from '../scripts/build-meta'

const desktopSrc = fileURLToPath(new URL('../desktop/src', import.meta.url))
const buildMeta = resolveBuildMeta({
  packageJsonCandidates: [fileURLToPath(new URL('./package.json', import.meta.url))],
})

// Electron 22 内置 Chromium 108，与原 Tauri 端 chrome105 接近，复用 desktop 的 unocss 配置。
export default defineConfig({
  root: fileURLToPath(new URL('.', import.meta.url)),
  base: './',
  publicDir: fileURLToPath(new URL('../desktop/public', import.meta.url)),
  plugins: [
    vue(),
    UnoCSS({
      configFile: fileURLToPath(new URL('../desktop/uno.config.ts', import.meta.url)),
    }),
    AutoImport({
      dts: 'src/auto-imports.d.ts',
      imports: ['vue', 'pinia'],
      dirs: [
        fileURLToPath(new URL('../desktop/src/composables', import.meta.url)),
        fileURLToPath(new URL('../desktop/src/stores', import.meta.url)),
      ],
    }),
  ],
  resolve: {
    alias: {
      '@': desktopSrc,
      '@tauri-apps/api/core': fileURLToPath(new URL('./src/shim/tauri-core.ts', import.meta.url)),
      '@tauri-apps/api/window': fileURLToPath(new URL('./src/shim/tauri-window.ts', import.meta.url)),
      '@tauri-apps/api/event': fileURLToPath(new URL('./src/shim/tauri-event.ts', import.meta.url)),
    },
  },
  define: {
    __APP_VERSION__: JSON.stringify(buildMeta.version),
    __APP_BUILD_CODE__: JSON.stringify(buildMeta.buildCode),
    __APP_BUILD_DATE__: JSON.stringify(buildMeta.buildDate),
  },
  envPrefix: ['VITE_'],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'chrome108',
    minify: 'esbuild',
    sourcemap: false,
  },
  server: {
    host: '127.0.0.1',
    port: 1430,
    strictPort: true,
  },
})

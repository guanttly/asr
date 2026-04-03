import basicSsl from '@vitejs/plugin-basic-ssl'
import { existsSync, readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { fileURLToPath, URL } from 'node:url'

import vue from '@vitejs/plugin-vue'
import UnoCSS from 'unocss/vite'
import AutoImport from 'unplugin-auto-import/vite'
import { NaiveUiResolver } from 'unplugin-vue-components/resolvers'
import Components from 'unplugin-vue-components/vite'
import { defineConfig, loadEnv } from 'vite'

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '')
  const useHttps = env.VITE_DEV_HTTPS !== 'false'
  const certPath = env.VITE_DEV_HTTPS_CERT ? resolve(process.cwd(), env.VITE_DEV_HTTPS_CERT) : ''
  const keyPath = env.VITE_DEV_HTTPS_KEY ? resolve(process.cwd(), env.VITE_DEV_HTTPS_KEY) : ''

  const httpsOptions = certPath && keyPath && existsSync(certPath) && existsSync(keyPath)
    ? {
        cert: readFileSync(certPath),
        key: readFileSync(keyPath),
      }
    : useHttps
        ? {}
        : undefined

  const forwardedHeaders = useHttps
    ? {
        'X-Forwarded-Proto': 'https',
      }
    : undefined

  return {
    plugins: [
      vue(),
      UnoCSS(),
      AutoImport({
        dts: 'src/auto-imports.d.ts',
        imports: ['vue', 'vue-router', 'pinia'],
        dirs: ['src/composables', 'src/stores'],
      }),
      Components({
        dts: 'src/components.d.ts',
        resolvers: [NaiveUiResolver()],
      }),
      ...(useHttps ? [basicSsl()] : []),
    ],
    resolve: {
      alias: {
        '@': fileURLToPath(new URL('./src', import.meta.url)),
      },
    },
    server: {
      host: '0.0.0.0',
      port: 5173,
      https: httpsOptions,
      proxy: {
        '/api': {
          target: 'http://127.0.0.1:10010',
          changeOrigin: false,
          headers: forwardedHeaders,
        },
        '/uploads': {
          target: 'http://127.0.0.1:10010',
          changeOrigin: false,
          headers: forwardedHeaders,
        },
        '/ws': {
          target: 'ws://127.0.0.1:10010',
          ws: true,
          changeOrigin: false,
          headers: forwardedHeaders,
        },
      },
    },
  }
})

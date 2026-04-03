import { defineConfig, presetAttributify, presetUno } from 'unocss'

export default defineConfig({
  presets: [presetUno(), presetAttributify()],
  theme: {
    colors: {
      ink: '#16202c',
      mist: '#eef3f8',
      sand: '#f5ede2',
      tide: '#0f766e',
      ember: '#d97706',
      slate: '#435266',
    },
    fontFamily: {
      sans: '"IBM Plex Sans", "Noto Sans SC", sans-serif',
      display: '"Space Grotesk", "Noto Sans SC", sans-serif',
    },
  },
  shortcuts: {
    'panel-shell': 'rounded-3 border border-white/60 bg-white/68 shadow-sm backdrop-blur-xl',
    'page-shell': 'h-[100dvh] min-h-0 overflow-hidden bg-[radial-gradient(circle_at_top_left,_rgba(15,118,110,0.12),_transparent_32%),radial-gradient(circle_at_top_right,_rgba(217,119,6,0.08),_transparent_28%),linear-gradient(180deg,_#f5ede2_0%,_#eef3f8_40%,_#f7fafc_100%)] text-ink',
    'card-main': '!rounded-3 !bg-white/82 !backdrop-blur-lg !border !border-gray-200/40 !shadow-[0_1px_3px_rgba(0,0,0,0.04),0_6px_24px_rgba(0,0,0,0.03)]',
    'subtle-panel': 'rounded-2.5 bg-mist/60 p-3.5',
  },
})
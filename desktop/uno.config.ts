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
    'mic-idle': 'bg-slate/10 text-slate border-slate/20',
    'mic-recording': 'bg-red-500 text-white border-red-500',
    'mic-processing': 'bg-tide text-white border-tide',
  },
})

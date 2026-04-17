import { invoke } from '@tauri-apps/api/core'

interface InjectResult {
  success: boolean
  message: string
}

export function useInjector() {
  const injectText = async (text: string): Promise<InjectResult> => {
    return invoke<InjectResult>('inject_text', { text })
  }

  const readClipboard = async (): Promise<string> => {
    return invoke<string>('read_clipboard')
  }

  return { injectText, readClipboard }
}

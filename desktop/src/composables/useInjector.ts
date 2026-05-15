import { invoke } from '@tauri-apps/api/core'
import { useInputBridge } from './useInputBridge'

interface InjectResult {
  success: boolean
  message: string
}

export function useInjector() {
  const inputBridge = useInputBridge()

  const injectText = async (text: string): Promise<InjectResult> => {
    return inputBridge.pasteText(text, 'asr-final')
  }

  const readClipboard = async (): Promise<string> => {
    return invoke<string>('read_clipboard')
  }

  return { injectText, readClipboard }
}

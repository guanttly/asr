import { reactive } from 'vue'

export type ConfirmTone = 'default' | 'danger' | 'primary'

export interface ConfirmOptions {
  title?: string
  message: string
  confirmText?: string
  cancelText?: string
  tone?: ConfirmTone
}

interface ConfirmState extends Required<ConfirmOptions> {
  visible: boolean
  resolver: ((value: boolean) => void) | null
}

const state = reactive<ConfirmState>({
  visible: false,
  title: '提示',
  message: '',
  confirmText: '确认',
  cancelText: '取消',
  tone: 'primary',
  resolver: null,
})

export function useConfirm() {
  function confirm(options: ConfirmOptions): Promise<boolean> {
    return new Promise((resolve) => {
      // 上一个未结束的 confirm 直接以取消收尾，避免 promise 悬挂
      if (state.resolver)
        state.resolver(false)

      state.title = options.title?.trim() || '提示'
      state.message = options.message
      state.confirmText = options.confirmText?.trim() || '确认'
      state.cancelText = options.cancelText?.trim() || '取消'
      state.tone = options.tone || 'primary'
      state.visible = true
      state.resolver = resolve
    })
  }

  function resolveConfirm(value: boolean) {
    const resolver = state.resolver
    state.resolver = null
    state.visible = false
    resolver?.(value)
  }

  return {
    state,
    confirm,
    resolveConfirm,
  }
}

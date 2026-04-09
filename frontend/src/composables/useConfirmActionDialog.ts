import type { VNodeChild } from 'vue'

import { useDialog } from 'naive-ui'
import { h } from 'vue'

interface ConfirmActionOptions {
  title: string
  message: string
  description?: string
  positiveText?: string
  negativeText?: string
  positiveType?: 'default' | 'primary' | 'info' | 'success' | 'warning' | 'error'
  renderContent?: () => VNodeChild
}

export function useConfirmActionDialog() {
  const dialog = useDialog()

  return function confirmAction(options: ConfirmActionOptions) {
    return new Promise<boolean>((resolve) => {
      let settled = false
      const finish = (value: boolean) => {
        if (settled)
          return
        settled = true
        resolve(value)
      }

      const instance = dialog.warning({
        title: options.title,
        positiveText: options.positiveText || '确认',
        negativeText: options.negativeText || '取消',
        maskClosable: false,
        closeOnEsc: false,
        closable: false,
        positiveButtonProps: {
          type: options.positiveType || 'warning',
        },
        content: () => options.renderContent
          ? options.renderContent()
          : h('div', { class: 'grid gap-3 leading-6' }, [
              h('div', { class: 'text-sm text-ink' }, options.message),
              options.description
                ? h('div', { class: 'text-xs text-slate' }, options.description)
                : null,
            ]),
        onPositiveClick: () => {
          finish(true)
        },
        onNegativeClick: () => {
          finish(false)
        },
        onClose: () => {
          finish(false)
        },
        onAfterLeave: () => {
          instance.destroy()
        },
      })
    })
  }
}

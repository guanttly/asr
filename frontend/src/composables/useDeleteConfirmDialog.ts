import { useConfirmActionDialog } from './useConfirmActionDialog'

interface DeleteConfirmOptions {
  title?: string
  entityType: string
  entityName: string
  description?: string
}

export function useDeleteConfirmDialog() {
  const confirmAction = useConfirmActionDialog()

  return function confirmDelete(options: DeleteConfirmOptions) {
    return confirmAction({
      title: options.title || `确认删除${options.entityType}`,
      message: `将要删除${options.entityType}「${options.entityName}」。`,
      description: options.description || '删除后不可恢复，请确认当前对象不再需要保留。',
      positiveText: '确认删除',
      positiveType: 'error',
    })
  }
}

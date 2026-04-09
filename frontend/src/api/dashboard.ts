import request from './request'

export function getDashboardOverview() {
  return request.get('/api/admin/dashboard/overview')
}

export function syncDashboardTask(taskId: string | number) {
  return request.post(`/api/admin/dashboard/tasks/${taskId}/sync`)
}

export function retryDashboardPostProcessTasks(limit: number, taskIds?: number[]) {
  return request.post('/api/admin/dashboard/tasks/retry-post-process', {
    limit,
    task_ids: taskIds,
  })
}

export function clearDashboardRetryHistory() {
  return request.post('/api/admin/dashboard/retry-history/clear')
}

export function deleteDashboardRetryHistoryItem(createdAt: string) {
  return request.post('/api/admin/dashboard/retry-history/delete-item', {
    created_at: createdAt,
  })
}

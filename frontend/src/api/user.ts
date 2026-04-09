import type { WorkflowBindings } from '@/types/workflow'

import request from './request'

export function login(payload: { username: string, password: string }) {
  return request.post('/api/admin/auth/login', payload)
}

export function getUsers(params?: { offset?: number, limit?: number }) {
  return request.get('/api/admin/users', { params })
}

export function createUser(payload: { username: string, password: string, display_name?: string, role: 'admin' | 'user' }) {
  return request.post('/api/admin/users', payload)
}

export function getCurrentUser() {
  return request.get('/api/admin/me')
}

export function getCurrentUserWorkflowBindings() {
  return request.get('/api/admin/me/workflow-bindings')
}

export function updateCurrentUserWorkflowBindings(payload: WorkflowBindings) {
  return request.put('/api/admin/me/workflow-bindings', payload)
}

import request from './request'

export interface DownloadArtifact {
  name: string
  size_bytes: number
  modified_at: string
  download_url: string
}

export function getDownloadArtifacts() {
  return request.get('/api/admin/public/downloads')
}
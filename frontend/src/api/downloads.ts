import request from './request'

export interface DownloadArtifact {
  name: string
  size_bytes: number
  modified_at: string
  download_url: string
  // 后端按文件名识别的目标平台：
  // - "win10+"：Tauri 客户端，推荐 Windows 10/11 用户使用
  // - "win7"：Electron 22 客户端，专为 Windows 7 兼容打包
  // - "other"：未识别，前端归入通用区
  platform?: 'win7' | 'win10+' | 'other'
}

export function getDownloadArtifacts() {
  return request.get('/api/admin/public/downloads')
}

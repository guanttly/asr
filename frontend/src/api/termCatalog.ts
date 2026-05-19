import request from './request'

export interface CatalogTreeNode {
  name: string
  path: string
  is_dir: boolean
  title?: string
  excel_path?: string
  total_terms?: number
  l1_count?: number
  l2_count?: number
  l3_count?: number
  children?: CatalogTreeNode[]
}

export interface CatalogTerm {
  key: string
  standard_term: string
  english_or_abbr: string
  pinyin: string
  mixed_score: number
  rare_score: number
  glyph_score: number
  level: 'L1' | 'L2' | 'L3' | ''
  common_misrecs: string[]
  notes: string
  subsection_title: string
  source_path: string
}

export interface CatalogFileDetail {
  path: string
  name: string
  title: string
  markdown_body: string
  terms: CatalogTerm[]
}

export function getTermCatalogTree() {
  return request.get<{ items: CatalogTreeNode[], source: string }>('/api/admin/term-catalog/tree')
}

export function getTermCatalogFile(path: string) {
  return request.get<CatalogFileDetail>('/api/admin/term-catalog/file', { params: { path } })
}

export function termCatalogExportUrl() {
  return '/api/admin/term-catalog/export.xlsx'
}

export function downloadTermCatalogXlsx(path: string) {
  return request.get<Blob>(termCatalogExportUrl(), {
    params: { path },
    responseType: 'blob',
  }) as unknown as Promise<Blob>
}

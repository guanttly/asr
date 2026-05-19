import request from './request'

export interface RulesTreeNode {
  name: string
  path: string
  is_dir: boolean
  title?: string
  excel_path?: string
  total_rules?: number
  enabled_count?: number
  regex_count?: number
  literal_count?: number
  children?: RulesTreeNode[]
}

export interface CatalogRule {
  key: string
  category: string
  pattern: string
  replacement: string
  match_type: 'literal' | 'regex' | 'number_normalize'
  priority: number
  conflict_group: string
  enabled: boolean
  example: string
  notes: string
  subsection_title: string
  source_path: string
}

export interface RulesFileDetail {
  path: string
  name: string
  title: string
  markdown_body: string
  rules: CatalogRule[]
}

export interface BatchImportResult {
  imported: number
  dict_id: number
}

export function getRulesCatalogTree() {
  return request.get<{ items: RulesTreeNode[], source: string }>('/api/admin/rules-catalog/tree')
}

export function getRulesCatalogFile(path: string) {
  return request.get<RulesFileDetail>('/api/admin/rules-catalog/file', { params: { path } })
}

export function rulesCatalogExportUrl() {
  return '/api/admin/rules-catalog/export.xlsx'
}

export function downloadRulesCatalogXlsx(path: string) {
  return request.get<Blob>(rulesCatalogExportUrl(), {
    params: { path },
    responseType: 'blob',
  }) as unknown as Promise<Blob>
}

export function importRulesCatalogXlsx(file: File, dictId?: number) {
  const form = new FormData()
  form.append('file', file)
  const params = dictId ? { dict_id: dictId } : {}
  return request.post<BatchImportResult>('/api/admin/rules-catalog/import', form, {
    params,
    headers: { 'Content-Type': 'multipart/form-data' },
  })
}

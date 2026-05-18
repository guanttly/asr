<script setup lang="ts">
import type { CatalogFileDetail, CatalogTreeNode } from '@/api/termCatalog'
import MarkdownIt from 'markdown-it'
import { NButton, NSpin, NTabPane, NTabs, NTag, useMessage } from 'naive-ui'
import { computed, onMounted, ref, watch } from 'vue'

import request from '@/api/request'
import { getTermCatalogFile, getTermCatalogTree, termCatalogExportUrl } from '@/api/termCatalog'

interface TabLevel {
  // Path of the parent directory ('.' for root).
  parentPath: string
  // Currently selected child at this level.
  activeKey: string
  // Tab options to render at this level.
  options: CatalogTreeNode[]
}

const message = useMessage()

const tree = ref<CatalogTreeNode[]>([])
const source = ref('')
const treeLoading = ref(false)
const detail = ref<CatalogFileDetail | null>(null)
const detailLoading = ref(false)
const downloading = ref(false)
const selectedPath = ref<string | null>(null)

const markdown = new MarkdownIt({ html: false, linkify: false, breaks: false })

const tabLevels = computed<TabLevel[]>(() => {
  const levels: TabLevel[] = []
  if (!tree.value.length || !selectedPath.value)
    return levels

  const segments = selectedPath.value.split('/')
  let cursor: CatalogTreeNode[] = tree.value
  let parentPath = '.'

  for (let depth = 0; depth < segments.length; depth++) {
    const segment = segments[depth]
    const activeNode = cursor.find(node => node.name === segment)
    const activeKey = activeNode ? activeNode.path : (cursor[0]?.path || '')
    levels.push({ parentPath, activeKey, options: cursor })

    if (!activeNode || !activeNode.is_dir || !activeNode.children?.length)
      break
    cursor = activeNode.children
    parentPath = activeNode.path
  }

  return levels
})

const renderedMarkdown = computed(() => {
  if (!detail.value)
    return ''
  return markdown.render(detail.value.markdown_body)
})

const totalTerms = computed(() => sumTerms(tree.value))
const totalL1 = computed(() => sumLevel(tree.value, 'l1_count'))
const totalL2 = computed(() => sumLevel(tree.value, 'l2_count'))
const totalL3 = computed(() => sumLevel(tree.value, 'l3_count'))

function sumTerms(nodes: CatalogTreeNode[]): number {
  let total = 0
  for (const node of nodes) {
    if (node.is_dir)
      total += sumTerms(node.children || [])
    else
      total += node.total_terms || 0
  }
  return total
}

function sumLevel(nodes: CatalogTreeNode[], key: 'l1_count' | 'l2_count' | 'l3_count'): number {
  let total = 0
  for (const node of nodes) {
    if (node.is_dir)
      total += sumLevel(node.children || [], key)
    else
      total += node[key] || 0
  }
  return total
}

function findFirstFile(nodes: CatalogTreeNode[]): CatalogTreeNode | null {
  for (const node of nodes) {
    if (!node.is_dir)
      return node
    if (node.children?.length) {
      const found = findFirstFile(node.children)
      if (found)
        return found
    }
  }
  return null
}

function findNodeByPath(nodes: CatalogTreeNode[], path: string): CatalogTreeNode | null {
  for (const node of nodes) {
    if (node.path === path)
      return node
    if (node.is_dir && node.children?.length) {
      const found = findNodeByPath(node.children, path)
      if (found)
        return found
    }
  }
  return null
}

async function loadTree() {
  treeLoading.value = true
  try {
    const result = await getTermCatalogTree()
    tree.value = result.data.items || []
    source.value = result.data.source || ''

    if (!tree.value.length) {
      selectedPath.value = null
      detail.value = null
      return
    }

    if (selectedPath.value && findNodeByPath(tree.value, selectedPath.value)) {
      await loadDetail(selectedPath.value)
    }
    else {
      const first = findFirstFile(tree.value)
      if (first) {
        selectedPath.value = first.path
        await loadDetail(first.path)
      }
    }
  }
  catch {
    message.error('术语目录加载失败')
  }
  finally {
    treeLoading.value = false
  }
}

async function loadDetail(path: string) {
  detailLoading.value = true
  try {
    const result = await getTermCatalogFile(path)
    detail.value = result.data
  }
  catch {
    detail.value = null
    message.error('术语文件加载失败')
  }
  finally {
    detailLoading.value = false
  }
}

function handleTabChange(value: string) {
  const node = findNodeByPath(tree.value, value)
  if (!node)
    return

  if (node.is_dir) {
    // Drill into the directory: pick the first file inside.
    const target = findFirstFile(node.children || [])
    if (target)
      selectedPath.value = target.path
    return
  }
  selectedPath.value = node.path
}

watch(selectedPath, (value) => {
  if (value)
    loadDetail(value)
})

async function handleDownload() {
  downloading.value = true
  try {
    const response = await request.get(termCatalogExportUrl(), { responseType: 'blob' })
    const blob = response.data instanceof Blob
      ? response.data
      : new Blob([response.data as BlobPart], { type: 'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet' })
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = 'radiology-term-catalog.xlsx'
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
    message.success('Excel 已下载，可按需修改后到「术语库管理」上传')
  }
  catch {
    message.error('Excel 下载失败')
  }
  finally {
    downloading.value = false
  }
}

function tabLabel(node: CatalogTreeNode) {
  const label = node.title?.trim() || node.name.replace(/\.md$/i, '')
  return label
}

onMounted(loadTree)
</script>

<template>
  <div class="catalog-page flex-1 flex flex-col min-h-0 gap-4">
    <NCard class="card-main">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="text-base font-600 text-ink">
            影像术语库浏览
          </div>
          <div class="mt-1 text-xs text-slate">
            按目录浏览影像报告易错术语；下方按钮可下载全部术语 Excel，本地修改后到「术语库管理 → 批量导入」上传到目标词库。
          </div>
          <div class="mt-2 flex flex-wrap items-center gap-2 text-[12px] text-slate">
            <NTag size="small" round bordered type="default">
              共 {{ totalTerms }} 条
            </NTag>
            <NTag size="small" round bordered type="default">
              L1 {{ totalL1 }}
            </NTag>
            <NTag size="small" round bordered type="warning">
              L2 {{ totalL2 }}
            </NTag>
            <NTag size="small" round bordered type="error">
              L3 {{ totalL3 }}
            </NTag>
            <NTag v-if="source" size="small" round :bordered="false" type="info">
              数据源：{{ source }}
            </NTag>
          </div>
        </div>
        <div class="flex items-center gap-2">
          <NButton size="small" quaternary @click="loadTree">
            刷新
          </NButton>
          <NButton size="small" type="primary" color="#0f766e" :loading="downloading" @click="handleDownload">
            下载全部术语 Excel
          </NButton>
        </div>
      </div>
    </NCard>

    <NCard class="card-main flex flex-col min-h-0" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;gap:12px;">
      <NSpin :show="treeLoading || detailLoading" class="flex-1 min-h-0 flex flex-col">
        <div v-if="tabLevels.length" class="catalog-tab-stack flex flex-col gap-1 pt-1">
          <NTabs
            v-for="(level, depth) in tabLevels"
            :key="`${level.parentPath}@${depth}`"
            type="card"
            size="small"
            :value="level.activeKey"
            :tabs-padding="6"
            @update:value="handleTabChange"
          >
            <NTabPane
              v-for="node in level.options"
              :key="node.path"
              :name="node.path"
              :tab="tabLabel(node)"
              display-directive="show"
            />
          </NTabs>
        </div>

        <div v-if="detail" class="catalog-markdown-wrap mt-2 flex-1 min-h-0 overflow-auto rounded-3 border border-mist bg-white/95">
          <div class="markdown-body p-6 text-[14px] leading-7" v-html="renderedMarkdown" />
        </div>

        <NEmpty v-else-if="!treeLoading" description="未加载到术语目录" class="flex-1 self-center" />
      </NSpin>
    </NCard>
  </div>
</template>

<style scoped>
.catalog-page :deep(.n-tabs .n-tabs-tab) {
  font-size: 13px;
}

.catalog-tab-stack :deep(.n-tabs-pane-wrapper),
.catalog-tab-stack :deep(.n-tab-pane) {
  display: none;
}

.markdown-body :deep(h1) {
  margin: 0 0 18px;
  padding-bottom: 10px;
  border-bottom: 1px solid rgba(15, 23, 42, 0.08);
  color: #0f172a;
  font-size: 22px;
  font-weight: 700;
  line-height: 30px;
}

.markdown-body :deep(h2) {
  margin: 28px 0 12px;
  color: #0f172a;
  font-size: 17px;
  font-weight: 600;
  line-height: 24px;
}

.markdown-body :deep(h3) {
  margin: 18px 0 10px;
  color: #0f172a;
  font-size: 15px;
  font-weight: 600;
  line-height: 22px;
}

.markdown-body :deep(p) {
  margin: 10px 0;
  color: #334155;
}

.markdown-body :deep(blockquote) {
  margin: 14px 0;
  padding: 8px 14px;
  border-left: 3px solid #0f766e;
  background: rgba(15, 118, 110, 0.06);
  color: #334155;
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin: 10px 0 10px 22px;
  color: #334155;
}

.markdown-body :deep(li) {
  margin: 4px 0;
}

.markdown-body :deep(code) {
  padding: 1px 6px;
  border-radius: 4px;
  background: rgba(15, 118, 110, 0.08);
  color: #0f766e;
  font-size: 12.5px;
}

.markdown-body :deep(pre) {
  margin: 12px 0;
  padding: 12px 14px;
  overflow-x: auto;
  border-radius: 6px;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 12.5px;
  line-height: 20px;
}

.markdown-body :deep(pre code) {
  padding: 0;
  background: transparent;
  color: inherit;
}

.markdown-body :deep(table) {
  margin: 14px 0;
  width: 100%;
  border-collapse: separate;
  border-spacing: 0;
  border: 1px solid rgba(15, 23, 42, 0.1);
  border-radius: 8px;
  overflow: hidden;
  font-size: 13px;
  background: #fff;
}

.markdown-body :deep(thead) {
  background: linear-gradient(180deg, #f8fafc 0%, #f1f5f9 100%);
}

.markdown-body :deep(th),
.markdown-body :deep(td) {
  padding: 8px 12px;
  border-bottom: 1px solid rgba(15, 23, 42, 0.06);
  border-right: 1px solid rgba(15, 23, 42, 0.06);
  vertical-align: top;
  text-align: left;
}

.markdown-body :deep(th:last-child),
.markdown-body :deep(td:last-child) {
  border-right: none;
}

.markdown-body :deep(tbody tr:last-child td) {
  border-bottom: none;
}

.markdown-body :deep(th) {
  color: #475569;
  font-weight: 600;
  white-space: nowrap;
}

.markdown-body :deep(tbody tr:hover td) {
  background: rgba(15, 118, 110, 0.04);
}

.markdown-body :deep(td:nth-child(7)) {
  font-weight: 600;
}

.markdown-body :deep(a) {
  color: #0f766e;
  text-decoration: none;
}

.markdown-body :deep(a:hover) {
  text-decoration: underline;
}

.markdown-body :deep(hr) {
  margin: 24px 0;
  border: none;
  border-top: 1px dashed rgba(15, 23, 42, 0.12);
}
</style>

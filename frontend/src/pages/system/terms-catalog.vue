<script setup lang="ts">
import type { MenuOption } from 'naive-ui'
import type { CatalogFileDetail, CatalogTreeNode } from '@/api/termCatalog'
import MarkdownIt from 'markdown-it'
import { NButton, NMenu, NSpin, NTag, useMessage } from 'naive-ui'
import { computed, onMounted, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { downloadTermCatalogXlsx, getTermCatalogFile, getTermCatalogTree } from '@/api/termCatalog'
import {
  catalogMenuLabel,
  findCatalogNodeByPath,
  findCatalogParentPaths,
  findCatalogPathNodes,
  findFirstCatalogFile,
} from '@/utils/termCatalogMenu'

const message = useMessage()
const route = useRoute()
const router = useRouter()

const tree = ref<CatalogTreeNode[]>([])
const source = ref('')
const treeLoading = ref(false)
const detail = ref<CatalogFileDetail | null>(null)
const detailLoading = ref(false)
const downloading = ref(false)
const selectedPath = ref<string | null>(null)
const catalogExpandedKeys = ref<string[]>([])

const markdown = new MarkdownIt({ html: false, linkify: false, breaks: false })

const catalogMenuOptions = computed<MenuOption[]>(() => buildCatalogMenuOptions(tree.value))
const selectedNodeTrail = computed(() => selectedPath.value ? findCatalogPathNodes(tree.value, selectedPath.value) : [])
const activeCatalogScope = computed(() => selectedNodeTrail.value.find(node => node.is_dir) || null)
const activeCatalogScopeLabel = computed(() => {
  if (activeCatalogScope.value)
    return catalogMenuLabel(activeCatalogScope.value)
  return '目录'
})
const catalogTitle = computed(() => activeCatalogScopeLabel.value === '目录'
  ? '术语收集'
  : `术语收集 · ${activeCatalogScopeLabel.value}`)
const activeScopeNodes = computed(() => activeCatalogScope.value?.children || tree.value)
const downloadButtonLabel = computed(() => activeCatalogScope.value
  ? `下载${activeCatalogScopeLabel.value}术语 Excel`
  : '下载术语 Excel')

const renderedMarkdown = computed(() => {
  if (!detail.value)
    return ''
  return markdown.render(detail.value.markdown_body)
})

const totalTerms = computed(() => sumTerms(activeScopeNodes.value))
const totalL1 = computed(() => sumLevel(activeScopeNodes.value, 'l1_count'))
const totalL2 = computed(() => sumLevel(activeScopeNodes.value, 'l2_count'))
const totalL3 = computed(() => sumLevel(activeScopeNodes.value, 'l3_count'))

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

function currentRouteCatalogPath(): string | null {
  const value = route.query.path
  if (typeof value === 'string')
    return value
  if (Array.isArray(value) && typeof value[0] === 'string')
    return value[0]
  return null
}

function resolveSelectableFile(path: string | null): CatalogTreeNode | null {
  if (!path)
    return null
  const node = findCatalogNodeByPath(tree.value, path)
  if (!node)
    return null
  if (!node.is_dir)
    return node
  return findFirstCatalogFile(node.children || [])
}

function syncRoutePath(path: string, replace = false) {
  if (currentRouteCatalogPath() === path)
    return
  const location = { path: route.path, query: { ...route.query, path } }
  if (replace)
    void router.replace(location)
  else
    void router.push(location)
}

function buildCatalogMenuOptions(nodes: CatalogTreeNode[]): MenuOption[] {
  return nodes
    .map((node): MenuOption | null => {
      if (node.is_dir) {
        const children = buildCatalogMenuOptions(node.children || [])
        if (!children.length)
          return null
        return {
          label: catalogMenuLabel(node),
          key: node.path,
          children,
        }
      }
      return {
        label: catalogMenuLabel(node),
        key: node.path,
      }
    })
    .filter((item): item is MenuOption => Boolean(item))
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

    const target = resolveSelectableFile(currentRouteCatalogPath())
      || resolveSelectableFile(selectedPath.value)
      || findFirstCatalogFile(tree.value)

    if (target) {
      const previousPath = selectedPath.value
      selectedPath.value = target.path
      syncRoutePath(target.path, true)
      if (previousPath === target.path)
        await loadDetail(target.path)
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
    if (selectedPath.value === path)
      detail.value = result.data
  }
  catch {
    if (selectedPath.value === path)
      detail.value = null
    message.error('术语文件加载失败')
  }
  finally {
    detailLoading.value = false
  }
}

function handleCatalogSelect(value: string) {
  const target = resolveSelectableFile(value)
  if (!target)
    return

  selectedPath.value = target.path
  syncRoutePath(target.path)
}

function handleCatalogExpand(keys: string[]) {
  catalogExpandedKeys.value = keys
}

watch(selectedPath, (value) => {
  if (value) {
    catalogExpandedKeys.value = findCatalogParentPaths(tree.value, value)
    loadDetail(value)
  }
  else {
    detail.value = null
    catalogExpandedKeys.value = []
  }
})

watch(
  () => route.query.path,
  () => {
    if (!tree.value.length)
      return
    const target = resolveSelectableFile(currentRouteCatalogPath())
    if (target && target.path !== selectedPath.value)
      selectedPath.value = target.path
  },
)

watch(tree, () => {
  if (selectedPath.value)
    catalogExpandedKeys.value = findCatalogParentPaths(tree.value, selectedPath.value)
})

async function handleDownload() {
  const scope = activeCatalogScope.value
  if (!scope?.excel_path) {
    message.warning('当前科室尚未配置术语 Excel')
    return
  }
  downloading.value = true
  try {
    const blob = await downloadTermCatalogXlsx(scope.path)
    const url = URL.createObjectURL(blob)
    const anchor = document.createElement('a')
    anchor.href = url
    anchor.download = `${activeCatalogScopeLabel.value}-术语库.xlsx`
    document.body.appendChild(anchor)
    anchor.click()
    anchor.remove()
    URL.revokeObjectURL(url)
    message.success(`${activeCatalogScopeLabel.value}术语 Excel 已下载，可按需修改后到「术语库管理」上传`)
  }
  catch {
    message.error('Excel 下载失败')
  }
  finally {
    downloading.value = false
  }
}

onMounted(loadTree)
</script>

<template>
  <div class="catalog-page flex-1 flex flex-col min-h-0 gap-4">
    <NCard class="card-main">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <div class="text-base font-600 text-ink">
            {{ catalogTitle }}
          </div>
          <div class="mt-1 text-xs text-slate">
            按学科目录浏览医学易错术语；可下载当前科室内置 Excel，本地修改后到「术语库管理 → 批量导入」上传到目标词库。
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
          <NButton size="small" type="primary" color="#0f766e" :loading="downloading" :disabled="!activeCatalogScope?.excel_path" @click="handleDownload">
            {{ downloadButtonLabel }}
          </NButton>
        </div>
      </div>
    </NCard>

    <NCard class="catalog-body-card card-main flex flex-col min-h-0" content-style="display:flex;min-height:0;height:100%;padding:0;">
      <NSpin :show="treeLoading || detailLoading" class="catalog-spin">
        <div class="catalog-browser">
          <aside class="catalog-sidebar">
            <div class="catalog-sidebar-title">
              {{ activeCatalogScopeLabel }}
            </div>
            <NMenu
              class="catalog-tree-menu"
              :value="selectedPath || undefined"
              :options="catalogMenuOptions"
              :expanded-keys="catalogExpandedKeys"
              :root-indent="12"
              :indent="18"
              @update:value="handleCatalogSelect"
              @update:expanded-keys="handleCatalogExpand"
            />
          </aside>

          <section class="catalog-document">
            <div v-if="detail" class="catalog-markdown-wrap flex-1 min-h-0 overflow-auto rounded-2 border border-mist bg-white/95">
              <div class="markdown-body p-6 text-[14px] leading-7" v-html="renderedMarkdown" />
            </div>

            <NEmpty v-else-if="!treeLoading" description="未加载到术语目录" class="catalog-empty" />
          </section>
        </div>
      </NSpin>
    </NCard>
  </div>
</template>

<style scoped>
.catalog-page {
  overflow: hidden;
}

.catalog-body-card {
  flex: 1 1 auto;
}

.catalog-spin {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  width: 100%;
}

.catalog-spin :deep(.n-spin-content) {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  width: 100%;
}

.catalog-browser {
  display: grid;
  grid-template-columns: minmax(220px, 264px) minmax(0, 1fr);
  flex: 1 1 auto;
  min-height: 0;
  width: 100%;
}

.catalog-sidebar {
  min-height: 0;
  overflow: auto;
  padding: 16px 12px;
  border-right: 1px solid rgba(15, 23, 42, 0.08);
  background: rgba(248, 250, 252, 0.62);
}

.catalog-sidebar-title {
  padding: 0 10px 10px;
  color: #475569;
  font-size: 12px;
  font-weight: 700;
}

.catalog-tree-menu :deep(.n-menu-item-content),
.catalog-tree-menu :deep(.n-submenu > .n-menu-item-content) {
  border-radius: 8px !important;
}

.catalog-tree-menu :deep(.n-menu-item-content::before),
.catalog-tree-menu :deep(.n-submenu > .n-menu-item-content::before) {
  border-radius: 8px !important;
}

.catalog-tree-menu :deep(.n-menu-item-content-header) {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.catalog-tree-menu :deep(.n-menu-item-content.n-menu-item-content--selected::before) {
  background: rgba(15, 118, 110, 0.12) !important;
}

.catalog-document {
  display: flex;
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  padding: 16px 18px 18px;
}

.catalog-empty {
  align-self: center;
  justify-self: center;
  width: 100%;
}

@media (max-width: 960px) {
  .catalog-browser {
    grid-template-columns: minmax(0, 1fr);
    grid-template-rows: minmax(0, auto) minmax(0, 1fr);
  }

  .catalog-sidebar {
    max-height: 260px;
    border-right: none;
    border-bottom: 1px solid rgba(15, 23, 42, 0.08);
  }

  .catalog-document {
    padding: 14px;
  }
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

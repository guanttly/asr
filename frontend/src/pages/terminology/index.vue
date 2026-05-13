<script setup lang="ts">
import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, reactive, ref } from 'vue'

import {
  createTermDict,
  createTermEntry,
  createTermRule,
  deleteTermDict,
  deleteTermEntry,
  deleteTermRule,
  getTermDicts,
  getTermEntries,
  getTermRules,
  importTermEntries,
  updateTermDict,
  updateTermEntry,
  updateTermRule,
} from '@/api/terminology'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'

interface DictItem {
  id: number
  name: string
  domain: string
}

interface EntryItem {
  id: number
  correct_term: string
  wrong_variants: string[]
}

interface RuleItem {
  id: number
  match_type: string
  pattern: string
  replacement: string
  enabled: boolean
  sort_order: number
  priority?: number
  conflict_group?: string
}

type RuleMatchType = 'literal' | 'regex' | 'number_normalize'

interface RuleMatchMeta {
  label: string
  badge: string
  summary: string
  detail: string
  patternLabel: string
  patternPlaceholder: string
  replacementLabel: string
  replacementPlaceholder: string
}

interface RuleExample {
  title: string
  pattern?: string
  replacement?: string
  before: string
  after: string
}

interface RulePreviewState {
  before: string
  after: string
  hint: string
}

const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()
const loading = ref(false)
const entryLoading = ref(false)
const ruleLoading = ref(false)
const dictSaving = ref(false)
const entrySaving = ref(false)
const ruleSaving = ref(false)
const importingEntries = ref(false)
const deletingDictId = ref<number | null>(null)
const deletingEntryId = ref<number | null>(null)
const deletingRuleId = ref<number | null>(null)
const showDictModal = ref(false)
const showEntryModal = ref(false)
const showRuleModal = ref(false)
const showImportModal = ref(false)
const editingDictId = ref<number | null>(null)
const editingEntryId = ref<number | null>(null)
const editingRuleId = ref<number | null>(null)
const currentDictId = ref<number | null>(null)
const importFileInput = ref<HTMLInputElement | null>(null)
const lastImportResult = ref<{ imported: number, skipped: number } | null>(null)
const detailTab = ref<'entries' | 'rules'>('entries')
const dicts = ref<DictItem[]>([])
const entries = ref<EntryItem[]>([])
const rules = ref<RuleItem[]>([])
const dictForm = reactive({
  name: '',
  domain: '',
})
const entryForm = reactive({
  correctTerm: '',
  wrongVariantsText: '',
})
const ruleForm = reactive<{
  matchType: RuleMatchType
  pattern: string
  replacement: string
  sortOrder: number
}>({
  matchType: 'regex',
  pattern: '',
  replacement: '',
  sortOrder: 100,
})

const currentDict = computed(() => dicts.value.find(item => item.id === currentDictId.value) || null)
const dictModalTitle = computed(() => editingDictId.value ? '编辑术语词库' : '新建术语词库')
const entryModalTitle = computed(() => editingEntryId.value ? '编辑词条' : '新增词条')
const ruleModalTitle = computed(() => editingRuleId.value ? '编辑纠错规则' : '新增纠错规则')
const ruleConflictWarnings = computed(() => {
  const groups = new Map<string, RuleItem[]>()
  for (const rule of rules.value) {
    if (!rule.enabled || rule.match_type === 'number_normalize')
      continue
    const key = rule.conflict_group?.trim() || `${rule.match_type}:${rule.pattern.trim()}`
    if (!key.endsWith(':')) {
      const items = groups.get(key) || []
      items.push(rule)
      groups.set(key, items)
    }
  }
  return Array.from(groups.entries())
    .filter(([, items]) => items.length > 1)
    .map(([key, items]) => `${key} 存在 ${items.length} 条启用规则，请调整优先级或关闭重复规则。`)
})

const ruleMatchMeta: Record<RuleMatchType, RuleMatchMeta> = {
  literal: {
    label: '词条替换（旧）',
    badge: '已迁移',
    summary: '固定错词由术语词条的误写变体承接。',
    detail: '旧规则仍会继续执行，新增规则不再提供固定错词配置。',
    patternLabel: '错误文字',
    patternPlaceholder: '',
    replacementLabel: '正确文字',
    replacementPlaceholder: '',
  },
  number_normalize: {
    label: '数字格式自动规范',
    badge: '默认规则',
    summary: '按词库场景自动整理口语数字、尺寸和小数。',
    detail: '这类规则由系统默认写入数据库，可在列表里启停或调整执行顺序。',
    patternLabel: '',
    patternPlaceholder: '',
    replacementLabel: '',
    replacementPlaceholder: '',
  },
  regex: {
    label: '高级匹配',
    badge: '唯一可新增',
    summary: '用于成批调整复杂格式，由匹配式和替换结果组成。',
    detail: '新增规则只维护高级匹配；常见数字规范和场景格式已随默认词库写入数据库。',
    patternLabel: '高级匹配式',
    patternPlaceholder: '如：血压(\\d+)/(\\d+)',
    replacementLabel: '替换结果',
    replacementPlaceholder: '如：血压$1-$2',
  },
}

const ruleExamples: Record<RuleMatchType, RuleExample[]> = {
  literal: [
    { title: '旧版错词', pattern: '舒张亚', replacement: '舒张压', before: '患者舒张亚偏高', after: '患者舒张压偏高' },
  ],
  number_normalize: [
    { title: '尺寸和小数', before: '大小十二乘二十三毫米，血钾二点三', after: '大小12x23mm，血钾2.3' },
  ],
  regex: [
    { title: '血压格式', pattern: '血压(\\d+)/(\\d+)', replacement: '血压$1-$2', before: '血压120/80', after: '血压120-80' },
    { title: '时间格式', pattern: '([0-2]?[0-9])点([0-5]?[0-9])分?', replacement: '$1:$2', before: '会议3点20分开始', after: '会议3:20开始' },
  ],
}

const currentRuleMatch = computed(() => ruleMatchMeta[ruleForm.matchType] || ruleMatchMeta.regex)
const currentRuleExamples = computed(() => ruleExamples[ruleForm.matchType])
const rulePreview = computed(() => buildRulePreview())

function ruleMatchLabel(value: string) {
  return ruleMatchMeta[value as RuleMatchType]?.label || value
}

function normalizeRuleMatchType(value: string): RuleMatchType {
  return value in ruleMatchMeta ? value as RuleMatchType : 'regex'
}

function applyRegexPreview(pattern: string, replacement: string, source: string) {
  try {
    const matcher = new RegExp(pattern, 'g')
    const next = source.replace(matcher, replacement)
    return next === source ? null : next
  }
  catch {
    return null
  }
}

function guessRulePreviewSource(pattern: string) {
  const normalized = pattern.trim()
  if (!normalized)
    return ''

  if (normalized.includes('血压') && normalized.includes('/'))
    return '血压120/80'
  if (normalized.includes('点') && normalized.includes('分'))
    return '会议3点20分开始'
  if (normalized.includes('第') && (normalized.includes('项') || normalized.includes('条')))
    return '第十项 '
  if (normalized.includes('毫米') || normalized.includes('乘'))
    return '大小十二乘二十三毫米，血钾二点三'

  return ''
}

function formatRulePreviewText(text: string) {
  const showInvisible = (value: string) => value
    .replace(/ /g, '[space]')
    .replace(/\t/g, '[tab]')
    .replace(/\r?\n/g, '[newline]')

  return text
    .replace(/^\s+/, showInvisible)
    .replace(/\s+$/, showInvisible)
}

function resolveRulePreview(matchType: RuleMatchType, pattern: string, replacement: string): RulePreviewState {
  const examples = ruleExamples[matchType] || ruleExamples.regex
  const fallback = examples[0] || ruleExamples.regex[0]
  const trimmedPattern = pattern.trim()
  const trimmedReplacement = replacement.trim()

  if (matchType === 'number_normalize') {
    return {
      before: fallback.before,
      after: fallback.after,
      hint: '这类规则由默认配置提供，不需要维护错词。',
    }
  }

  if (matchType === 'literal') {
    if (!trimmedPattern || !trimmedReplacement) {
      return {
        before: fallback.before,
        after: fallback.after,
        hint: '旧版固定错词规则仍会执行，新增请使用术语词条或高级匹配。',
      }
    }

    const before = fallback.before.includes(trimmedPattern)
      ? fallback.before
      : `患者${trimmedPattern}待复核`

    return {
      before,
      after: before.split(trimmedPattern).join(trimmedReplacement),
      hint: '预览仅用于辅助核对，最终以后端保存校验为准。',
    }
  }

  if (!trimmedPattern || !trimmedReplacement) {
    return {
      before: fallback.before,
      after: fallback.after,
      hint: '填写上方两项后，这里会展示高级匹配效果。',
    }
  }

  const previewSources = Array.from(new Set([
    ...examples.map(item => item.before),
    guessRulePreviewSource(trimmedPattern),
  ].filter(Boolean)))

  for (const source of previewSources) {
    const after = applyRegexPreview(trimmedPattern, trimmedReplacement, source)
    if (after) {
      return {
        before: source,
        after,
        hint: '预览仅用于辅助核对，最终以后端保存校验为准。',
      }
    }
  }

  return {
    before: '实际命中的原文片段',
    after: '会按当前规则替换成对应结果',
    hint: '这条规则暂时无法自动生成可靠示例，请在编辑弹窗中校对。',
  }
}

function buildRulePreview() {
  return resolveRulePreview(ruleForm.matchType, ruleForm.pattern, ruleForm.replacement)
}

function renderRulePreview(row: RuleItem) {
  const preview = resolveRulePreview(normalizeRuleMatchType(row.match_type), row.pattern || '', row.replacement || '')

  return h('div', { class: 'grid min-w-0 gap-2 py-1' }, [
    h('div', { class: 'rounded-3 bg-[#f8fafc] px-3 py-2' }, [
      h('div', { class: 'text-[12px] leading-4 text-slate' }, '原文'),
      h('div', { class: 'mt-1 break-all text-[13px] leading-5 text-ink font-600' }, formatRulePreviewText(preview.before)),
    ]),
    h('div', { class: 'rounded-3 bg-[#f0fdfa] px-3 py-2' }, [
      h('div', { class: 'text-[12px] leading-4 text-slate' }, '结果'),
      h('div', { class: 'mt-1 break-all text-[13px] leading-5 text-teal-700 font-600' }, formatRulePreviewText(preview.after)),
    ]),
  ])
}

const dictColumns = [
  { title: '词库名称', key: 'name' },
  { title: '领域', key: 'domain' },
  {
    title: '操作',
    key: 'actions',
    width: 220,
    render: (row: DictItem) => h('div', { class: 'flex items-center gap-2' }, [
      row.id === currentDictId.value
        ? h(NTag, {
            size: 'small',
            round: true,
            bordered: false,
            type: 'success',
          }, { default: () => '当前词库' })
        : h(NButton, {
            text: true,
            type: 'primary',
            size: 'small',
            onClick: () => selectDict(row.id),
          }, { default: () => '查看' }),
      h(NButton, {
        text: true,
        size: 'small',
        onClick: () => openEditDictModal(row),
      }, { default: () => '编辑' }),
      h(NButton, {
        text: true,
        type: 'error',
        size: 'small',
        loading: deletingDictId.value === row.id,
        onClick: () => handleDeleteDict(row),
      }, { default: () => '删除' }),
    ]),
  },
]

const entryColumns = [
  { title: '标准术语', key: 'correct_term' },
  {
    title: '误写变体',
    key: 'wrong_variants',
    render: (row: EntryItem) => row.wrong_variants.join(' / ') || '-',
  },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row: EntryItem) => h('div', { class: 'flex items-center gap-2' }, [
      h(NButton, {
        text: true,
        size: 'small',
        onClick: () => openEditEntryModal(row),
      }, { default: () => '编辑' }),
      h(NButton, {
        text: true,
        type: 'error',
        size: 'small',
        loading: deletingEntryId.value === row.id,
        onClick: () => handleDeleteEntry(row),
      }, { default: () => '删除' }),
    ]),
  },
]

const ruleColumns = [
  { title: '纠错方式', key: 'match_type', width: 140, render: (row: RuleItem) => ruleMatchLabel(row.match_type) },
  { title: '预览效果', key: 'preview', minWidth: 360, render: (row: RuleItem) => renderRulePreview(row) },
  { title: '优先级', key: 'priority', width: 96, render: (row: RuleItem) => row.priority || row.sort_order || 100 },
  {
    title: '状态',
    key: 'enabled',
    width: 96,
    render: (row: RuleItem) => h(NTag, {
      size: 'small',
      round: true,
      bordered: false,
      type: row.enabled ? 'success' : 'default',
    }, { default: () => row.enabled ? '启用' : '停用' }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 140,
    render: (row: RuleItem) => {
      const actions = []
      if (row.match_type !== 'literal') {
        actions.push(h(NButton, {
          text: true,
          size: 'small',
          onClick: () => openEditRuleModal(row),
        }, { default: () => '编辑' }))
      }
      actions.push(h(NButton, {
        text: true,
        type: 'error',
        size: 'small',
        loading: deletingRuleId.value === row.id,
        onClick: () => handleDeleteRule(row),
      }, { default: () => '删除' }))
      return h('div', { class: 'flex items-center gap-2' }, actions)
    },
  },
]

function resetDictForm() {
  editingDictId.value = null
  dictForm.name = ''
  dictForm.domain = ''
}

function resetEntryForm() {
  editingEntryId.value = null
  entryForm.correctTerm = ''
  entryForm.wrongVariantsText = ''
}

function resetRuleForm() {
  editingRuleId.value = null
  ruleForm.matchType = 'regex'
  ruleForm.pattern = ''
  ruleForm.replacement = ''
  ruleForm.sortOrder = 100
}

function applyRuleExample(example: RuleExample) {
  if (ruleForm.matchType !== 'regex')
    return

  ruleForm.pattern = example.pattern || ''
  ruleForm.replacement = example.replacement || ''
}

async function loadDicts() {
  loading.value = true
  try {
    const result = await getTermDicts({ offset: 0, limit: 100 })
    dicts.value = result.data.items

    if (dicts.value.length === 0) {
      currentDictId.value = null
      entries.value = []
      rules.value = []
      return
    }

    const nextDictId = currentDictId.value && dicts.value.some(item => item.id === currentDictId.value)
      ? currentDictId.value
      : dicts.value[0].id

    await selectDict(nextDictId)
  }
  catch {
    message.error('术语词典加载失败')
  }
  finally {
    loading.value = false
  }
}

async function selectDict(dictId: number) {
  currentDictId.value = dictId
  await Promise.all([loadEntries(dictId), loadRules(dictId)])
}

async function loadEntries(dictId = currentDictId.value) {
  if (!dictId) {
    entries.value = []
    return
  }

  entryLoading.value = true
  try {
    const result = await getTermEntries(dictId)
    entries.value = result.data
  }
  catch {
    message.error('词条加载失败')
  }
  finally {
    entryLoading.value = false
  }
}

function openImportModal() {
  if (!currentDictId.value) {
    message.warning('请先选择词库')
    return
  }
  lastImportResult.value = null
  showImportModal.value = true
}

function chooseImportFile() {
  importFileInput.value?.click()
}

async function handleImportFileSelected(event: Event) {
  const file = (event.target as HTMLInputElement | null)?.files?.[0]
  if (!file || !currentDictId.value)
    return
  if (file.size > 5 * 1024 * 1024) {
    message.warning('导入文件不能超过 5MB')
    return
  }
  const formData = new FormData()
  formData.append('file', file)
  importingEntries.value = true
  try {
    const result = await importTermEntries(currentDictId.value, formData)
    lastImportResult.value = result.data
    message.success(`已导入 ${result.data.imported} 条词条`)
    await loadEntries(currentDictId.value)
  }
  catch (error) {
    const responseMessage = (error as { response?: { data?: { message?: string } } })?.response?.data?.message
    message.error(responseMessage || '词条导入失败，请确认文件格式为 CSV/TSV/TXT/XLSX')
  }
  finally {
    importingEntries.value = false
    if (importFileInput.value)
      importFileInput.value.value = ''
  }
}

function downloadImportTemplate() {
  const blob = new Blob(['correct_term,wrong_variants\n示例标准词,错误写法1|错误写法2\n'], { type: 'text/csv;charset=utf-8' })
  const url = URL.createObjectURL(blob)
  const anchor = document.createElement('a')
  anchor.href = url
  anchor.download = 'terminology-import-template.csv'
  document.body.appendChild(anchor)
  anchor.click()
  anchor.remove()
  URL.revokeObjectURL(url)
}

async function loadRules(dictId = currentDictId.value) {
  if (!dictId) {
    rules.value = []
    return
  }

  ruleLoading.value = true
  try {
    const result = await getTermRules(dictId)
    rules.value = result.data
  }
  catch {
    message.error('规则加载失败')
  }
  finally {
    ruleLoading.value = false
  }
}

function openCreateDictModal() {
  resetDictForm()
  showDictModal.value = true
}

function openEditDictModal(row: DictItem) {
  editingDictId.value = row.id
  dictForm.name = row.name
  dictForm.domain = row.domain
  showDictModal.value = true
}

function openCreateEntryModal() {
  resetEntryForm()
  showEntryModal.value = true
}

function openCreateRuleModal() {
  resetRuleForm()
  showRuleModal.value = true
}

function openEditEntryModal(row: EntryItem) {
  editingEntryId.value = row.id
  entryForm.correctTerm = row.correct_term
  entryForm.wrongVariantsText = row.wrong_variants.join('\n')
  showEntryModal.value = true
}

function openEditRuleModal(row: RuleItem) {
  editingRuleId.value = row.id
  ruleForm.matchType = normalizeRuleMatchType(row.match_type)
  ruleForm.pattern = row.pattern || ''
  ruleForm.replacement = row.replacement || ''
  ruleForm.sortOrder = row.sort_order || 100
  showRuleModal.value = true
}

async function handleSubmitDict() {
  if (!dictForm.name.trim() || !dictForm.domain.trim()) {
    message.warning('请填写词库名称和领域')
    return
  }

  dictSaving.value = true
  try {
    const payload = {
      name: dictForm.name.trim(),
      domain: dictForm.domain.trim(),
    }

    if (editingDictId.value) {
      await updateTermDict(editingDictId.value, payload)
      message.success('词库更新成功')
    }
    else {
      await createTermDict(payload)
      message.success('词库创建成功')
    }

    showDictModal.value = false
    resetDictForm()
    await loadDicts()
  }
  catch {
    message.error(editingDictId.value ? '词库更新失败' : '词库创建失败')
  }
  finally {
    dictSaving.value = false
  }
}

async function handleDeleteDict(row: DictItem) {
  const confirmed = await confirmDelete({
    entityType: '词库',
    entityName: row.name,
    description: '删除后，该词库下的全部词条与纠错规则会一并删除，且无法恢复。',
  })
  if (!confirmed)
    return

  deletingDictId.value = row.id
  try {
    await deleteTermDict(row.id)
    message.success('词库已删除')
    if (currentDictId.value === row.id) {
      currentDictId.value = null
      entries.value = []
      rules.value = []
    }
    await loadDicts()
  }
  catch {
    message.error('词库删除失败')
  }
  finally {
    deletingDictId.value = null
  }
}

async function handleSubmitEntry() {
  if (!currentDictId.value) {
    message.warning('请先选择词库')
    return
  }

  if (!entryForm.correctTerm.trim()) {
    message.warning('请填写标准术语')
    return
  }

  const wrongVariants = entryForm.wrongVariantsText
    .split(/[\n,，]/)
    .map(item => item.trim())
    .filter(Boolean)

  entrySaving.value = true
  try {
    const payload = {
      correct_term: entryForm.correctTerm.trim(),
      wrong_variants: wrongVariants,
    }

    if (editingEntryId.value) {
      await updateTermEntry(currentDictId.value, editingEntryId.value, payload)
      message.success('词条更新成功')
    }
    else {
      await createTermEntry(currentDictId.value, payload)
      message.success('词条创建成功')
    }

    showEntryModal.value = false
    resetEntryForm()
    await selectDict(currentDictId.value)
  }
  catch {
    message.error(editingEntryId.value ? '词条更新失败' : '词条创建失败')
  }
  finally {
    entrySaving.value = false
  }
}

async function handleSubmitRule() {
  if (!currentDictId.value) {
    message.warning('请先选择词库')
    return
  }

  if (ruleForm.matchType === 'regex' && !ruleForm.pattern.trim()) {
    message.warning('请填写高级匹配式')
    return
  }

  if (ruleForm.matchType === 'regex' && !ruleForm.replacement.trim()) {
    message.warning('请填写替换结果')
    return
  }

  ruleSaving.value = true
  try {
    const payload = {
      match_type: ruleForm.matchType,
      pattern: ruleForm.matchType === 'number_normalize' ? '' : ruleForm.pattern.trim(),
      replacement: ruleForm.matchType === 'number_normalize' ? '' : ruleForm.replacement.trim(),
      enabled: true,
      sort_order: ruleForm.sortOrder || 100,
      priority: ruleForm.sortOrder || 100,
    }

    if (editingRuleId.value) {
      await updateTermRule(currentDictId.value, editingRuleId.value, payload)
      message.success('规则更新成功')
    }
    else {
      await createTermRule(currentDictId.value, payload)
      message.success('规则创建成功')
    }

    showRuleModal.value = false
    resetRuleForm()
    await loadRules(currentDictId.value)
  }
  catch {
    message.error(editingRuleId.value ? '规则更新失败' : '规则创建失败')
  }
  finally {
    ruleSaving.value = false
  }
}

async function handleDeleteRule(row: RuleItem) {
  if (!currentDictId.value)
    return

  const confirmed = await confirmDelete({
    entityType: '纠错规则',
    entityName: row.pattern || ruleMatchLabel(row.match_type),
    description: '删除后，这条规则将不再参与术语纠错链路。',
  })
  if (!confirmed)
    return

  deletingRuleId.value = row.id
  try {
    await deleteTermRule(currentDictId.value, row.id)
    message.success('规则已删除')
    await loadRules(currentDictId.value)
  }
  catch {
    message.error('规则删除失败')
  }
  finally {
    deletingRuleId.value = null
  }
}

async function handleDeleteEntry(row: EntryItem) {
  if (!currentDictId.value)
    return

  const confirmed = await confirmDelete({
    entityType: '词条',
    entityName: row.correct_term,
    description: '删除后，该术语的误写变体映射会失效。',
  })
  if (!confirmed)
    return

  deletingEntryId.value = row.id
  try {
    await deleteTermEntry(currentDictId.value, row.id)
    message.success('词条已删除')
    await selectDict(currentDictId.value)
  }
  catch {
    message.error('词条删除失败')
  }
  finally {
    deletingEntryId.value = null
  }
}

onMounted(loadDicts)
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 gap-5">
    <div class="grid grid-cols-1 gap-5 xl:grid-cols-[0.9fr_1.1fr] flex-1 min-h-0">
      <NCard class="card-main flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <span class="text-sm font-600">词典列表</span>
            <div class="flex flex-wrap items-center gap-2">
              <NButton quaternary size="small" @click="loadDicts">
                刷新
              </NButton>
              <NButton size="small" type="primary" color="#0f766e" @click="openCreateDictModal">
                新建词库
              </NButton>
            </div>
          </div>
        </template>
        <NDataTable flex-height class="flex-1 min-h-0" :columns="dictColumns" :data="dicts" :loading="loading" :pagination="false" size="small" />
      </NCard>

      <NCard class="card-main flex flex-col min-h-0" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <span class="text-sm font-600">{{ currentDict ? currentDict.name : '词库详情' }}</span>
              <NTag v-if="currentDict" round type="info" size="small">
                {{ currentDict.domain }}
              </NTag>
            </div>
          </div>
        </template>

        <NTabs v-model:value="detailTab" class="flex-1 min-h-0 detail-tabs" pane-class="min-h-0 flex-1 flex flex-col">
          <NTabPane name="entries" tab="术语词条">
            <div class="mb-3 flex justify-end gap-2 shrink-0">
              <NButton :disabled="!currentDictId" quaternary size="small" @click="currentDictId && loadEntries(currentDictId)">
                刷新
              </NButton>
              <NButton :disabled="!currentDictId" quaternary size="small" @click="openImportModal">
                批量导入
              </NButton>
              <NButton :disabled="!currentDictId" quaternary size="small" @click="openCreateEntryModal">
                新增词条
              </NButton>
            </div>
            <NDataTable flex-height class="flex-1 min-h-0" :columns="entryColumns" :data="entries" :loading="entryLoading" :pagination="{ pageSize: 8 }" size="small" />
          </NTabPane>
          <NTabPane name="rules" tab="纠错规则">
            <div v-if="ruleConflictWarnings.length" class="mb-3 grid gap-2 shrink-0">
              <NAlert v-for="warning in ruleConflictWarnings" :key="warning" type="warning" :show-icon="false">
                {{ warning }}
              </NAlert>
            </div>
            <div class="mb-3 flex justify-end gap-2 shrink-0">
              <NButton :disabled="!currentDictId" quaternary size="small" @click="currentDictId && loadRules(currentDictId)">
                刷新
              </NButton>
              <NButton :disabled="!currentDictId" quaternary size="small" @click="openCreateRuleModal">
                新增规则
              </NButton>
            </div>
            <NDataTable flex-height class="flex-1 min-h-0" :columns="ruleColumns" :data="rules" :loading="ruleLoading" :pagination="{ pageSize: 8 }" size="small" />
          </NTabPane>
        </NTabs>
      </NCard>
    </div>

    <NModal v-model:show="showDictModal" preset="card" :title="dictModalTitle" class="modal-card max-w-140">
      <NForm :model="dictForm" label-placement="top">
        <NFormItem label="词库名称">
          <NInput v-model:value="dictForm.name" placeholder="如：医疗查房" />
        </NFormItem>
        <NFormItem label="领域">
          <NInput v-model:value="dictForm.domain" placeholder="如：医疗" />
        </NFormItem>
        <div class="modal-footer-row">
          <NButton @click="showDictModal = false">
            取消
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="dictSaving" @click="handleSubmitDict">
            {{ editingDictId ? '保存' : '创建' }}
          </NButton>
        </div>
      </NForm>
    </NModal>

    <NModal v-model:show="showImportModal" preset="card" title="批量导入词条" class="modal-card max-w-140">
      <div class="grid gap-4 text-sm leading-7 text-slate">
        <NAlert type="info" :show-icon="false">
          支持 CSV/TSV/TXT/XLSX 文件，单次最多 5000 行且文件不超过 5MB。表头可使用 correct_term 与 wrong_variants；已存在或本次重复的标准词会跳过。
        </NAlert>
        <input ref="importFileInput" type="file" accept=".csv,.tsv,.txt,.xlsx" class="hidden" @change="handleImportFileSelected">
        <div v-if="lastImportResult" class="rounded-2 bg-mist/70 px-3 py-2 text-xs text-slate">
          已导入 {{ lastImportResult.imported }} 条，跳过 {{ lastImportResult.skipped }} 条。
        </div>
        <div class="modal-footer-row">
          <NButton @click="downloadImportTemplate">
            下载模板
          </NButton>
          <NButton @click="showImportModal = false">
            关闭
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="importingEntries" @click="chooseImportFile">
            选择文件导入
          </NButton>
        </div>
      </div>
    </NModal>

    <NModal v-model:show="showEntryModal" preset="card" :title="entryModalTitle" class="modal-card max-w-160">
      <NForm :model="entryForm" label-placement="top">
        <NFormItem label="所属词库">
          <NInput :value="currentDict?.name || ''" disabled />
        </NFormItem>
        <NFormItem label="标准术语">
          <NInput v-model:value="entryForm.correctTerm" placeholder="如：冠状动脉" />
        </NFormItem>
        <NFormItem label="误写变体">
          <NInput
            v-model:value="entryForm.wrongVariantsText"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }"
            placeholder="每行一个，或使用逗号分隔"
          />
        </NFormItem>
        <div class="modal-footer-row">
          <NButton @click="showEntryModal = false">
            取消
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="entrySaving" @click="handleSubmitEntry">
            {{ editingEntryId ? '保存' : '创建' }}
          </NButton>
        </div>
      </NForm>
    </NModal>

    <NModal v-model:show="showRuleModal" preset="card" :title="ruleModalTitle" class="modal-card rule-modal max-w-190">
      <NForm :model="ruleForm" label-placement="top" class="rule-form">
        <NFormItem label="所属词库">
          <NInput :value="currentDict?.name || ''" disabled />
        </NFormItem>

        <div class="rule-guide">
          <div class="rule-guide-head">
            <div>
              <div class="rule-guide-title">
                {{ currentRuleMatch.label }}
              </div>
              <div class="rule-guide-summary">
                {{ currentRuleMatch.summary }}
              </div>
            </div>
            <span class="rule-type-badge">{{ currentRuleMatch.badge }}</span>
          </div>
          <div class="rule-guide-text">
            {{ currentRuleMatch.detail }}
          </div>
        </div>

        <template v-if="ruleForm.matchType === 'regex'">
          <div class="rule-input-grid">
            <NFormItem :show-feedback="false" :label="currentRuleMatch.patternLabel">
              <NInput v-model:value="ruleForm.pattern" :placeholder="currentRuleMatch.patternPlaceholder" />
              <div class="rule-field-tip">
                这项用于复杂格式，现场不确定时请使用下方示例或联系技术支持。
              </div>
            </NFormItem>
            <NFormItem :show-feedback="false" :label="currentRuleMatch.replacementLabel">
              <NInput v-model:value="ruleForm.replacement" :placeholder="currentRuleMatch.replacementPlaceholder" />
              <div class="rule-field-tip">
                可使用 $1、$2 这类占位结果，请按技术人员提供的内容填写。
              </div>
            </NFormItem>
          </div>
        </template>

        <div v-if="currentRuleExamples.length" class="rule-examples">
          <div class="rule-section-title">
            示例
          </div>
          <div class="rule-example-list">
            <button
              v-for="example in currentRuleExamples"
              :key="example.title"
              type="button"
              class="rule-example-card"
              :disabled="ruleForm.matchType !== 'regex'"
              @click="applyRuleExample(example)"
            >
              <span class="rule-example-title">{{ example.title }}</span>
              <span class="rule-example-line"><span>原文</span>{{ example.before }}</span>
              <span class="rule-example-line"><span>结果</span>{{ example.after }}</span>
            </button>
          </div>
        </div>

        <div class="rule-preview">
          <div class="rule-section-title">
            效果预览
          </div>
          <div class="rule-preview-lines">
            <div class="rule-preview-line">
              <span>原文</span>
              <strong>{{ formatRulePreviewText(rulePreview.before) }}</strong>
            </div>
            <div class="rule-preview-line is-result">
              <span>结果</span>
              <strong>{{ formatRulePreviewText(rulePreview.after) }}</strong>
            </div>
          </div>
          <div class="rule-preview-hint">
            {{ rulePreview.hint }}
          </div>
        </div>

        <div class="rule-settings">
          <NFormItem :show-feedback="false" label="执行顺序" class="rule-order-item">
            <NInputNumber v-model:value="ruleForm.sortOrder" :min="1" :step="10" class="w-full" />
            <div class="rule-field-tip">
              数字越小越先执行，一般保持 100 即可。
            </div>
          </NFormItem>
        </div>

        <div class="modal-footer-row">
          <NButton @click="showRuleModal = false">
            取消
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="ruleSaving" @click="handleSubmitRule">
            {{ editingRuleId ? '保存' : '创建' }}
          </NButton>
        </div>
      </NForm>
    </NModal>
  </div>
</template>

<style scoped>
.detail-tabs :deep(.n-tabs-content),
.detail-tabs :deep(.n-tab-pane) {
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.rule-modal :deep(.n-card__content) {
  max-height: min(82vh, 720px);
  overflow: auto;
}

.rule-modal {
  width: min(94vw, 880px);
}

.rule-modal :deep(.modal-footer-row) {
  position: sticky;
  bottom: -18px;
  margin: 8px -20px -18px;
  padding: 14px 24px;
  background: rgba(255, 255, 255, 0.96);
  backdrop-filter: blur(8px);
}

.rule-form {
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.rule-guide-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}

.rule-type-badge {
  flex-shrink: 0;
  border-radius: 999px;
  background: rgba(15, 118, 110, 0.1);
  padding: 2px 8px;
  color: #0f766e;
  font-size: 12px;
  line-height: 18px;
}

.rule-guide,
.rule-preview {
  border: 1px solid rgba(15, 118, 110, 0.14);
  border-radius: 8px;
  background: #f8fbfa;
  padding: 12px 14px;
}

.rule-guide-title,
.rule-section-title {
  color: #1f2937;
  font-size: 14px;
  font-weight: 600;
  line-height: 20px;
}

.rule-guide-summary {
  margin-top: 2px;
  color: #334155;
  font-size: 13px;
  line-height: 20px;
}

.rule-guide-text,
.rule-preview-hint,
.rule-field-tip {
  margin-top: 4px;
  color: #64748b;
  font-size: 12px;
  line-height: 18px;
}

.rule-input-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px 16px;
}

.rule-input-grid :deep(.n-form-item-blank) {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
  min-height: 0;
}

.rule-input-grid :deep(.n-form-item-blank > .n-input),
.rule-input-grid :deep(.n-form-item-blank > .n-input-number) {
  width: 100%;
}

.rule-settings {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 4px 16px;
  align-items: start;
}

.rule-settings :deep(.n-form-item-blank) {
  display: flex;
  flex-direction: column;
  align-items: stretch;
  gap: 6px;
}

.rule-examples {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rule-example-list {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.rule-example-card {
  display: flex;
  flex-direction: column;
  min-height: 116px;
  border: 1px solid rgba(15, 23, 42, 0.1);
  border-radius: 8px;
  background: #fff;
  padding: 10px 12px;
  color: #334155;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.16s ease, background 0.16s ease;
}

.rule-example-card:hover:not(:disabled) {
  border-color: rgba(15, 118, 110, 0.46);
  background: #f7fbfa;
}

.rule-example-card:disabled {
  cursor: default;
}

.rule-example-title {
  display: block;
  margin-bottom: 8px;
  color: #0f172a;
  font-size: 13px;
  font-weight: 600;
  line-height: 18px;
}

.rule-example-line {
  display: block;
  color: #475569;
  font-size: 12px;
  line-height: 18px;
  word-break: break-word;
}

.rule-example-line span {
  display: inline-block;
  min-width: 32px;
  color: #94a3b8;
}

.rule-preview {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.rule-preview-lines {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.rule-preview-line {
  min-height: 58px;
  border-radius: 8px;
  background: #fff;
  padding: 10px 12px;
}

.rule-preview-line span {
  display: block;
  color: #94a3b8;
  font-size: 12px;
  line-height: 16px;
}

.rule-preview-line strong {
  display: block;
  margin-top: 4px;
  color: #334155;
  font-size: 13px;
  font-weight: 600;
  line-height: 20px;
  word-break: break-word;
}

.rule-preview-line.is-result strong {
  color: #0f766e;
}

@media (max-width: 1080px) {
  .rule-input-grid,
  .rule-settings,
  .rule-preview-lines {
    grid-template-columns: 1fr;
  }
}

@media (max-width: 720px) {
  .rule-example-list,
  .rule-input-grid,
  .rule-settings,
  .rule-preview-lines {
    grid-template-columns: 1fr;
  }
}
</style>

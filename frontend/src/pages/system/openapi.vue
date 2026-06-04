<script setup lang="ts">
import type { AxiosError } from 'axios'
import type { DataTableColumns } from 'naive-ui'
import type { OpenPlatformApp, OpenPlatformCallLog, OpenPlatformCapability, OpenPlatformCreateAppPayload, OpenPlatformCreateAppResponse } from '@/api/openplatform'

import MarkdownIt from 'markdown-it'
import { NButton, NTag, NTooltip, useMessage } from 'naive-ui'
import { computed, h, nextTick, reactive, ref, watch } from 'vue'

import {
  createOpenPlatformApp,
  disableOpenPlatformApp,
  enableOpenPlatformApp,
  getOpenPlatformAppCalls,
  getOpenPlatformApps,
  getOpenPlatformCapabilities,
  getOpenPlatformDocs,
  revokeOpenPlatformApp,
  rotateOpenPlatformAppSecret,
  updateOpenPlatformApp,
} from '@/api/openplatform'
import { useConfirmActionDialog } from '@/composables/useConfirmActionDialog'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'
import { useUserStore } from '@/stores/user'
import { WORKFLOW_TYPES } from '@/types/workflow'

const userStore = useUserStore()
const message = useMessage()
const confirmAction = useConfirmActionDialog()

const batchCatalog = useWorkflowCatalog(WORKFLOW_TYPES.BATCH)
const realtimeCatalog = useWorkflowCatalog(WORKFLOW_TYPES.REALTIME)
const meetingCatalog = useWorkflowCatalog(WORKFLOW_TYPES.MEETING)
const voiceCatalog = useWorkflowCatalog(WORKFLOW_TYPES.VOICE_CONTROL)

const isAdmin = computed(() => userStore.profile?.role === 'admin')
const initializing = ref(false)
const loading = ref(false)
const saving = ref(false)
const keyword = ref('')
const statusFilter = ref<'all' | OpenPlatformApp['status']>('all')
const capabilityFilter = ref('all')
const activeTab = ref('apps')
const docsLoading = ref(false)
const docsContent = ref('')
const docsCapabilityFilter = ref('all')
const docsKeyword = ref('')

const docsError = ref('')
const docsContainerRef = ref<HTMLElement | null>(null)

const apps = ref<OpenPlatformApp[]>([])
const capabilities = ref<OpenPlatformCapability[]>([])
const formVisible = ref(false)
const editingAppId = ref<number | null>(null)

const callsVisible = ref(false)
const callsLoading = ref(false)
const callsTitle = ref('')
const callLogs = ref<OpenPlatformCallLog[]>([])
const appPagination = reactive({
  page: 1,
  pageSize: 10,
})

const form = reactive({
  name: '',
  description: '',
  allowed_caps: [] as string[],
  rate_limit_per_sec: 30 as number | null,
  callback_whitelist_text: '',
  meta_json: '',
  default_workflow_asr_recognize: null as number | null,
  default_workflow_nlp_correct: null as number | null,
  default_workflow_asr_stream: null as number | null,
  default_workflow_meeting_summary: null as number | null,
  default_workflow_skill_invoke: null as number | null,
})

const secretDialog = reactive({
  visible: false,
  actionLabel: '创建应用',
  appName: '',
  appId: '',
  appSecret: '',
  secretVersion: 0,
})

function extractErrorMessage(error: unknown, fallback: string) {
  const responseMessage = (error as AxiosError<{ message?: string }>)?.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim())
    return responseMessage
  return fallback
}

function formatDateTime(value?: string) {
  if (!value)
    return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function parseMultilineList(raw: string) {
  return raw
    .split(/[\n,]/)
    .map(item => item.trim())
    .filter(Boolean)
}

function hasCapability(capability: string) {
  return form.allowed_caps.includes(capability)
}

function statusMeta(status: OpenPlatformApp['status']) {
  switch (status) {
    case 'active':
      return { label: '启用', type: 'success' as const }
    case 'disabled':
      return { label: '停用', type: 'warning' as const }
    default:
      return { label: '已撤销', type: 'error' as const }
  }
}

const capabilityMap = computed(() => {
  const map = new Map<string, OpenPlatformCapability>()
  for (const item of capabilities.value)
    map.set(item.id, item)
  return map
})

const activeCount = computed(() => apps.value.filter(item => item.status === 'active').length)
const disabledCount = computed(() => apps.value.filter(item => item.status === 'disabled').length)
const revokedCount = computed(() => apps.value.filter(item => item.status === 'revoked').length)
const selectedCapabilityCount = computed(() => form.allowed_caps.length)
const modalTitle = computed(() => editingAppId.value ? '编辑 OpenAPI 应用' : '新增 OpenAPI 应用')
const markdown = new MarkdownIt({ html: false, linkify: true, breaks: false })

const statusRank: Record<OpenPlatformApp['status'], number> = {
  active: 0,
  disabled: 1,
  revoked: 2,
}

const sortedApps = computed(() => [...apps.value].sort((a, b) => {
  const statusDiff = statusRank[a.status] - statusRank[b.status]
  if (statusDiff !== 0)
    return statusDiff

  const updatedA = Date.parse(a.updated_at || a.created_at || '')
  const updatedB = Date.parse(b.updated_at || b.created_at || '')
  const timeA = Number.isNaN(updatedA) ? 0 : updatedA
  const timeB = Number.isNaN(updatedB) ? 0 : updatedB
  if (timeA !== timeB)
    return timeB - timeA

  return b.id - a.id
}))

const capabilityCounts = computed(() => {
  const counts = new Map<string, number>()
  for (const item of apps.value) {
    for (const capability of item.allowed_caps)
      counts.set(capability, (counts.get(capability) || 0) + 1)
  }
  return counts
})

const statusFilterOptions = computed(() => [
  { label: '全部应用', value: 'all' as const, count: apps.value.length, type: 'default' as const },
  { label: '启用中', value: 'active' as const, count: activeCount.value, type: 'success' as const },
  { label: '已停用', value: 'disabled' as const, count: disabledCount.value, type: 'warning' as const },
  { label: '已撤销', value: 'revoked' as const, count: revokedCount.value, type: 'error' as const },
])

const capabilityFilterOptions = computed(() => [
  { label: '全部能力', value: 'all', count: apps.value.length },
  ...capabilities.value
    .map(item => ({ label: item.display_name || item.id, value: item.id, count: capabilityCounts.value.get(item.id) || 0 }))
    .filter(item => item.count > 0),
])

const defaultHeadingOpen = markdown.renderer.rules.heading_open || ((tokens, idx, options, env, self) => self.renderToken(tokens, idx, options))
markdown.renderer.rules.heading_open = (tokens, idx, options, env, self) => {
  const token = tokens[idx]
  const inline = tokens[idx + 1]
  const text = inline?.content || ''
  const used = (env.slugCounts ||= new Map<string, number>())
  const base = slugify(text)
  const count = used.get(base) || 0
  used.set(base, count + 1)
  token.attrSet('id', count === 0 ? base : `${base}-${count}`)
  return defaultHeadingOpen(tokens, idx, options, env, self)
}

const defaultFence = markdown.renderer.rules.fence || ((tokens, idx, options, env, self) => self.renderToken(tokens, idx, options))
markdown.renderer.rules.fence = (tokens, idx, options, env, self) => {
  const original = defaultFence(tokens, idx, options, env, self)
  return `<div class="docs-code"><button class="docs-code-copy" data-copy type="button">复制</button>${original}</div>`
}

// eslint-disable-next-line regexp/no-obscure-range
const CJK_CHAR_RE = /[一-龥]/
function slugify(text: string) {
  const out: string[] = []
  let lastDash = false
  for (const ch of text.toLowerCase().trim()) {
    if (/[a-z0-9_]/.test(ch) || CJK_CHAR_RE.test(ch)) {
      out.push(ch)
      lastDash = false
    }
    else if (!lastDash) {
      out.push('-')
      lastDash = true
    }
  }
  return out.join('').replace(/^-|-$/g, '') || 'section'
}

const docsCapabilityOptions = computed(() => [
  { label: '全部能力', value: 'all' },
  ...capabilities.value.map(item => ({ label: item.display_name || item.id, value: item.id })),
])

const docsSections = computed(() => docsContent.value.trim().split(/\n(?=#{1,3}\s)/g))

const filteredDocsContent = computed(() => {
  const content = docsContent.value.trim()
  const keyword = docsKeyword.value.trim().toLowerCase()
  const selectedCapability = docsCapabilityFilter.value === 'all' ? null : capabilityMap.value.get(docsCapabilityFilter.value)
  if (!content || (!keyword && !selectedCapability))
    return content

  const capabilityTerms = selectedCapability
    ? [selectedCapability.id, selectedCapability.display_name, selectedCapability.description].filter(Boolean).map(item => String(item).toLowerCase())
    : []
  const matched = docsSections.value.filter((section) => {
    const haystack = section.toLowerCase()
    const matchesCapability = capabilityTerms.length === 0 || capabilityTerms.some(term => haystack.includes(term))
    const matchesKeyword = !keyword || haystack.includes(keyword)
    return matchesCapability && matchesKeyword
  })
  return matched.join('\n\n')
})

const docsHasFilter = computed(() => docsKeyword.value.trim() !== '' || docsCapabilityFilter.value !== 'all')

const docsRender = computed(() => {
  const content = filteredDocsContent.value
  if (!content) {
    return { html: '', toc: [] as { id: string, level: number, text: string }[] }
  }
  const html = markdown.render(content, {})
  if (typeof window === 'undefined' || typeof DOMParser === 'undefined')
    return { html, toc: [] as { id: string, level: number, text: string }[] }
  const doc = new DOMParser().parseFromString(`<root>${html}</root>`, 'text/html')
  const toc: { id: string, level: number, text: string }[] = []
  doc.querySelectorAll('h2[id], h3[id]').forEach((el) => {
    toc.push({
      id: (el as HTMLElement).id,
      level: el.tagName === 'H2' ? 2 : 3,
      text: (el.textContent || '').trim(),
    })
  })
  return { html, toc }
})

const docsMatchCount = computed(() => {
  if (!filteredDocsContent.value)
    return 0
  return filteredDocsContent.value.split(/\n(?=#{1,3}\s)/g).filter(Boolean).length
})

const filteredApps = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  return sortedApps.value.filter((item) => {
    if (statusFilter.value !== 'all' && item.status !== statusFilter.value)
      return false
    if (capabilityFilter.value !== 'all' && !item.allowed_caps.includes(capabilityFilter.value))
      return false
    if (!value)
      return true
    const caps = item.allowed_caps.map(capabilityLabel).join(' ')
    return item.name.toLowerCase().includes(value)
      || item.app_id.toLowerCase().includes(value)
      || (item.description || '').toLowerCase().includes(value)
      || item.status.toLowerCase().includes(value)
      || statusMeta(item.status).label.includes(value)
      || caps.toLowerCase().includes(value)
  })
})

function capabilityLabel(capability: string) {
  return capabilityMap.value.get(capability)?.display_name || capability
}

function compactCapabilityLabels(capabilities: string[], maxVisible = 2) {
  const labels = capabilities.map(capabilityLabel)
  return {
    visible: labels.slice(0, maxVisible),
    hidden: labels.slice(maxVisible),
  }
}

function appTooltipContent(row: OpenPlatformApp) {
  return h('div', { class: 'openapi-app-tooltip grid gap-1' }, [
    h('div', { class: 'font-600' }, row.name),
    h('div', { class: 'text-xs opacity-80' }, row.app_id),
    row.description ? h('div', { class: 'text-xs opacity-80' }, row.description) : null,
  ])
}

function workflowLabel(capability: string, workflowId?: number) {
  if (!workflowId)
    return '-'
  switch (capability) {
    case 'asr.recognize':
    case 'nlp.correct':
      return batchCatalog.labelForWorkflow(workflowId)
    case 'asr.stream':
      return realtimeCatalog.labelForWorkflow(workflowId)
    case 'meeting.summary':
      return meetingCatalog.labelForWorkflow(workflowId)
    case 'skill.invoke':
      return voiceCatalog.labelForWorkflow(workflowId)
    default:
      return `工作流 #${workflowId}`
  }
}

function defaultWorkflowLabels(item: OpenPlatformApp) {
  return Object.entries(item.default_workflows || {}).map(([capability, workflowId]) => `${capabilityLabel(capability)}: ${workflowLabel(capability, workflowId)}`)
}

function resetForm() {
  editingAppId.value = null
  form.name = ''
  form.description = ''
  form.allowed_caps = []
  form.rate_limit_per_sec = 30
  form.callback_whitelist_text = ''
  form.meta_json = ''
  form.default_workflow_asr_recognize = null
  form.default_workflow_nlp_correct = null
  form.default_workflow_asr_stream = null
  form.default_workflow_meeting_summary = null
  form.default_workflow_skill_invoke = null
}

function fillForm(item: OpenPlatformApp) {
  editingAppId.value = item.id
  form.name = item.name
  form.description = item.description || ''
  form.allowed_caps = [...item.allowed_caps]
  form.rate_limit_per_sec = item.rate_limit_per_sec || 30
  form.callback_whitelist_text = (item.callback_whitelist || []).join('\n')
  form.meta_json = ''
  form.default_workflow_asr_recognize = item.default_workflows?.['asr.recognize'] || null
  form.default_workflow_nlp_correct = item.default_workflows?.['nlp.correct'] || null
  form.default_workflow_asr_stream = item.default_workflows?.['asr.stream'] || null
  form.default_workflow_meeting_summary = item.default_workflows?.['meeting.summary'] || null
  form.default_workflow_skill_invoke = item.default_workflows?.['skill.invoke'] || null
}

function buildPayload(): OpenPlatformCreateAppPayload {
  const defaultWorkflows: Record<string, number> = {}
  if (hasCapability('asr.recognize') && form.default_workflow_asr_recognize)
    defaultWorkflows['asr.recognize'] = form.default_workflow_asr_recognize
  if (hasCapability('nlp.correct') && form.default_workflow_nlp_correct)
    defaultWorkflows['nlp.correct'] = form.default_workflow_nlp_correct
  if (hasCapability('asr.stream') && form.default_workflow_asr_stream)
    defaultWorkflows['asr.stream'] = form.default_workflow_asr_stream
  if (hasCapability('meeting.summary') && form.default_workflow_meeting_summary)
    defaultWorkflows['meeting.summary'] = form.default_workflow_meeting_summary
  if (hasCapability('skill.invoke') && form.default_workflow_skill_invoke)
    defaultWorkflows['skill.invoke'] = form.default_workflow_skill_invoke

  return {
    name: form.name.trim(),
    description: form.description.trim(),
    allowed_caps: [...form.allowed_caps],
    default_workflows: defaultWorkflows,
    callback_whitelist: parseMultilineList(form.callback_whitelist_text),
    rate_limit_per_sec: Math.max(1, Math.round(form.rate_limit_per_sec || 30)),
    meta_json: form.meta_json.trim(),
  }
}

function validatePayload(payload: OpenPlatformCreateAppPayload) {
  if (!payload.name)
    return '请填写应用名称'
  if (payload.allowed_caps.length === 0)
    return '请至少选择一个能力'
  if (payload.allowed_caps.includes('skill.invoke') && (payload.callback_whitelist || []).length === 0)
    return '启用 Skill 回调时必须配置回调白名单'
  return ''
}

function presentSecret(result: OpenPlatformCreateAppResponse, actionLabel: string) {
  secretDialog.visible = true
  secretDialog.actionLabel = actionLabel
  secretDialog.appName = result.name
  secretDialog.appId = result.app_id
  secretDialog.appSecret = result.app_secret
  secretDialog.secretVersion = result.secret_version
}

async function loadCapabilities() {
  const result = await getOpenPlatformCapabilities()
  capabilities.value = result.data.items || []
}

async function loadDocs() {
  docsLoading.value = true
  docsError.value = ''
  try {
    const result = await getOpenPlatformDocs()
    docsContent.value = result.data.content || ''
    if (!docsContent.value.trim())
      docsError.value = '后端未返回文档内容'
  }
  catch (error) {
    const errorMessage = extractErrorMessage(error, 'OpenAPI 对接文档加载失败')
    docsError.value = errorMessage
    message.error(errorMessage)
  }
  finally {
    docsLoading.value = false
  }
}

function resetDocsFilters() {
  docsKeyword.value = ''
  docsCapabilityFilter.value = 'all'
}

function scrollToHeading(id: string) {
  const container = docsContainerRef.value
  if (!container)
    return
  const target = container.querySelector(`#${CSS.escape(id)}`) as HTMLElement | null
  if (target)
    target.scrollIntoView({ behavior: 'smooth', block: 'start' })
}

function handleDocsClick(event: MouseEvent) {
  const node = event.target as HTMLElement | null
  const button = node?.closest('button[data-copy]') as HTMLButtonElement | null
  if (!button)
    return
  const pre = button.parentElement?.querySelector('pre code, pre') as HTMLElement | null
  const text = pre?.textContent || ''
  if (!text.trim())
    return
  void copyText(text, '已复制代码示例')
}

async function copyText(text: string, successMessage: string) {
  if (!text.trim()) {
    message.warning('没有可复制的内容')
    return
  }
  try {
    await navigator.clipboard.writeText(text)
    message.success(successMessage)
  }
  catch {
    message.error('复制失败，请检查浏览器剪贴板权限')
  }
}

function copyDocsContent() {
  void copyText(filteredDocsContent.value, '已复制当前文档内容')
}

function copyAuthExample() {
  const origin = typeof window !== 'undefined' ? window.location.origin : 'https://your-asr-host'
  const sample = [
    `curl -X POST ${origin}/openapi/v1/auth/token \\`,
    `  -H 'Content-Type: application/json' \\`,
    `  -d '{"app_id":"YOUR_APP_ID","app_secret":"YOUR_APP_SECRET"}'`,
  ].join('\n')
  void copyText(sample, '已复制鉴权 curl 示例')
}

async function loadApps() {
  loading.value = true
  try {
    const result = await getOpenPlatformApps({ offset: 0, limit: 200 })
    apps.value = result.data.items || []
  }
  catch (error) {
    message.error(extractErrorMessage(error, 'OpenAPI 应用列表加载失败'))
  }
  finally {
    loading.value = false
  }
}

async function loadWorkflowCatalogs() {
  const results = await Promise.allSettled([
    batchCatalog.loadWorkflows(),
    realtimeCatalog.loadWorkflows(),
    meetingCatalog.loadWorkflows(),
    voiceCatalog.loadWorkflows(),
  ])
  if (results.some(result => result.status === 'rejected'))
    message.warning('部分默认工作流选项加载失败，保存时仍会由后端校验')
}

async function initializePage() {
  if (!isAdmin.value || initializing.value)
    return
  initializing.value = true
  try {
    await Promise.allSettled([
      loadCapabilities(),
      loadApps(),
      loadDocs(),
      loadWorkflowCatalogs(),
    ])
  }
  finally {
    initializing.value = false
  }
}

function openCreateModal() {
  resetForm()
  formVisible.value = true
}

function openEditModal(item: OpenPlatformApp) {
  fillForm(item)
  formVisible.value = true
}

async function handleSubmit() {
  const payload = buildPayload()
  const validationMessage = validatePayload(payload)
  if (validationMessage) {
    message.warning(validationMessage)
    return
  }

  saving.value = true
  try {
    if (editingAppId.value) {
      await updateOpenPlatformApp(editingAppId.value, payload)
      message.success('OpenAPI 应用已更新')
    }
    else {
      const result = await createOpenPlatformApp(payload)
      presentSecret(result.data as OpenPlatformCreateAppResponse, '创建应用')
      message.success('OpenAPI 应用已创建')
    }
    formVisible.value = false
    await loadApps()
  }
  catch (error) {
    message.error(extractErrorMessage(error, editingAppId.value ? 'OpenAPI 应用更新失败' : 'OpenAPI 应用创建失败'))
  }
  finally {
    saving.value = false
  }
}

async function handleRotateSecret(item: OpenPlatformApp) {
  const confirmed = await confirmAction({
    title: '重置应用密钥',
    message: `确认重置 ${item.name} 的 app_secret 吗？`,
    description: '旧密钥签发的 access_token 会全部失效，页面只会再展示一次新的完整密钥。',
    positiveText: '确认重置',
  })
  if (!confirmed)
    return

  try {
    const result = await rotateOpenPlatformAppSecret(item.id)
    presentSecret(result.data as OpenPlatformCreateAppResponse, '重置密钥')
    message.success('应用密钥已重置')
    await loadApps()
  }
  catch (error) {
    message.error(extractErrorMessage(error, '应用密钥重置失败'))
  }
}

async function handleToggleStatus(item: OpenPlatformApp) {
  if (item.status === 'active') {
    const confirmed = await confirmAction({
      title: '停用应用',
      message: `确认停用 ${item.name} 吗？`,
      description: '停用后通过该应用获取的 token 调用任意 OpenAPI 都会被拒绝。',
      positiveText: '确认停用',
    })
    if (!confirmed)
      return
    try {
      await disableOpenPlatformApp(item.id)
      message.success('应用已停用')
      await loadApps()
    }
    catch (error) {
      message.error(extractErrorMessage(error, '应用停用失败'))
    }
    return
  }

  if (item.status === 'disabled') {
    const confirmed = await confirmAction({
      title: '启用应用',
      message: `确认启用 ${item.name} 吗？`,
      description: '启用后应用可以重新签发 token 并恢复接口调用。',
      positiveText: '确认启用',
      positiveType: 'success',
    })
    if (!confirmed)
      return
    try {
      await enableOpenPlatformApp(item.id)
      message.success('应用已启用')
      await loadApps()
    }
    catch (error) {
      message.error(extractErrorMessage(error, '应用启用失败'))
    }
  }
}

async function handleRevoke(item: OpenPlatformApp) {
  const confirmed = await confirmAction({
    title: '撤销应用',
    message: `确认撤销 ${item.name} 吗？`,
    description: '撤销通常用于凭证泄露或应用永久下线，撤销后现有 token 会全部失效。',
    positiveText: '确认撤销',
    positiveType: 'error',
  })
  if (!confirmed)
    return

  try {
    await revokeOpenPlatformApp(item.id)
    message.success('应用已撤销')
    await loadApps()
  }
  catch (error) {
    message.error(extractErrorMessage(error, '应用撤销失败'))
  }
}

async function openCallLogs(item: OpenPlatformApp) {
  callsVisible.value = true
  callsTitle.value = `${item.name} · ${item.app_id}`
  callsLoading.value = true
  try {
    const result = await getOpenPlatformAppCalls(item.id, { limit: 100 })
    callLogs.value = result.data.items || []
  }
  catch (error) {
    callLogs.value = []
    message.error(extractErrorMessage(error, '调用日志加载失败'))
  }
  finally {
    callsLoading.value = false
  }
}

function updateAppPage(page: number) {
  appPagination.page = page
}

function httpStatusType(status: number) {
  if (status < 400)
    return 'success'
  if (status < 500)
    return 'warning'
  return 'error'
}

function logErrorText(value?: string) {
  return value?.trim() || '无错误码'
}

const appColumns = computed<DataTableColumns<OpenPlatformApp>>(() => [
  {
    title: '应用',
    key: 'name',
    width: 248,
    render: row => h(
      NTooltip,
      { placement: 'top-start', delay: 300 },
      {
        trigger: () => h('div', { class: 'openapi-app-cell min-w-0' }, [
          h('div', { class: 'truncate text-sm font-600 text-ink' }, row.name),
          h('div', { class: 'truncate text-xs text-slate' }, row.app_id),
          row.description ? h('div', { class: 'truncate text-xs text-slate' }, row.description) : null,
        ]),
        default: () => appTooltipContent(row),
      },
    ),
  },
  {
    title: '状态',
    key: 'status',
    width: 82,
    render: (row) => {
      const meta = statusMeta(row.status)
      return h(NTag, { size: 'small', round: true, bordered: false, type: meta.type }, { default: () => meta.label })
    },
  },
  {
    title: '能力授权',
    key: 'allowed_caps',
    width: 194,
    render: (row) => {
      const labels = compactCapabilityLabels(row.allowed_caps)
      const children = labels.visible.map(label => h(NTag, { size: 'small', round: true, bordered: false }, { default: () => label }))
      if (labels.hidden.length > 0) {
        children.push(h(
          NTooltip,
          { placement: 'top', delay: 300 },
          {
            trigger: () => h(NTag, { size: 'small', round: true, bordered: false }, { default: () => `+${labels.hidden.length}` }),
            default: () => labels.hidden.join(' / '),
          },
        ))
      }
      return h('div', { class: 'flex min-w-0 flex-wrap gap-1.5' }, children)
    },
  },
  {
    title: '默认工作流',
    key: 'default_workflows',
    width: 184,
    render: (row) => {
      const labels = defaultWorkflowLabels(row)
      if (labels.length === 0)
        return '-'
      const [first, second, ...rest] = labels
      const children = [
        h('div', { class: 'truncate text-xs text-slate' }, first),
      ]
      if (second)
        children.push(h('div', { class: 'truncate text-xs text-slate' }, second))
      if (rest.length > 0)
        children.push(h('div', { class: 'text-xs text-slate/80' }, `+${rest.length}`))
      return h(
        NTooltip,
        { placement: 'top-start', delay: 300 },
        {
          trigger: () => h('div', { class: 'grid min-w-0 gap-1' }, children),
          default: () => labels.join('\n'),
        },
      )
    },
  },
  {
    title: '限流',
    key: 'rate_limit_per_sec',
    width: 72,
    render: row => `${row.rate_limit_per_sec}/s`,
  },
  {
    title: '密钥提示',
    key: 'secret_hint',
    width: 132,
    render: row => h('div', { class: 'grid gap-1' }, [
      h('div', { class: 'truncate text-xs text-slate' }, row.secret_hint || '仅创建或重置后显示完整密钥'),
      h('div', { class: 'text-xs text-slate/80' }, `版本 v${row.secret_version}`),
    ]),
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 152,
    render: row => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 166,
    render: (row) => {
      const primaryActions = [
        h(NButton, { quaternary: true, size: 'tiny', onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { quaternary: true, size: 'tiny', onClick: () => void openCallLogs(row) }, { default: () => '日志' }),
      ]
      const secondaryActions: typeof primaryActions = []
      if (row.status !== 'revoked') {
        secondaryActions.push(h(NButton, { quaternary: true, size: 'tiny', onClick: () => void handleRotateSecret(row) }, { default: () => '重置密钥' }))
      }
      if (row.status === 'active')
        secondaryActions.push(h(NButton, { quaternary: true, size: 'tiny', type: 'warning', onClick: () => void handleToggleStatus(row) }, { default: () => '停用' }))
      if (row.status === 'disabled')
        secondaryActions.push(h(NButton, { quaternary: true, size: 'tiny', type: 'success', onClick: () => void handleToggleStatus(row) }, { default: () => '启用' }))
      if (row.status !== 'revoked')
        secondaryActions.push(h(NButton, { quaternary: true, size: 'tiny', type: 'error', onClick: () => void handleRevoke(row) }, { default: () => '撤销' }))

      return h('div', { class: 'openapi-action-cell' }, [
        h('div', { class: 'openapi-action-row' }, primaryActions),
        h('div', { class: 'openapi-action-row openapi-action-row-secondary' }, secondaryActions),
      ])
    },
  },
])

watch(
  isAdmin,
  (value) => {
    if (value)
      void initializePage()
  },
  { immediate: true },
)

watch([keyword, statusFilter, capabilityFilter], () => {
  appPagination.page = 1
})

watch(activeTab, (tab) => {
  if (tab === 'docs' && !docsContent.value && !docsLoading.value)
    void loadDocs()
  if (tab === 'docs') {
    void nextTick(() => {
      docsContainerRef.value?.scrollTo({ top: 0 })
    })
  }
})
</script>

<template>
  <div class="h-full min-h-0 min-w-0 flex flex-col gap-5 overflow-hidden">
    <NAlert v-if="!isAdmin" type="warning" title="仅管理员可访问" class="card-main">
      OpenAPI 应用管理接口受管理员角色保护，当前账号无法查看或修改开放平台应用。
    </NAlert>

    <template v-else>
      <NCard class="card-main min-w-0 flex flex-col min-h-0 flex-1 overflow-hidden" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <NTabs v-model:value="activeTab" type="line" animated class="openapi-tabs flex-1 min-h-0 flex flex-col">
          <NTabPane name="apps" tab="应用管理" display-directive="show">
            <template #tab>
              应用管理
            </template>
            <div class="flex flex-wrap items-center justify-between gap-3 pt-4">
              <div class="grid gap-1">
                <span class="text-sm font-600">OpenAPI 应用</span>
                <span class="text-xs text-slate">管理 app_id / app_secret、能力授权、默认工作流和最近调用日志。</span>
              </div>
              <div class="flex flex-wrap items-center gap-2">
                <NInput v-model:value="keyword" :maxlength="128" clearable placeholder="搜索名称 / App ID / 能力" size="small" class="w-full sm:!w-72" />
                <NButton quaternary size="small" :loading="loading || initializing" @click="initializePage">
                  刷新
                </NButton>
                <NButton type="primary" size="small" color="#0f766e" @click="openCreateModal">
                  新增应用
                </NButton>
              </div>
            </div>

            <div class="flex flex-col gap-4 py-4 flex-1 min-h-0">
              <NAlert type="info" :show-icon="false">
                创建应用或重置密钥后，前端只会展示一次完整的 app_secret；关闭后列表中仅保留掩码提示。
              </NAlert>

              <div class="grid gap-3 md:grid-cols-4 shrink-0">
                <div class="rounded-3 border border-gray-200/70 bg-white/60 px-4 py-3 backdrop-blur-sm">
                  <div class="text-xs text-slate/80">
                    应用总数
                  </div>
                  <div class="mt-2 text-2xl font-700 text-ink">
                    {{ apps.length }}
                  </div>
                </div>
                <div class="rounded-3 border border-gray-200/70 bg-white/60 px-4 py-3 backdrop-blur-sm">
                  <div class="text-xs text-slate/80">
                    启用中
                  </div>
                  <div class="mt-2 text-2xl font-700 text-teal-700">
                    {{ activeCount }}
                  </div>
                </div>
                <div class="rounded-3 border border-gray-200/70 bg-white/60 px-4 py-3 backdrop-blur-sm">
                  <div class="text-xs text-slate/80">
                    已停用
                  </div>
                  <div class="mt-2 text-2xl font-700 text-amber-600">
                    {{ disabledCount }}
                  </div>
                </div>
                <div class="rounded-3 border border-gray-200/70 bg-white/60 px-4 py-3 backdrop-blur-sm">
                  <div class="text-xs text-slate/80">
                    已撤销
                  </div>
                  <div class="mt-2 text-2xl font-700 text-red-600">
                    {{ revokedCount }}
                  </div>
                </div>
              </div>

              <div class="grid gap-2 shrink-0">
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-xs font-600 text-slate/80">状态</span>
                  <NTag
                    v-for="item in statusFilterOptions"
                    :key="item.value"
                    size="small"
                    round
                    :bordered="statusFilter !== item.value"
                    :type="statusFilter === item.value ? item.type : 'default'"
                    class="openapi-filter-tag"
                    @click="statusFilter = item.value"
                  >
                    {{ item.label }} {{ item.count }}
                  </NTag>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <span class="text-xs font-600 text-slate/80">能力</span>
                  <NTag
                    v-for="item in capabilityFilterOptions"
                    :key="item.value"
                    size="small"
                    round
                    :bordered="capabilityFilter !== item.value"
                    :type="capabilityFilter === item.value ? 'info' : 'default'"
                    class="openapi-filter-tag"
                    @click="capabilityFilter = item.value"
                  >
                    {{ item.label }} {{ item.count }}
                  </NTag>
                </div>
              </div>

              <NDataTable
                flex-height
                class="flex-1 min-h-0"
                :columns="appColumns"
                :data="filteredApps"
                :loading="loading"
                :pagination="{ page: appPagination.page, pageSize: appPagination.pageSize, onUpdatePage: updateAppPage }"
                size="small"
              />
            </div>
          </NTabPane>
          <NTabPane name="docs" tab="对接文档" display-directive="show:lazy">
            <div class="flex flex-col gap-3 py-4 flex-1 min-h-0">
              <div class="flex flex-wrap items-center gap-2 shrink-0">
                <NSelect v-model:value="docsCapabilityFilter" class="w-full sm:!w-56" size="small" :options="docsCapabilityOptions" />
                <NInput v-model:value="docsKeyword" :maxlength="128" clearable placeholder="搜索接口、字段或示例" size="small" class="w-full sm:!w-72" />
                <NButton v-if="docsHasFilter" size="small" quaternary @click="resetDocsFilters">
                  清除筛选
                </NButton>
                <NTag v-if="docsHasFilter" size="small" round :bordered="false" type="info">
                  命中 {{ docsMatchCount }} 段
                </NTag>
                <span class="flex-1" />
                <NButton size="small" quaternary :loading="docsLoading" @click="loadDocs">
                  刷新文档
                </NButton>
                <NButton size="small" quaternary :disabled="!docsContent" @click="copyDocsContent">
                  复制当前文档
                </NButton>
                <NButton size="small" quaternary @click="copyAuthExample">
                  复制鉴权示例
                </NButton>
              </div>

              <NSpin :show="docsLoading" class="flex-1 min-h-0">
                <div v-if="docsError && !docsContent" class="docs-empty">
                  <div class="docs-empty-title">
                    无法加载对接文档
                  </div>
                  <div class="docs-empty-desc">
                    {{ docsError }}
                  </div>
                  <NButton type="primary" color="#0f766e" size="small" @click="loadDocs">
                    重试
                  </NButton>
                </div>
                <div v-else-if="!docsContent && !docsLoading" class="docs-empty">
                  <div class="docs-empty-title">
                    暂无对接文档
                  </div>
                  <div class="docs-empty-desc">
                    点击刷新尝试重新加载。
                  </div>
                  <NButton type="primary" color="#0f766e" size="small" @click="loadDocs">
                    刷新文档
                  </NButton>
                </div>
                <div v-else-if="docsHasFilter && docsMatchCount === 0" class="docs-empty">
                  <div class="docs-empty-title">
                    未命中任何章节
                  </div>
                  <div class="docs-empty-desc">
                    尝试更换关键字或能力筛选。
                  </div>
                  <NButton size="small" quaternary @click="resetDocsFilters">
                    清除筛选
                  </NButton>
                </div>
                <div v-else class="docs-layout">
                  <aside class="docs-toc">
                    <div class="docs-toc-title">
                      章节导航
                    </div>
                    <ul class="docs-toc-list">
                      <li
                        v-for="item in docsRender.toc"
                        :key="item.id"
                        class="docs-toc-item" :class="[`docs-toc-h${item.level}`]"
                        @click="scrollToHeading(item.id)"
                      >
                        {{ item.text }}
                      </li>
                    </ul>
                  </aside>
                  <div ref="docsContainerRef" class="docs-content markdown-body" @click="handleDocsClick" v-html="docsRender.html" />
                </div>
              </NSpin>
            </div>
          </NTabPane>
        </NTabs>
      </NCard>

      <NModal v-model:show="formVisible" preset="card" :title="modalTitle" class="modal-card max-w-220">
        <div class="grid gap-5">
          <NAlert type="info" :show-icon="false">
            这里维护的是开放平台应用授权，不影响后台登录用户。默认工作流只对支持的能力生效。
          </NAlert>

          <NForm :model="form" label-placement="top">
            <div class="grid gap-4 md:grid-cols-2">
              <NFormItem label="应用名称" required>
                <NInput v-model:value="form.name" :maxlength="128" placeholder="例如：第三方会议助手" />
              </NFormItem>
              <NFormItem label="每秒限流">
                <NInputNumber v-model:value="form.rate_limit_per_sec" :min="1" :max="5000" class="w-full" />
              </NFormItem>
            </div>

            <NFormItem label="描述">
              <NInput v-model:value="form.description" :maxlength="512" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="描述应用用途、调用场景或租户信息" />
            </NFormItem>

            <NFormItem label="能力授权" required>
              <NCheckboxGroup v-model:value="form.allowed_caps">
                <div class="grid gap-3 md:grid-cols-2">
                  <div
                    v-for="item in capabilities"
                    :key="item.id"
                    class="rounded-3 border border-gray-200/60 bg-[#fbfdff] px-4 py-3"
                  >
                    <NCheckbox :value="item.id">
                      <div class="grid gap-1 pl-2">
                        <div class="text-sm font-600 text-ink">
                          {{ item.display_name }}
                        </div>
                        <div class="text-xs leading-5 text-slate">
                          {{ item.description }}
                        </div>
                      </div>
                    </NCheckbox>
                  </div>
                </div>
              </NCheckboxGroup>
            </NFormItem>

            <NFormItem label="默认工作流">
              <div class="grid gap-3">
                <NAlert v-if="selectedCapabilityCount === 0" type="warning" :show-icon="false">
                  先选择能力，再按需绑定默认工作流。未绑定时，调用方仍可在请求中显式指定 workflow_id。
                </NAlert>

                <div v-if="hasCapability('asr.recognize')" class="rounded-3 border border-gray-200/60 bg-[#fbfdff] p-4">
                  <div class="text-sm font-600 text-ink">
                    语音转文字默认工作流
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate">
                    短音频同步识别和异步 ASR 任务会优先复用批量转写工作流。
                  </div>
                  <NSelect v-model:value="form.default_workflow_asr_recognize" class="mt-3" clearable filterable :options="batchCatalog.workflowOptions.value" placeholder="选择批量转写工作流" />
                </div>

                <div v-if="hasCapability('nlp.correct')" class="rounded-3 border border-gray-200/60 bg-[#fbfdff] p-4">
                  <div class="text-sm font-600 text-ink">
                    文本纠错默认工作流
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate">
                    独立纠错能力会校验批量转写类型的工作流配置。
                  </div>
                  <NSelect v-model:value="form.default_workflow_nlp_correct" class="mt-3" clearable filterable :options="batchCatalog.workflowOptions.value" placeholder="选择批量转写工作流" />
                </div>

                <div v-if="hasCapability('asr.stream')" class="rounded-3 border border-gray-200/60 bg-[#fbfdff] p-4">
                  <div class="text-sm font-600 text-ink">
                    实时流识别默认工作流
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate">
                    流式识别结束后的后处理会优先套用实时工作流。
                  </div>
                  <NSelect v-model:value="form.default_workflow_asr_stream" class="mt-3" clearable filterable :options="realtimeCatalog.workflowOptions.value" placeholder="选择实时工作流" />
                </div>

                <div v-if="hasCapability('meeting.summary')" class="rounded-3 border border-gray-200/60 bg-[#fbfdff] p-4">
                  <div class="text-sm font-600 text-ink">
                    会议纪要默认工作流
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate">
                    音频纪要和文本纪要接口会优先使用会议纪要工作流。
                  </div>
                  <NSelect v-model:value="form.default_workflow_meeting_summary" class="mt-3" clearable filterable :options="meetingCatalog.workflowOptions.value" placeholder="选择会议纪要工作流" />
                </div>

                <div v-if="hasCapability('skill.invoke')" class="rounded-3 border border-gray-200/60 bg-[#fbfdff] p-4">
                  <div class="text-sm font-600 text-ink">
                    Skill 回调默认工作流
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate">
                    命中语音指令后的识别链路会校验语音控制类型的工作流。
                  </div>
                  <NSelect v-model:value="form.default_workflow_skill_invoke" class="mt-3" clearable filterable :options="voiceCatalog.workflowOptions.value" placeholder="选择语音控制工作流" />
                </div>
              </div>
            </NFormItem>

            <NFormItem label="回调白名单">
              <NInput
                v-model:value="form.callback_whitelist_text"
                :maxlength="4000"
                type="textarea"
                :autosize="{ minRows: 3, maxRows: 6 }"
                placeholder="每行一个地址前缀，例如：https://partner.example.com/openapi/callback"
              />
              <div class="mt-2 text-xs leading-5 text-slate">
                启用 Skill 回调时必填。平台只允许回调到这里配置的 URL 前缀。
              </div>
            </NFormItem>

            <NFormItem label="Meta JSON">
              <NInput
                v-model:value="form.meta_json"
                :maxlength="4000"
                type="textarea"
                :autosize="{ minRows: 2, maxRows: 5 }"
                placeholder="可选，原样存储的扩展元数据，例如：{&quot;tenant&quot;:&quot;hospital-a&quot;}"
              />
            </NFormItem>
          </NForm>

          <div class="flex justify-end gap-3">
            <NButton @click="formVisible = false">
              取消
            </NButton>
            <NButton type="primary" color="#0f766e" :loading="saving" @click="handleSubmit">
              {{ editingAppId ? '保存修改' : '创建应用' }}
            </NButton>
          </div>
        </div>
      </NModal>

      <NModal v-model:show="secretDialog.visible" preset="card" title="应用凭证" class="modal-card max-w-180">
        <div class="grid gap-4">
          <NAlert type="warning">
            请立即保存 app_secret。{{ secretDialog.actionLabel }}后关闭此弹窗，页面将只保留密钥掩码提示。
          </NAlert>
          <NForm label-placement="top">
            <NFormItem label="应用名称">
              <NInput :value="secretDialog.appName" readonly />
            </NFormItem>
            <NFormItem label="App ID">
              <NInput :value="secretDialog.appId" readonly />
            </NFormItem>
            <NFormItem label="App Secret">
              <NInput :value="secretDialog.appSecret" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" readonly />
            </NFormItem>
            <div class="text-xs text-slate">
              密钥版本：v{{ secretDialog.secretVersion }}
            </div>
          </NForm>
        </div>
      </NModal>

      <NModal
        v-model:show="callsVisible"
        preset="card"
        title="最近调用日志"
        class="modal-card openapi-call-log-modal"
        size="small"
        :style="{ width: 'min(680px, calc(100vw - 48px))', maxWidth: 'calc(100vw - 48px)' }"
        content-style="padding: 10px 16px 14px;"
      >
        <div class="grid min-w-0 gap-2.5">
          <div class="flex min-w-0 items-center justify-between gap-2">
            <NTooltip placement="top-start" :delay="300">
              <template #trigger>
                <div class="min-w-0 truncate text-[11px] text-slate">
                  {{ callsTitle }}
                </div>
              </template>
              <div class="openapi-text-tooltip">
                {{ callsTitle }}
              </div>
            </NTooltip>
            <NTag size="small" round :bordered="false" type="info">
              {{ callLogs.length }} 条
            </NTag>
          </div>

          <NSpin :show="callsLoading">
            <div v-if="!callsLoading && callLogs.length === 0" class="openapi-call-log-empty">
              暂无调用日志
            </div>
            <div v-else class="openapi-call-log-list">
              <div v-for="item in callLogs" :key="item.id" class="openapi-call-log-item">
                <div class="openapi-call-log-main">
                  <div class="openapi-call-log-time">
                    {{ formatDateTime(item.created_at) }}
                  </div>
                  <NTag size="small" round :bordered="false" :type="httpStatusType(item.http_status)">
                    {{ item.http_status }}
                  </NTag>
                  <div class="openapi-call-log-duration">
                    {{ item.latency_ms }} ms
                  </div>
                </div>

                <div class="openapi-call-log-path-row">
                  <NTooltip placement="top-start" :delay="300">
                    <template #trigger>
                      <div class="openapi-call-log-path">
                        {{ item.route }}
                      </div>
                    </template>
                    <div class="openapi-text-tooltip">
                      {{ item.route }}
                    </div>
                  </NTooltip>
                </div>

                <div class="openapi-call-log-meta">
                  <NTag size="small" round :bordered="false">
                    {{ capabilityLabel(item.capability) }}
                  </NTag>
                  <NTooltip placement="top-start" :delay="300">
                    <template #trigger>
                      <span class="openapi-call-log-chip is-error">
                        {{ logErrorText(item.err_code) }}
                      </span>
                    </template>
                    <div class="openapi-text-tooltip">
                      {{ logErrorText(item.err_code) }}
                    </div>
                  </NTooltip>
                  <NTooltip placement="top-start" :delay="300">
                    <template #trigger>
                      <span class="openapi-call-log-chip">
                        {{ item.request_id }}
                      </span>
                    </template>
                    <div class="openapi-text-tooltip">
                      {{ item.request_id }}
                    </div>
                  </NTooltip>
                </div>
              </div>
            </div>
          </NSpin>
        </div>
      </NModal>
    </template>
  </div>
</template>

<style scoped>
.openapi-tabs :deep(.n-tabs-content),
.openapi-tabs :deep(.n-tabs-pane-wrapper),
.openapi-tabs :deep(.n-tab-pane) {
  min-height: 0;
  flex: 1;
  display: flex;
  flex-direction: column;
}

.openapi-app-cell {
  max-width: 100%;
}

.openapi-app-tooltip {
  max-width: 360px;
  word-break: break-word;
  white-space: normal;
}

.openapi-text-tooltip {
  max-width: 420px;
  word-break: break-word;
  white-space: normal;
}

.openapi-action-cell {
  display: grid;
  min-width: 0;
  gap: 2px;
}

.openapi-action-row {
  display: flex;
  min-width: 0;
  height: 22px;
  align-items: center;
  gap: 4px;
  white-space: nowrap;
}

.openapi-action-row-secondary {
  color: #64748b;
}

.openapi-action-cell :deep(.n-button) {
  padding: 0 3px;
}

.openapi-call-log-modal {
  width: min(680px, calc(100vw - 48px));
}

.openapi-call-log-modal :deep(.n-card__content) {
  overflow: hidden;
}

.openapi-call-log-empty {
  display: flex;
  min-height: 96px;
  align-items: center;
  justify-content: center;
  border: 1px dashed rgba(148, 163, 184, 0.45);
  border-radius: 8px;
  background: rgba(248, 250, 252, 0.72);
  color: #64748b;
  font-size: 12px;
}

.openapi-call-log-list {
  display: grid;
  max-height: min(420px, calc(100vh - 240px));
  gap: 8px;
  overflow-y: auto;
  padding-right: 2px;
}

.openapi-call-log-item {
  display: grid;
  min-width: 0;
  gap: 6px;
  border: 1px solid rgba(203, 213, 225, 0.62);
  border-radius: 8px;
  background: rgba(255, 255, 255, 0.86);
  padding: 9px 10px;
}

.openapi-call-log-main {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto auto;
  min-width: 0;
  align-items: center;
  gap: 8px;
  font-size: 11.5px;
}

.openapi-call-log-time {
  min-width: 0;
  overflow: hidden;
  color: #334155;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.openapi-call-log-duration {
  color: #64748b;
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.openapi-call-log-path-row {
  min-width: 0;
}

.openapi-call-log-path {
  min-width: 0;
  overflow: hidden;
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.04);
  padding: 4px 7px;
  color: #1e293b;
  font-family: "JetBrains Mono", "Fira Code", "Menlo", monospace;
  font-size: 11.5px;
  line-height: 17px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.openapi-call-log-meta {
  display: flex;
  min-width: 0;
  flex-wrap: wrap;
  align-items: center;
  gap: 5px;
}

.openapi-call-log-chip {
  display: inline-flex;
  max-width: 220px;
  min-width: 0;
  overflow: hidden;
  align-items: center;
  border-radius: 999px;
  background: rgba(241, 245, 249, 0.92);
  padding: 2px 7px;
  color: #475569;
  font-size: 11px;
  line-height: 18px;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.openapi-call-log-chip.is-error {
  max-width: 180px;
}

.openapi-filter-tag {
  cursor: pointer;
  user-select: none;
  transition: border-color 0.15s ease, color 0.15s ease, background 0.15s ease;
}

.openapi-filter-tag:hover {
  border-color: rgba(15, 118, 110, 0.4);
  color: #0f766e;
}

.docs-layout {
  display: grid;
  grid-template-columns: 220px minmax(0, 1fr);
  gap: 16px;
  min-height: 0;
  height: 100%;
}

@media (max-width: 900px) {
  .docs-layout {
    grid-template-columns: 1fr;
  }
  .docs-toc {
    position: static;
    max-height: 200px;
  }
}

.docs-toc {
  position: sticky;
  top: 0;
  align-self: start;
  max-height: calc(100vh - 220px);
  overflow-y: auto;
  border: 1px solid rgba(203, 213, 225, 0.55);
  border-radius: 10px;
  background: rgba(255, 255, 255, 0.7);
  padding: 12px 8px;
}

.docs-toc-title {
  font-size: 12px;
  color: #64748b;
  padding: 4px 8px 8px;
  letter-spacing: 0.05em;
}

.docs-toc-list {
  list-style: none;
  margin: 0;
  padding: 0;
  display: grid;
  gap: 2px;
}

.docs-toc-item {
  padding: 6px 10px;
  border-radius: 6px;
  font-size: 12.5px;
  line-height: 1.4;
  color: #334155;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
}

.docs-toc-item:hover {
  background: rgba(15, 118, 110, 0.08);
  color: #0f766e;
}

.docs-toc-h3 {
  padding-left: 22px;
  font-size: 12px;
  color: #475569;
}

.docs-content {
  border: 1px solid rgba(203, 213, 225, 0.55);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.85);
  padding: 24px 28px;
  font-size: 13.5px;
  line-height: 1.75;
  color: #1f2937;
  overflow-y: auto;
  max-height: calc(100vh - 220px);
  scroll-behavior: smooth;
}

.docs-empty {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: 12px;
  padding: 56px 24px;
  border: 1px dashed rgba(148, 163, 184, 0.6);
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.6);
  text-align: center;
}

.docs-empty-title {
  font-size: 14px;
  font-weight: 600;
  color: #0f172a;
}

.docs-empty-desc {
  font-size: 12.5px;
  color: #64748b;
  max-width: 360px;
}
</style>

<style>
.markdown-body h1,
.markdown-body h2,
.markdown-body h3 {
  scroll-margin-top: 12px;
  color: #0f172a;
  font-weight: 600;
  letter-spacing: -0.005em;
}

.markdown-body h1 {
  font-size: 22px;
  margin: 4px 0 18px;
  padding-bottom: 10px;
  border-bottom: 1px solid rgba(148, 163, 184, 0.3);
}

.markdown-body h2 {
  font-size: 17px;
  margin: 28px 0 12px;
  padding-left: 10px;
  border-left: 3px solid #0f766e;
}

.markdown-body h3 {
  font-size: 14.5px;
  margin: 20px 0 8px;
  color: #1e293b;
}

.markdown-body p {
  margin: 8px 0;
}

.markdown-body ul,
.markdown-body ol {
  padding-left: 22px;
  margin: 8px 0;
}

.markdown-body li {
  margin: 4px 0;
}

.markdown-body a {
  color: #0f766e;
  text-decoration: none;
  border-bottom: 1px dashed rgba(15, 118, 110, 0.5);
}

.markdown-body a:hover {
  color: #0d5d56;
}

.markdown-body blockquote {
  margin: 12px 0;
  padding: 10px 14px;
  border-left: 3px solid #d97706;
  background: rgba(217, 119, 6, 0.06);
  color: #6b4f1b;
  border-radius: 4px;
  font-size: 13px;
}

.markdown-body table {
  width: 100%;
  border-collapse: collapse;
  margin: 12px 0;
  font-size: 12.5px;
  background: #fff;
  border-radius: 8px;
  overflow: hidden;
  border: 1px solid rgba(203, 213, 225, 0.55);
}

.markdown-body thead {
  background: rgba(15, 118, 110, 0.08);
  color: #0f172a;
}

.markdown-body th,
.markdown-body td {
  padding: 8px 12px;
  text-align: left;
  border-bottom: 1px solid rgba(203, 213, 225, 0.4);
  vertical-align: top;
}

.markdown-body tbody tr:last-child td {
  border-bottom: none;
}

.markdown-body code {
  font-family: "JetBrains Mono", "Fira Code", "Menlo", monospace;
  background: rgba(15, 23, 42, 0.06);
  border-radius: 4px;
  padding: 1px 5px;
  font-size: 12.5px;
  color: #be185d;
}

.markdown-body pre {
  margin: 0;
  padding: 14px 16px;
  background: #0f172a;
  color: #e2e8f0;
  border-radius: 8px;
  overflow-x: auto;
  font-size: 12.5px;
  line-height: 1.6;
}

.markdown-body pre code {
  background: transparent;
  color: inherit;
  padding: 0;
  font-size: inherit;
}

.markdown-body .docs-code {
  position: relative;
  margin: 12px 0;
}

.markdown-body .docs-code-copy {
  position: absolute;
  top: 8px;
  right: 8px;
  padding: 3px 10px;
  font-size: 11.5px;
  color: #cbd5e1;
  background: rgba(255, 255, 255, 0.08);
  border: 1px solid rgba(255, 255, 255, 0.18);
  border-radius: 5px;
  cursor: pointer;
  transition: background 0.15s ease, color 0.15s ease;
  z-index: 1;
}

.markdown-body .docs-code-copy:hover {
  background: rgba(255, 255, 255, 0.15);
  color: #fff;
}

.markdown-body hr {
  border: none;
  border-top: 1px solid rgba(203, 213, 225, 0.5);
  margin: 18px 0;
}

.markdown-body > *:first-child {
  margin-top: 0;
}
</style>

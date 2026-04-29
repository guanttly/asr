<script setup lang="ts">
import type { AxiosError } from 'axios'
import type { DataTableColumns } from 'naive-ui'
import type { OpenPlatformApp, OpenPlatformCallLog, OpenPlatformCapability, OpenPlatformCreateAppPayload, OpenPlatformCreateAppResponse } from '@/api/openplatform'

import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, reactive, ref, watch } from 'vue'

import {
  createOpenPlatformApp,
  disableOpenPlatformApp,
  enableOpenPlatformApp,
  getOpenPlatformAppCalls,
  getOpenPlatformApps,
  getOpenPlatformCapabilities,
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

const apps = ref<OpenPlatformApp[]>([])
const capabilities = ref<OpenPlatformCapability[]>([])
const formVisible = ref(false)
const editingAppId = ref<number | null>(null)

const callsVisible = ref(false)
const callsLoading = ref(false)
const callsTitle = ref('')
const callLogs = ref<OpenPlatformCallLog[]>([])

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

const filteredApps = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  if (!value)
    return apps.value

  return apps.value.filter((item) => {
    const caps = item.allowed_caps.map(capabilityLabel).join(' ')
    return item.name.toLowerCase().includes(value)
      || item.app_id.toLowerCase().includes(value)
      || (item.description || '').toLowerCase().includes(value)
      || item.status.toLowerCase().includes(value)
      || caps.toLowerCase().includes(value)
  })
})

function capabilityLabel(capability: string) {
  return capabilityMap.value.get(capability)?.display_name || capability
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

const appColumns = computed<DataTableColumns<OpenPlatformApp>>(() => [
  {
    title: '应用',
    key: 'name',
    minWidth: 240,
    render: (row) => {
      const lines = [
        h('div', { class: 'text-sm font-600 text-ink' }, row.name),
        h('div', { class: 'text-xs text-slate break-all' }, row.app_id),
      ]
      if (row.description)
        lines.push(h('div', { class: 'text-xs text-slate line-clamp-2' }, row.description))
      return h('div', { class: 'grid gap-1' }, lines)
    },
  },
  {
    title: '状态',
    key: 'status',
    width: 92,
    render: (row) => {
      const meta = statusMeta(row.status)
      return h(NTag, { size: 'small', round: true, bordered: false, type: meta.type }, { default: () => meta.label })
    },
  },
  {
    title: '能力授权',
    key: 'allowed_caps',
    minWidth: 280,
    render: (row) => h(
      'div',
      { class: 'flex flex-wrap gap-1.5' },
      row.allowed_caps.map(capability => h(NTag, { size: 'small', round: true, bordered: false }, { default: () => capabilityLabel(capability) })),
    ),
  },
  {
    title: '默认工作流',
    key: 'default_workflows',
    minWidth: 260,
    render: (row) => {
      const labels = defaultWorkflowLabels(row)
      if (labels.length === 0)
        return '-'
      return h('div', { class: 'grid gap-1' }, labels.map(label => h('div', { class: 'text-xs text-slate' }, label)))
    },
  },
  {
    title: '限流',
    key: 'rate_limit_per_sec',
    width: 90,
    render: (row) => `${row.rate_limit_per_sec}/s`,
  },
  {
    title: '密钥提示',
    key: 'secret_hint',
    minWidth: 180,
    render: (row) => h('div', { class: 'grid gap-1' }, [
      h('div', { class: 'text-xs text-slate break-all' }, row.secret_hint || '仅创建或重置后显示完整密钥'),
      h('div', { class: 'text-xs text-slate/80' }, `版本 v${row.secret_version}`),
    ]),
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 168,
    render: (row) => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    minWidth: 300,
    render: (row) => {
      const children = [
        h(NButton, { quaternary: true, size: 'tiny', onClick: () => openEditModal(row) }, { default: () => '编辑' }),
        h(NButton, { quaternary: true, size: 'tiny', onClick: () => void openCallLogs(row) }, { default: () => '日志' }),
      ]
      if (row.status !== 'revoked') {
        children.push(h(NButton, { quaternary: true, size: 'tiny', onClick: () => void handleRotateSecret(row) }, { default: () => '重置密钥' }))
        children.push(h(NButton, { quaternary: true, size: 'tiny', type: 'error', onClick: () => void handleRevoke(row) }, { default: () => '撤销' }))
      }
      if (row.status === 'active')
        children.push(h(NButton, { quaternary: true, size: 'tiny', type: 'warning', onClick: () => void handleToggleStatus(row) }, { default: () => '停用' }))
      if (row.status === 'disabled')
        children.push(h(NButton, { quaternary: true, size: 'tiny', type: 'success', onClick: () => void handleToggleStatus(row) }, { default: () => '启用' }))
      return h('div', { class: 'flex flex-wrap gap-1.5' }, children)
    },
  },
])

const callColumns: DataTableColumns<OpenPlatformCallLog> = [
  {
    title: '时间',
    key: 'created_at',
    width: 168,
    render: row => formatDateTime(row.created_at),
  },
  {
    title: '能力',
    key: 'capability',
    width: 132,
    render: row => capabilityLabel(row.capability),
  },
  {
    title: '路径',
    key: 'route',
    minWidth: 220,
    render: row => h('div', { class: 'text-xs break-all text-slate' }, row.route),
  },
  {
    title: '状态',
    key: 'http_status',
    width: 90,
    render: (row) => h(
      NTag,
      {
        size: 'small',
        round: true,
        bordered: false,
        type: row.http_status < 400 ? 'success' : row.http_status < 500 ? 'warning' : 'error',
      },
      { default: () => String(row.http_status) },
    ),
  },
  {
    title: '耗时',
    key: 'latency_ms',
    width: 88,
    render: row => `${row.latency_ms} ms`,
  },
  {
    title: '错误码',
    key: 'err_code',
    width: 120,
    render: row => row.err_code || '-',
  },
  {
    title: '请求 ID',
    key: 'request_id',
    minWidth: 240,
    render: row => h('div', { class: 'text-xs break-all text-slate' }, row.request_id),
  },
]

watch(
  isAdmin,
  (value) => {
    if (value)
      void initializePage()
  },
  { immediate: true },
)
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 gap-5">
    <NAlert v-if="!isAdmin" type="warning" title="仅管理员可访问" class="card-main">
      OpenAPI 应用管理接口受管理员角色保护，当前账号无法查看或修改开放平台应用。
    </NAlert>

    <template v-else>
      <NCard class="card-main flex flex-col min-h-0 flex-1" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="grid gap-1">
              <span class="text-sm font-600">OpenAPI 应用</span>
              <span class="text-xs text-slate">管理 app_id / app_secret、能力授权、默认工作流和最近调用日志。</span>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <NInput v-model:value="keyword" clearable placeholder="搜索名称 / App ID / 能力" size="small" class="w-full sm:!w-72" />
              <NButton quaternary size="small" :loading="loading || initializing" @click="initializePage">
                刷新
              </NButton>
              <NButton type="primary" size="small" color="#0f766e" @click="openCreateModal">
                新增应用
              </NButton>
            </div>
          </div>
        </template>

        <div class="grid gap-4 py-4 flex-1 min-h-0">
          <NAlert type="info" :show-icon="false">
            创建应用或重置密钥后，前端只会展示一次完整的 app_secret；关闭后列表中仅保留掩码提示。
          </NAlert>

          <div class="grid gap-3 md:grid-cols-4">
            <div class="rounded-3 bg-[#fbfdff] px-4 py-3">
              <div class="text-xs text-slate">应用总数</div>
              <div class="mt-2 text-2xl font-700 text-ink">
                {{ apps.length }}
              </div>
            </div>
            <div class="rounded-3 bg-[#fbfdff] px-4 py-3">
              <div class="text-xs text-slate">启用中</div>
              <div class="mt-2 text-2xl font-700 text-teal-700">
                {{ activeCount }}
              </div>
            </div>
            <div class="rounded-3 bg-[#fbfdff] px-4 py-3">
              <div class="text-xs text-slate">已停用</div>
              <div class="mt-2 text-2xl font-700 text-amber-600">
                {{ disabledCount }}
              </div>
            </div>
            <div class="rounded-3 bg-[#fbfdff] px-4 py-3">
              <div class="text-xs text-slate">已撤销</div>
              <div class="mt-2 text-2xl font-700 text-red-600">
                {{ revokedCount }}
              </div>
            </div>
          </div>

          <NDataTable
            flex-height
            class="flex-1 min-h-0"
            :columns="appColumns"
            :data="filteredApps"
            :loading="loading"
            :pagination="{ pageSize: 10 }"
            :scroll-x="1680"
            size="small"
          />
        </div>
      </NCard>

      <NModal v-model:show="formVisible" preset="card" :title="modalTitle" class="modal-card max-w-220">
        <div class="grid gap-5">
          <NAlert type="info" :show-icon="false">
            这里维护的是开放平台应用授权，不影响后台登录用户。默认工作流只对支持的能力生效。
          </NAlert>

          <NForm :model="form" label-placement="top">
            <div class="grid gap-4 md:grid-cols-2">
              <NFormItem label="应用名称" required>
                <NInput v-model:value="form.name" placeholder="例如：第三方会议助手" />
              </NFormItem>
              <NFormItem label="每秒限流">
                <NInputNumber v-model:value="form.rate_limit_per_sec" :min="1" :max="5000" class="w-full" />
              </NFormItem>
            </div>

            <NFormItem label="描述">
              <NInput v-model:value="form.description" type="textarea" :autosize="{ minRows: 2, maxRows: 4 }" placeholder="描述应用用途、调用场景或租户信息" />
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
                type="textarea"
                :autosize="{ minRows: 2, maxRows: 5 }"
                placeholder="可选，原样存储的扩展元数据，例如：{&quot;tenant&quot;:&quot;demo&quot;}"
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

      <NModal v-model:show="callsVisible" preset="card" title="最近调用日志" class="modal-card max-w-240">
        <div class="grid gap-3">
          <div class="text-xs text-slate">
            {{ callsTitle }}
          </div>
          <NDataTable :columns="callColumns" :data="callLogs" :loading="callsLoading" :pagination="{ pageSize: 8 }" :scroll-x="1280" size="small" />
        </div>
      </NModal>
    </template>
  </div>
</template>
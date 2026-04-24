<script setup lang="ts">
import type { DataTableColumns } from 'naive-ui'
import type { ActiveWorkflowType, WorkflowOwnerType, WorkflowSourceKind, WorkflowTargetKind, WorkflowType } from '@/types/workflow'

import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, reactive, ref, watch } from 'vue'
import { useRouter } from 'vue-router'

import { cloneWorkflow, createWorkflow, deleteWorkflow, getWorkflows, updateWorkflow } from '@/api/workflow'
import { useConfirmActionDialog } from '@/composables/useConfirmActionDialog'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'
import { PRODUCT_FEATURE_KEYS } from '@/constants/product'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'
import { WORKFLOW_OWNER_TYPES, WORKFLOW_SOURCE_KINDS, WORKFLOW_TARGET_KINDS, WORKFLOW_TYPES } from '@/types/workflow'

interface WorkflowItem {
  id: number
  name: string
  description?: string
  workflow_type: WorkflowType
  source_kind: WorkflowSourceKind
  target_kind: WorkflowTargetKind
  is_legacy: boolean
  validation_message?: string
  owner_type: WorkflowOwnerType
  owner_id: number
  source_id?: number
  is_published: boolean
  nodes?: Array<{ id: number, label?: string, node_type?: string, enabled?: boolean, is_fixed?: boolean }>
  created_at?: string
  updated_at?: string
}

const router = useRouter()
const appStore = useAppStore()
const userStore = useUserStore()
const message = useMessage()
const confirmAction = useConfirmActionDialog()
const confirmDelete = useDeleteConfirmDialog()
const loading = ref(false)
const creating = ref(false)
const cloningId = ref<number | null>(null)
const deletingId = ref<number | null>(null)
const createVisible = ref(false)
const scope = ref<'all' | 'system' | 'user'>('all')
const lineage = ref<'all' | 'derived'>('all')
const scenario = ref<'all' | ActiveWorkflowType | 'legacy'>('all')
const keyword = ref('')
const items = ref<WorkflowItem[]>([])
const systemTemplates = ref<WorkflowItem[]>([])
const userWorkflows = ref<WorkflowItem[]>([])
const form = reactive({
  name: '',
  description: '',
  owner_type: WORKFLOW_OWNER_TYPES.USER as WorkflowOwnerType,
  workflow_type: WORKFLOW_TYPES.BATCH as ActiveWorkflowType,
})

const workflowScenarioOptions = computed<Array<{ value: ActiveWorkflowType, label: string, description: string }>>(() => {
  const options: Array<{ value: ActiveWorkflowType, label: string, description: string }> = [
  {
    value: WORKFLOW_TYPES.BATCH,
    label: '批量转写整理',
    description: '自动固化非实时 ASR 源节点，后续只编辑中间处理链路。',
  },
  {
    value: WORKFLOW_TYPES.REALTIME,
    label: '实时转写整理',
    description: '自动固化实时 ASR 源节点，适合实时识别后的整理链路。',
  },
  ]
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.MEETING)) {
    options.push({
      value: WORKFLOW_TYPES.MEETING,
      label: '会议纪要',
      description: '自动固化首个 ASR 节点和末尾会议纪要节点。',
    })
  }
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICE_CONTROL)) {
    options.push({
      value: WORKFLOW_TYPES.VOICE_CONTROL,
      label: '语音控制',
      description: '固化唤醒词识别源节点与意图识别输出节点，用于终端语音控制。',
    })
  }
  return options
})

const workflowFilterOptions = computed<Array<{ value: 'all' | ActiveWorkflowType | 'legacy', label: string }>>(() => {
  const options: Array<{ value: 'all' | ActiveWorkflowType | 'legacy', label: string }> = [
    { value: 'all', label: '全部场景' },
    { value: WORKFLOW_TYPES.BATCH, label: '批量转写' },
    { value: WORKFLOW_TYPES.REALTIME, label: '实时转写' },
  ]
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.MEETING))
    options.push({ value: WORKFLOW_TYPES.MEETING, label: '会议纪要' })
  if (appStore.hasCapability(PRODUCT_FEATURE_KEYS.VOICE_CONTROL))
    options.push({ value: WORKFLOW_TYPES.VOICE_CONTROL, label: '语音控制' })
  options.push({ value: WORKFLOW_TYPES.LEGACY, label: 'Legacy' })
  return options
})

const isAdmin = computed(() => userStore.profile?.role === 'admin')
const workflowLookup = computed(() => {
  const map = new Map<number, WorkflowItem>()
  for (const item of [...systemTemplates.value, ...userWorkflows.value, ...items.value]) {
    map.set(item.id, item)
  }
  return map
})

function sourceWorkflowLabel(sourceId?: number) {
  if (!sourceId)
    return '-'
  const item = workflowLookup.value.get(sourceId)
  return item ? `${item.name} #${item.id}` : `#${sourceId}`
}

function sourceWorkflowMeta(sourceId?: number) {
  if (!sourceId)
    return null
  const item = workflowLookup.value.get(sourceId)
  if (!item) {
    return {
      label: `#${sourceId}`,
      ownerLabel: '来源未知',
      ownerType: 'warning',
    }
  }
  return {
    label: sourceWorkflowLabel(sourceId),
    ownerLabel: item.owner_type === 'system' ? '系统模板' : '用户工作流',
    ownerType: item.owner_type === 'system' ? 'success' : 'default',
  }
}

function formatDateTime(value?: string) {
  if (!value)
    return '-'
  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value
  return date.toLocaleString('zh-CN', { hour12: false })
}

function workflowTypeLabel(value?: WorkflowItem['workflow_type']) {
  const map: Record<string, string> = {
    [WORKFLOW_TYPES.LEGACY]: '旧版',
    [WORKFLOW_TYPES.BATCH]: '批量转写',
    [WORKFLOW_TYPES.REALTIME]: '实时转写',
    [WORKFLOW_TYPES.MEETING]: '会议纪要',
    [WORKFLOW_TYPES.VOICE_CONTROL]: '语音控制',
  }
  return map[value || ''] || value || '-'
}

function workflowProfileLabel(row: WorkflowItem) {
  if (row.is_legacy)
    return 'Legacy 文本后处理'

  const sourceMap: Record<string, string> = {
    [WORKFLOW_SOURCE_KINDS.LEGACY_TEXT]: '文本',
    [WORKFLOW_SOURCE_KINDS.BATCH_ASR]: '批量 ASR',
    [WORKFLOW_SOURCE_KINDS.REALTIME_ASR]: '实时 ASR',
    [WORKFLOW_SOURCE_KINDS.VOICE_WAKE]: '唤醒词',
  }
  const targetMap: Record<string, string> = {
    [WORKFLOW_TARGET_KINDS.TRANSCRIPT]: '整理文本',
    [WORKFLOW_TARGET_KINDS.MEETING_SUMMARY]: '会议纪要',
    [WORKFLOW_TARGET_KINDS.VOICE_COMMAND]: '控制指令',
  }
  return `${sourceMap[row.source_kind] || row.source_kind} -> ${targetMap[row.target_kind] || row.target_kind}`
}

function countFixedNodes(row: WorkflowItem) {
  return (row.nodes || []).filter(node => node.is_fixed).length
}

function publishStatusMeta(item: WorkflowItem) {
  if (item.owner_type === 'system') {
    return item.is_published
      ? { label: '已上架', type: 'success', actionLabel: '下架模板' }
      : { label: '未上架', type: 'default', actionLabel: '上架模板' }
  }

  return item.is_published
    ? { label: '已发布', type: 'success', actionLabel: '取消发布' }
    : { label: '草稿', type: 'warning', actionLabel: '发布' }
}

function resetCreateForm() {
  form.name = ''
  form.description = ''
  form.owner_type = 'user'
  form.workflow_type = WORKFLOW_TYPES.BATCH
}

async function loadWorkflows() {
  loading.value = true
  try {
    const params = scope.value === 'all' ? { offset: 0, limit: 200 } : { offset: 0, limit: 200, scope: scope.value }
    const result = await getWorkflows(params)
    items.value = result.data.items || []

    const [systemResult, userResult] = await Promise.allSettled([
      getWorkflows({ offset: 0, limit: 200, scope: 'system' }),
      getWorkflows({ offset: 0, limit: 200, scope: 'user' }),
    ])

    if (systemResult.status === 'fulfilled')
      systemTemplates.value = systemResult.value.data.items || []
    if (userResult.status === 'fulfilled')
      userWorkflows.value = userResult.value.data.items || []
  }
  catch {
    message.error('工作流列表加载失败')
  }
  finally {
    loading.value = false
  }
}

async function handleCreate() {
  if (!form.name.trim()) {
    message.warning('请填写工作流名称')
    return
  }

  creating.value = true
  try {
    const ownerType = form.owner_type
    const result = await createWorkflow({
      name: form.name.trim(),
      description: form.description.trim(),
      owner_type: ownerType,
      workflow_type: form.workflow_type,
    })
    createVisible.value = false
    resetCreateForm()
    message.success(ownerType === 'system' ? '系统模板已创建' : '工作流已创建')
    await loadWorkflows()
    if (result.data?.id)
      router.push(`/workflows/${result.data.id}`)
  }
  catch {
    message.error('工作流创建失败')
  }
  finally {
    creating.value = false
  }
}

async function handleClone(row: WorkflowItem) {
  cloningId.value = row.id
  try {
    const result = await cloneWorkflow(row.id)
    message.success('工作流已克隆到我的工作流')
    await loadWorkflows()
    if (result.data?.id)
      router.push(`/workflows/${result.data.id}`)
  }
  catch {
    message.error('工作流克隆失败')
  }
  finally {
    cloningId.value = null
  }
}

async function handleDelete(row: WorkflowItem) {
  const label = row.owner_type === 'system' ? '系统模板' : '工作流'
  const confirmed = await confirmDelete({
    entityType: label,
    entityName: row.name,
    description: row.owner_type === 'system'
      ? '删除后，这条系统模板将从模板列表中移除，已有副本不会自动删除。'
      : '删除后，这条工作流及其节点配置会一并移除，已有执行记录不会自动回滚。',
  })
  if (!confirmed)
    return

  deletingId.value = row.id
  try {
    await deleteWorkflow(row.id)
    message.success(`${label}已删除`)
    await loadWorkflows()
  }
  catch {
    message.error(`${label}删除失败`)
  }
  finally {
    deletingId.value = null
  }
}

async function handleTogglePublish(row: WorkflowItem) {
  const nextPublished = !row.is_published
  if (!nextPublished) {
    const label = row.owner_type === 'system' ? '系统模板' : '工作流'
    const confirmed = await confirmAction({
      title: `确认${publishStatusMeta(row).actionLabel}`,
      message: `将要${publishStatusMeta(row).actionLabel}${label}「${row.name}」。`,
      description: '取消发布后，这条内容会从可发布工作流视图中隐藏；已有副本和执行记录不会被删除。',
      positiveText: publishStatusMeta(row).actionLabel,
      positiveType: 'warning',
    })
    if (!confirmed)
      return
  }

  try {
    await updateWorkflow(row.id, {
      name: row.name,
      description: row.description,
      is_published: nextPublished,
    })
    row.is_published = nextPublished
    message.success(row.owner_type === 'system'
      ? (row.is_published ? '系统模板已上架' : '系统模板已下架')
      : (row.is_published ? '工作流已发布' : '工作流已转为草稿'))
  }
  catch {
    message.error('发布状态更新失败')
  }
}

function handleScopeChange(nextScope: 'all' | 'system' | 'user') {
  scope.value = nextScope
}

const filteredItems = computed(() => {
  let baseItems = lineage.value === 'derived'
    ? items.value.filter(item => !!item.source_id)
    : items.value

  if (scenario.value !== 'all') {
    baseItems = baseItems.filter((item) => {
      if (scenario.value === 'legacy')
        return item.is_legacy
      return item.workflow_type === scenario.value
    })
  }

  const value = keyword.value.trim().toLowerCase()
  if (!value)
    return baseItems
  return baseItems.filter(item =>
    String(item.id).includes(value)
    || item.name.toLowerCase().includes(value)
    || (item.description || '').toLowerCase().includes(value),
  )
})

const derivedCount = computed(() => filteredItems.value.filter(item => !!item.source_id).length)
const scenarioWorkflowCount = computed(() => filteredItems.value.filter(item => !item.is_legacy).length)
const fixedNodeCount = computed(() => filteredItems.value.reduce((sum, item) => sum + countFixedNodes(item), 0))

const columns: DataTableColumns<WorkflowItem> = [
  {
    title: '名称',
    key: 'name',
    minWidth: 220,
    render: row => h('div', { class: 'min-w-0' }, [
      h('div', { class: 'truncate text-sm font-700 text-ink' }, row.name),
      h('div', { class: 'mt-1 truncate text-xs text-slate' }, row.description || '暂无描述'),
    ]),
  },
  {
    title: '归属',
    key: 'owner_type',
    width: 120,
    render: row => h(NTag, { size: 'small', round: true, bordered: false, type: row.owner_type === 'system' ? 'success' : 'default' }, {
      default: () => row.owner_type === 'system' ? '系统模板' : '我的工作流',
    }),
  },
  {
    title: '类型',
    key: 'workflow_type',
    width: 120,
    render: row => h(NTag, { size: 'small', round: true, bordered: false, type: row.is_legacy ? 'warning' : 'info' }, {
      default: () => workflowTypeLabel(row.workflow_type),
    }),
  },
  {
    title: '节点数',
    key: 'nodes',
    width: 90,
    render: row => String(row.nodes?.length || 0),
  },
  {
    title: '固化',
    key: 'fixed_nodes',
    width: 90,
    render: row => {
      const count = countFixedNodes(row)
      return count > 0 ? `${count} 个` : '-'
    },
  },
  {
    title: '画像',
    key: 'profile',
    width: 170,
    render: row => h('div', { class: 'min-w-0 text-xs leading-6 text-slate' }, workflowProfileLabel(row)),
  },
  {
    title: '来源',
    key: 'source_id',
    width: 220,
    render: (row) => {
      if (!row.source_id)
        return '-'
      const source = sourceWorkflowMeta(row.source_id)
      return h(NButton, {
        text: true,
        size: 'small',
        type: 'primary',
        onClick: () => router.push(`/workflows/${row.source_id}`),
      }, {
        default: () => h('div', { class: 'min-w-0 text-left' }, [
          h('div', { class: 'truncate text-sm text-ink' }, `分叉自 ${source?.label || `#${row.source_id}`}`),
          h(NTag, {
            size: 'small',
            round: true,
            bordered: false,
            type: source?.ownerType as any,
            class: 'mt-1',
          }, { default: () => source?.ownerLabel || '来源未知' }),
        ]),
      })
    },
  },
  {
    title: '发布',
    key: 'is_published',
    width: 110,
    render: (row) => {
      const meta = publishStatusMeta(row)
      return h(NTag, { size: 'small', round: true, bordered: false, type: meta.type as any }, {
        default: () => meta.label,
      })
    },
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 180,
    render: row => formatDateTime(row.updated_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render: row => h('div', { class: 'flex flex-wrap items-center gap-1' }, [
      h(NButton, { text: true, type: 'primary', size: 'small', onClick: () => router.push(`/workflows/${row.id}`) }, { default: () => '编辑' }),
      h(NButton, { text: true, size: 'small', loading: cloningId.value === row.id, onClick: () => handleClone(row) }, { default: () => '克隆' }),
      h(NButton, { text: true, size: 'small', onClick: () => handleTogglePublish(row) }, { default: () => publishStatusMeta(row).actionLabel }),
      h(NButton, { text: true, size: 'small', type: 'error', loading: deletingId.value === row.id, onClick: () => handleDelete(row) }, { default: () => '删除' }),
    ]),
  },
]

watch(scope, () => {
  loadWorkflows()
})

watch(createVisible, (visible) => {
  if (!visible)
    resetCreateForm()
})

watch(workflowScenarioOptions, (options) => {
  if (!options.some(option => option.value === form.workflow_type))
    form.workflow_type = WORKFLOW_TYPES.BATCH
})

watch(workflowFilterOptions, (options) => {
  if (!options.some(option => option.value === scenario.value))
    scenario.value = 'all'
})

onMounted(loadWorkflows)
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-col gap-5 overflow-x-hidden overflow-y-auto pr-1">
    <NCard class="card-main shrink-0">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-sm font-600">工作流列表</span>
          <div class="flex flex-wrap items-center gap-2">
            <NButton size="small" @click="router.push('/workflows/nodes')">
              节点管理
            </NButton>
            <div class="flex items-center gap-1 rounded-full bg-mist/80 p-1">
              <NButton size="small" :type="scope === 'all' ? 'primary' : 'default'" :quaternary="scope !== 'all'" color="#0f766e" @click="handleScopeChange('all')">
                全部
              </NButton>
              <NButton size="small" :type="scope === 'system' ? 'primary' : 'default'" :quaternary="scope !== 'system'" color="#0f766e" @click="handleScopeChange('system')">
                系统模板
              </NButton>
              <NButton size="small" :type="scope === 'user' ? 'primary' : 'default'" :quaternary="scope !== 'user'" color="#0f766e" @click="handleScopeChange('user')">
                我的工作流
              </NButton>
            </div>
            <NInput v-model:value="keyword" clearable size="small" placeholder="搜索名称 / 描述 / ID" class="w-full sm:!w-56" />
            <NButton quaternary size="small" @click="loadWorkflows">
              刷新
            </NButton>
            <NButton size="small" type="primary" color="#0f766e" @click="createVisible = true">
              新建工作流
            </NButton>
          </div>
        </div>
      </template>
      <div class="grid gap-3 text-sm text-slate md:grid-cols-3 xl:grid-cols-5">
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            系统模板
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ systemTemplates.length }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            我的工作流
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ userWorkflows.length }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            可见中
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ systemTemplates.filter(item => item.is_published).length + userWorkflows.filter(item => item.is_published).length }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            当前场景流
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ scenarioWorkflowCount }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            当前固化节点
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ fixedNodeCount }}
          </div>
        </div>
      </div>
    </NCard>

    <NCard class="card-main min-h-[360px] flex-1 overflow-hidden" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-sm font-600">
              工作流清单
            </div>
            <div class="mt-1 text-xs text-slate">
              当前显示 {{ filteredItems.length }} 条，其中派生工作流 {{ derivedCount }} 条。
            </div>
          </div>
          <div class="flex flex-wrap items-center justify-end gap-2">
            <div class="flex items-center gap-1 rounded-full bg-mist/80 p-1">
              <NButton size="small" :type="lineage === 'all' ? 'primary' : 'default'" :quaternary="lineage !== 'all'" color="#0f766e" @click="lineage = 'all'">
                全部来源
              </NButton>
              <NButton size="small" :type="lineage === 'derived' ? 'primary' : 'default'" :quaternary="lineage !== 'derived'" color="#0f766e" @click="lineage = 'derived'">
                仅看派生
              </NButton>
            </div>
            <div class="flex flex-wrap items-center gap-1 rounded-2xl bg-[#fbfdff] p-1">
              <NButton
                v-for="option in workflowFilterOptions"
                :key="option.value"
                size="small"
                :type="scenario === option.value ? 'primary' : 'default'"
                :quaternary="scenario !== option.value"
                color="#0f766e"
                @click="scenario = option.value"
              >
                {{ option.label }}
              </NButton>
            </div>
          </div>
        </div>
      </template>
      <NDataTable
        flex-height
        class="flex-1 min-h-0"
        :columns="columns"
        :data="filteredItems"
        :loading="loading"
        :pagination="{ pageSize: 10 }"
        :scroll-x="920"
        size="small"
      />
    </NCard>

    <NModal v-model:show="createVisible" preset="card" title="新建工作流" class="modal-card max-w-xl">
      <div class="grid gap-4">
        <div v-if="isAdmin" class="rounded-3 border border-black/6 bg-mist/70 p-3">
          <div class="text-xs font-600 text-ink">
            创建归属
          </div>
          <div class="mt-2 flex flex-wrap gap-2">
            <NButton size="small" :type="form.owner_type === 'user' ? 'primary' : 'default'" color="#0f766e" @click="form.owner_type = 'user'">
              我的工作流
            </NButton>
            <NButton size="small" :type="form.owner_type === 'system' ? 'primary' : 'default'" color="#0f766e" @click="form.owner_type = 'system'">
              系统模板
            </NButton>
          </div>
          <div class="mt-2 text-xs text-slate">
            {{ form.owner_type === 'system'
              ? '系统模板会进入模板列表，需上架后才会对普通用户可见。'
              : '我的工作流只归当前账号所有，可直接继续编辑和发布。' }}
          </div>
        </div>
        <NInput v-model:value="form.name" placeholder="例如：医疗转写纠错模板" />
        <NInput v-model:value="form.description" type="textarea" :autosize="{ minRows: 4, maxRows: 8 }" placeholder="描述这条工作流的用途、目标场景和关键节点。" />
        <div class="rounded-3 border border-black/6 bg-[#fbfdff] p-3">
          <div class="text-xs font-600 text-ink">
            选择工作流场景
          </div>
          <div class="mt-3 grid gap-2 md:grid-cols-3">
            <button
              v-for="option in workflowScenarioOptions"
              :key="option.value"
              type="button"
              class="rounded-2 border px-3 py-3 text-left transition-all duration-150"
              :class="form.workflow_type === option.value ? 'border-teal bg-teal/[0.08] shadow-sm' : 'border-gray-200 bg-white hover:border-gray-300'"
              @click="form.workflow_type = option.value"
            >
              <div class="text-sm font-600 text-ink">
                {{ option.label }}
              </div>
              <div class="mt-1 text-xs leading-5 text-slate">
                {{ option.description }}
              </div>
            </button>
          </div>
          <div class="mt-2 text-xs text-slate">
            创建后会按场景自动固化必要节点，编辑时不能删除、移动或关闭这些边界节点。
          </div>
        </div>
        <div class="flex justify-end gap-2">
          <NButton @click="createVisible = false">
            取消
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="creating" @click="handleCreate">
            {{ form.owner_type === 'system' ? '创建模板并编辑' : '创建并编辑' }}
          </NButton>
        </div>
      </div>
    </NModal>
  </div>
</template>

<script setup lang="ts">
import type { VoiceCommandDictItem, VoiceCommandEntryItem } from '@/api/voiceCommands'

import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, reactive, ref } from 'vue'

import {
  createVoiceCommandDict,
  createVoiceCommandEntry,
  deleteVoiceCommandDict,
  deleteVoiceCommandEntry,
  getVoiceCommandDicts,
  getVoiceCommandEntries,
  updateVoiceCommandDict,
  updateVoiceCommandEntry,
} from '@/api/voiceCommands'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'
import {
  buildVoiceCommandGroupOptions,
  buildVoiceCommandIntentOptions,
  findBuiltinVoiceCommandGroup,
  findBuiltinVoiceCommandIntent,
  normalizeVoiceCommandGroupKey,
} from '@/constants/voiceCommands'

const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()
const loading = ref(false)
const entryLoading = ref(false)
const dictSaving = ref(false)
const entrySaving = ref(false)
const deletingDictId = ref<number | null>(null)
const deletingEntryId = ref<number | null>(null)
const showDictModal = ref(false)
const showEntryModal = ref(false)
const editingDictId = ref<number | null>(null)
const editingEntryId = ref<number | null>(null)
const currentDictId = ref<number | null>(null)
const dictKeyword = ref('')
const dictTypeFilter = ref<'all' | 'base' | 'custom'>('all')
const entryKeyword = ref('')
const dicts = ref<VoiceCommandDictItem[]>([])
const entries = ref<VoiceCommandEntryItem[]>([])

const dictForm = reactive({
  name: '',
  groupKey: '',
  description: '',
  isBase: false,
})

const entryForm = reactive({
  intent: '',
  label: '',
  utterancesText: '',
  enabled: true,
  sortOrder: 0,
})

const currentDict = computed(() => dicts.value.find(item => item.id === currentDictId.value) || null)
const dictModalTitle = computed(() => editingDictId.value ? '编辑控制指令组' : '新建控制指令组')
const entryModalTitle = computed(() => editingEntryId.value ? '编辑控制指令' : '新增控制指令')
const dictTypeOptions = [
  { label: '全部类型', value: 'all' },
  { label: '基础组', value: 'base' },
  { label: '扩展组', value: 'custom' },
]
const groupKeyOptions = buildVoiceCommandGroupOptions()
const currentGroupSpec = computed(() => findBuiltinVoiceCommandGroup(currentDict.value?.group_key))
const entryIntentOptions = computed(() => buildVoiceCommandIntentOptions(currentDict.value?.group_key))

const filteredDicts = computed(() => {
  const keyword = dictKeyword.value.trim().toLowerCase()
  return dicts.value.filter((item) => {
    if (dictTypeFilter.value === 'base' && !item.is_base)
      return false
    if (dictTypeFilter.value === 'custom' && item.is_base)
      return false
    if (!keyword)
      return true
    return item.name.toLowerCase().includes(keyword)
      || item.group_key.toLowerCase().includes(keyword)
      || (item.description || '').toLowerCase().includes(keyword)
  })
})

const filteredEntries = computed(() => {
  const keyword = entryKeyword.value.trim().toLowerCase()
  if (!keyword)
    return entries.value
  return entries.value.filter(item => item.label.toLowerCase().includes(keyword)
    || item.intent.toLowerCase().includes(keyword)
    || item.utterances.some(text => text.toLowerCase().includes(keyword)))
})

const dictColumns = [
  { title: '分组名称', key: 'name' },
  { title: '分组键', key: 'group_key' },
  {
    title: '类型',
    key: 'is_base',
    render: (row: VoiceCommandDictItem) => h('div', { class: 'flex items-center gap-2' }, [
      h(NTag, {
        size: 'small',
        round: true,
        bordered: false,
        type: row.is_base ? 'warning' : 'info',
      }, { default: () => row.is_base ? '基础组' : '扩展组' }),
      row.is_base
        ? h(NTag, { size: 'small', round: true, bordered: false, type: 'success' }, { default: () => '默认附加' })
        : null,
    ]),
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render: (row: VoiceCommandDictItem) => h('div', { class: 'flex items-center gap-2' }, [
      row.id === currentDictId.value
        ? h(NTag, { size: 'small', round: true, bordered: false, type: 'success' }, { default: () => '当前分组' })
        : h(NButton, { text: true, type: 'primary', size: 'small', onClick: () => selectDict(row.id) }, { default: () => '查看' }),
      h(NButton, { text: true, size: 'small', onClick: () => openEditDictModal(row) }, { default: () => '编辑' }),
      row.is_base
        ? h(NTag, { size: 'small', round: true, bordered: false, type: 'warning' }, { default: () => '受保护' })
        : h(NButton, {
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
  {
    title: '意图值',
    key: 'intent',
    render: (row: VoiceCommandEntryItem) => {
      const intent = findBuiltinVoiceCommandIntent(row.intent, currentDict.value?.group_key)
      return h('div', { class: 'flex flex-col gap-1' }, [
        h('span', { class: 'font-500 text-ink' }, row.intent),
        intent ? h('span', { class: 'text-xs text-slate/70' }, intent.handlerName) : null,
      ])
    },
  },
  { title: '展示名称', key: 'label' },
  {
    title: '候选话术',
    key: 'utterances',
    render: (row: VoiceCommandEntryItem) => row.utterances.join(' / '),
  },
  {
    title: '状态',
    key: 'enabled',
    render: (row: VoiceCommandEntryItem) => h(NTag, {
      size: 'small',
      round: true,
      bordered: false,
      type: row.enabled ? 'success' : 'default',
    }, { default: () => row.enabled ? '启用' : '停用' }),
  },
  {
    title: '排序',
    key: 'sort_order',
    width: 90,
  },
  {
    title: '操作',
    key: 'actions',
    width: 180,
    render: (row: VoiceCommandEntryItem) => h('div', { class: 'flex items-center gap-2' }, [
      h(NButton, { text: true, size: 'small', onClick: () => openEditEntryModal(row) }, { default: () => '编辑' }),
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

function resetDictForm() {
  editingDictId.value = null
  dictForm.name = ''
  dictForm.groupKey = groupKeyOptions[0]?.value || ''
  dictForm.description = ''
  dictForm.isBase = false
}

function resetEntryForm() {
  editingEntryId.value = null
  entryForm.intent = ''
  entryForm.label = ''
  entryForm.utterancesText = ''
  entryForm.enabled = true
  entryForm.sortOrder = 0
}

function textToList(value: string) {
  return value.split(/\n|,|，/).map(item => item.trim()).filter(Boolean)
}

function listToText(value: string[]) {
  return Array.isArray(value) ? value.join('\n') : ''
}

async function loadDicts() {
  loading.value = true
  try {
    const result = await getVoiceCommandDicts({ offset: 0, limit: 100 })
    dicts.value = result.data.items || []
    if (dicts.value.length === 0) {
      currentDictId.value = null
      entries.value = []
      return
    }
    const nextDictId = currentDictId.value && dicts.value.some(item => item.id === currentDictId.value)
      ? currentDictId.value
      : dicts.value[0].id
    await selectDict(nextDictId)
  }
  catch {
    message.error('控制指令组加载失败')
  }
  finally {
    loading.value = false
  }
}

async function selectDict(dictId: number) {
  currentDictId.value = dictId
  entryLoading.value = true
  try {
    const result = await getVoiceCommandEntries(dictId)
    entries.value = result.data || []
  }
  catch {
    message.error('控制指令加载失败')
  }
  finally {
    entryLoading.value = false
  }
}

function openCreateDictModal() {
  resetDictForm()
  showDictModal.value = true
}

function openEditDictModal(row: VoiceCommandDictItem) {
  editingDictId.value = row.id
  dictForm.name = row.name
  dictForm.groupKey = normalizeVoiceCommandGroupKey(row.group_key)
  dictForm.description = row.description || ''
  dictForm.isBase = row.is_base
  showDictModal.value = true
}

function openCreateEntryModal() {
  resetEntryForm()
  showEntryModal.value = true
}

function openEditEntryModal(row: VoiceCommandEntryItem) {
  editingEntryId.value = row.id
  entryForm.intent = findBuiltinVoiceCommandIntent(row.intent, currentDict.value?.group_key)?.key || row.intent
  entryForm.label = row.label
  entryForm.utterancesText = listToText(row.utterances)
  entryForm.enabled = row.enabled
  entryForm.sortOrder = row.sort_order
  showEntryModal.value = true
}

function handleIntentChange(value: string | null) {
  entryForm.intent = value || ''
  const intent = findBuiltinVoiceCommandIntent(value, currentDict.value?.group_key)
  if (intent)
    entryForm.label = intent.defaultLabel
}

async function handleSubmitDict() {
  if (!dictForm.name.trim() || !dictForm.groupKey.trim()) {
    message.warning('请填写分组名称和分组键')
    return
  }
  dictSaving.value = true
  try {
    const payload = {
      name: dictForm.name.trim(),
      group_key: dictForm.groupKey.trim(),
      description: dictForm.description.trim(),
      is_base: dictForm.isBase,
    }
    if (editingDictId.value) {
      await updateVoiceCommandDict(editingDictId.value, payload)
      message.success('控制指令组更新成功')
    }
    else {
      await createVoiceCommandDict(payload)
      message.success('控制指令组创建成功')
    }
    showDictModal.value = false
    resetDictForm()
    await loadDicts()
  }
  catch {
    message.error(editingDictId.value ? '控制指令组更新失败' : '控制指令组创建失败')
    if (editingDictId.value || dictForm.isBase)
      message.warning('若提示基础组冲突，请直接编辑现有基础控制指令组。')
  }
  finally {
    dictSaving.value = false
  }
}

async function handleDeleteDict(row: VoiceCommandDictItem) {
  const confirmed = await confirmDelete({
    entityType: '控制指令组',
    entityName: row.name,
    description: '删除后，该分组下的全部控制指令会一并删除，且无法恢复。',
  })
  if (!confirmed)
    return
  deletingDictId.value = row.id
  try {
    await deleteVoiceCommandDict(row.id)
    message.success('控制指令组已删除')
    if (currentDictId.value === row.id) {
      currentDictId.value = null
      entries.value = []
    }
    await loadDicts()
  }
  catch {
    message.error('控制指令组删除失败')
    if (row.is_base)
      message.warning('基础控制指令组受保护，不允许删除。')
  }
  finally {
    deletingDictId.value = null
  }
}

async function handleSubmitEntry() {
  if (!currentDictId.value) {
    message.warning('请先选择控制指令组')
    return
  }
  if (!entryForm.intent.trim() || !entryForm.label.trim()) {
    message.warning('请填写意图值和展示名称')
    return
  }
  const utterances = textToList(entryForm.utterancesText)
  if (utterances.length === 0) {
    message.warning('请至少填写一条候选话术')
    return
  }
  entrySaving.value = true
  try {
    const payload = {
      intent: entryForm.intent.trim(),
      label: entryForm.label.trim(),
      utterances,
      enabled: entryForm.enabled,
      sort_order: Math.round(entryForm.sortOrder || 0),
    }
    if (editingEntryId.value) {
      await updateVoiceCommandEntry(currentDictId.value, editingEntryId.value, payload)
      message.success('控制指令更新成功')
    }
    else {
      await createVoiceCommandEntry(currentDictId.value, payload)
      message.success('控制指令创建成功')
    }
    showEntryModal.value = false
    resetEntryForm()
    await selectDict(currentDictId.value)
  }
  catch {
    message.error(editingEntryId.value ? '控制指令更新失败' : '控制指令创建失败')
  }
  finally {
    entrySaving.value = false
  }
}

async function handleDeleteEntry(row: VoiceCommandEntryItem) {
  if (!currentDictId.value)
    return
  const confirmed = await confirmDelete({
    entityType: '控制指令',
    entityName: row.label,
    description: '删除后，该控制指令将不再参与语音控制意图识别。',
  })
  if (!confirmed)
    return
  deletingEntryId.value = row.id
  try {
    await deleteVoiceCommandEntry(currentDictId.value, row.id)
    message.success('控制指令已删除')
    await selectDict(currentDictId.value)
  }
  catch {
    message.error('控制指令删除失败')
  }
  finally {
    deletingEntryId.value = null
  }
}

onMounted(loadDicts)
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-5">
    <div class="rounded-3 bg-[rgba(37,99,235,0.08)] px-4 py-3 text-sm leading-6 text-ink">
      <div class="font-600 text-[#2563eb]">
        控制指令库规则
      </div>
      <div class="mt-1 text-xs text-slate/75">
        基础控制指令组会自动附加到 voice_intent 节点。扩展组则由具体 workflow 节点按“有效分组”选择；节点提示词可再额外追加当前流程的特殊约束。
      </div>
    </div>

    <div class="grid flex-1 min-h-0 grid-cols-1 gap-5 xl:grid-cols-[0.92fr_1.08fr]">
      <NCard class="card-main flex min-h-0 flex-col" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-sm font-600">
                控制指令组
              </div>
              <div class="mt-1 text-xs text-slate/70">
                每个分组代表一类可命中的控制流程，例如场景切换、录音控制或业务快捷指令。
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <NInput v-model:value="dictKeyword" clearable size="small" placeholder="搜索分组名称 / 分组键 / 说明" class="w-full sm:!w-64" />
              <NSelect v-model:value="dictTypeFilter" size="small" :options="dictTypeOptions" class="w-full sm:!w-34" />
              <NButton quaternary size="small" @click="loadDicts">
                刷新
              </NButton>
      			  <NButton size="small" type="primary" color="#0f766e" @click="openCreateDictModal">
                新建分组
              </NButton>
            </div>
          </div>
        </template>
        <NDataTable flex-height class="flex-1 min-h-0" :columns="dictColumns" :data="filteredDicts" :loading="loading" :pagination="false" size="small" />
      </NCard>

      <NCard class="card-main flex min-h-0 flex-col" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div class="flex items-center gap-2">
              <span class="text-sm font-600">{{ currentDict ? `${currentDict.name} 指令列表` : '控制指令列表' }}</span>
              <NTag v-if="currentDict" round size="small" :type="currentDict.is_base ? 'warning' : 'info'">
                {{ currentDict.is_base ? '基础组' : currentGroupSpec?.name || currentDict.group_key }}
              </NTag>
              <NTag v-if="currentDict?.is_base" round size="small" type="success">
                默认附加
              </NTag>
            </div>
            <div class="flex items-center gap-2">
              <NInput v-model:value="entryKeyword" clearable size="small" placeholder="搜索意图 / 话术" class="w-full sm:!w-56" />
              <NButton :disabled="!currentDictId" quaternary size="small" @click="currentDictId && selectDict(currentDictId)">
                刷新
              </NButton>
              <NButton :disabled="!currentDictId" quaternary size="small" @click="openCreateEntryModal">
                新增指令
              </NButton>
            </div>
          </div>
        </template>
        <div v-if="currentDict?.is_base" class="mb-3 rounded-2 bg-[rgba(245,158,11,0.1)] px-3 py-2 text-xs leading-5 text-[#9a6700]">
          当前查看的是基础控制指令组。它会自动参与所有 voice_intent 节点的识别，仅建议放通用且高置信的控制命令。
        </div>
        <NDataTable flex-height class="flex-1 min-h-0" :columns="entryColumns" :data="filteredEntries" :loading="entryLoading" :pagination="{ pageSize: 10 }" size="small" />
      </NCard>
    </div>

    <NModal v-model:show="showDictModal" preset="card" :title="dictModalTitle" class="modal-card max-w-160">
      <NForm :model="dictForm" label-placement="top">
        <NFormItem label="分组名称">
          <NInput v-model:value="dictForm.name" placeholder="如：场景切换控制" />
        </NFormItem>
        <NFormItem label="分组键">
          <NSelect v-model:value="dictForm.groupKey" :options="groupKeyOptions" placeholder="请选择系统注册的分组 key" />
          <div class="mt-2 text-xs leading-5 text-slate/70">
            所有控制分组 key 统一由注册表维护，禁止手填魔法字符串。
          </div>
        </NFormItem>
        <NFormItem label="说明">
          <NInput v-model:value="dictForm.description" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" placeholder="描述该分组适用的控制流程或业务场景" />
        </NFormItem>
        <NFormItem label="分组类型">
          <div class="flex items-center gap-3 rounded-2 bg-white/70 px-3 py-3">
            <NSwitch v-model:value="dictForm.isBase" />
            <span class="text-sm text-ink">设为基础控制指令组</span>
          </div>
          <div class="mt-2 text-xs leading-5 text-slate/70">
            基础控制指令组会默认叠加到所有 voice_intent 节点，系统只允许保留一个基础组，且基础组不允许删除。
          </div>
        </NFormItem>
        <div class="flex justify-end gap-3">
          <NButton @click="showDictModal = false">
            取消
          </NButton>
    		  <NButton type="primary" color="#0f766e" :loading="dictSaving" @click="handleSubmitDict">
            {{ editingDictId ? '保存' : '创建' }}
          </NButton>
        </div>
      </NForm>
    </NModal>

    <NModal v-model:show="showEntryModal" preset="card" :title="entryModalTitle" class="modal-card max-w-140">
      <NForm :model="entryForm" label-placement="top">
        <NFormItem label="所属分组">
          <NInput :value="currentDict?.name || ''" disabled />
        </NFormItem>
        <NFormItem label="意图值">
          <NSelect
            :value="entryForm.intent || null"
            :options="entryIntentOptions"
            placeholder="请选择系统注册的控制 handler"
            @update:value="handleIntentChange"
          />
          <div class="mt-2 text-xs leading-5 text-slate/70">
            下拉项显示的是控制 handler 中文名称；实际保存的是统一注册的接口 key，例如 scene_report_switch。
          </div>
        </NFormItem>
        <NFormItem label="展示名称">
          <NInput v-model:value="entryForm.label" placeholder="如：会议模式" />
        </NFormItem>
        <NFormItem label="候选话术">
          <NInput v-model:value="entryForm.utterancesText" type="textarea" :autosize="{ minRows: 4, maxRows: 8 }" placeholder="每行一条，填写用户可能说出的候选指令话术" />
        </NFormItem>
        <div class="grid gap-4 lg:grid-cols-2">
          <NFormItem label="启用状态">
            <div class="flex items-center gap-3 rounded-2 bg-white/70 px-3 py-3">
              <NSwitch v-model:value="entryForm.enabled" />
              <span class="text-sm text-ink">启用该控制指令</span>
            </div>
          </NFormItem>
          <NFormItem label="排序">
            <NInputNumber v-model:value="entryForm.sortOrder" :step="10" class="w-full" />
          </NFormItem>
        </div>
        <div class="flex justify-end gap-3">
          <NButton @click="showEntryModal = false">
            取消
          </NButton>
    		  <NButton type="primary" color="#0f766e" :loading="entrySaving" @click="handleSubmitEntry">
            {{ editingEntryId ? '保存' : '创建' }}
          </NButton>
        </div>
      </NForm>
    </NModal>
  </div>
</template>
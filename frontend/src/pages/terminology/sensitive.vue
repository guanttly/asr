<script setup lang="ts">
import type { SensitiveDictItem, SensitiveEntryItem } from '@/api/sensitive'
import { NButton, NTag, useMessage } from 'naive-ui'

import { computed, h, onMounted, reactive, ref } from 'vue'
import {
  createSensitiveDict,
  createSensitiveEntry,
  deleteSensitiveDict,
  deleteSensitiveEntry,
  getSensitiveDicts,
  getSensitiveEntries,
  updateSensitiveDict,
  updateSensitiveEntry,

} from '@/api/sensitive'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'

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
const dictTypeFilter = ref<'all' | 'base' | 'scene'>('all')
const sceneFilter = ref<'all' | string>('all')
const entryKeyword = ref('')
const dicts = ref<SensitiveDictItem[]>([])
const entries = ref<SensitiveEntryItem[]>([])
const dictForm = reactive({
  name: '',
  scene: '',
  description: '',
  isBase: false,
})
const entryForm = reactive({
  word: '',
  enabled: true,
})

const currentDict = computed(() => dicts.value.find(item => item.id === currentDictId.value) || null)
const dictModalTitle = computed(() => editingDictId.value ? '编辑敏感词库' : '新建敏感词库')
const entryModalTitle = computed(() => editingEntryId.value ? '编辑敏感词' : '新增敏感词')
const sceneOptions = computed(() => {
  const scenes = Array.from(new Set(dicts.value.map(item => item.scene).filter(Boolean)))
  return [
    { label: '全部场景', value: 'all' },
    ...scenes.map(scene => ({ label: scene, value: scene })),
  ]
})
const dictTypeOptions = [
  { label: '全部类型', value: 'all' },
  { label: '基础库', value: 'base' },
  { label: '场景库', value: 'scene' },
]
const filteredDicts = computed(() => {
  const keyword = dictKeyword.value.trim().toLowerCase()
  return dicts.value.filter((item) => {
    if (dictTypeFilter.value === 'base' && !item.is_base)
      return false
    if (dictTypeFilter.value === 'scene' && item.is_base)
      return false
    if (sceneFilter.value !== 'all' && item.scene !== sceneFilter.value)
      return false
    if (!keyword)
      return true
    return item.name.toLowerCase().includes(keyword)
      || item.scene.toLowerCase().includes(keyword)
      || (item.description || '').toLowerCase().includes(keyword)
  })
})
const filteredEntries = computed(() => {
  const keyword = entryKeyword.value.trim().toLowerCase()
  if (!keyword)
    return entries.value
  return entries.value.filter(item => item.word.toLowerCase().includes(keyword))
})

const dictColumns = [
  { title: '词库名称', key: 'name' },
  { title: '场景', key: 'scene' },
  {
    title: '类型',
    key: 'is_base',
    render: (row: SensitiveDictItem) => h('div', { class: 'flex items-center gap-2' }, [
      h(NTag, {
        size: 'small',
        round: true,
        bordered: false,
        type: row.is_base ? 'warning' : 'info',
      }, { default: () => row.is_base ? '基础库' : '场景库' }),
      row.is_base
        ? h(NTag, {
            size: 'small',
            round: true,
            bordered: false,
            type: 'success',
          }, { default: () => '默认叠加' })
        : null,
    ]),
  },
  {
    title: '操作',
    key: 'actions',
    width: 260,
    render: (row: SensitiveDictItem) => h('div', { class: 'flex items-center gap-2' }, [
      row.id === currentDictId.value
        ? h(NTag, { size: 'small', round: true, bordered: false, type: 'success' }, { default: () => '当前词库' })
        : h(NButton, { text: true, type: 'primary', size: 'small', onClick: () => selectDict(row.id) }, { default: () => '查看' }),
      h(NButton, { text: true, size: 'small', onClick: () => openEditDictModal(row) }, { default: () => '编辑' }),
      row.is_base
        ? h(NTag, {
            size: 'small',
            round: true,
            bordered: false,
            type: 'warning',
          }, { default: () => '受保护' })
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
  { title: '敏感词', key: 'word' },
  {
    title: '状态',
    key: 'enabled',
    render: (row: SensitiveEntryItem) => h(NTag, {
      size: 'small',
      round: true,
      bordered: false,
      type: row.enabled ? 'success' : 'default',
    }, { default: () => row.enabled ? '启用' : '停用' }),
  },
  {
    title: '操作',
    key: 'actions',
    width: 180,
    render: (row: SensitiveEntryItem) => h('div', { class: 'flex items-center gap-2' }, [
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
  dictForm.scene = ''
  dictForm.description = ''
  dictForm.isBase = false
}

function resetEntryForm() {
  editingEntryId.value = null
  entryForm.word = ''
  entryForm.enabled = true
}

async function loadDicts() {
  loading.value = true
  try {
    const result = await getSensitiveDicts({ offset: 0, limit: 100 })
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
    message.error('敏感词库加载失败')
  }
  finally {
    loading.value = false
  }
}

async function selectDict(dictId: number) {
  currentDictId.value = dictId
  entryLoading.value = true
  try {
    const result = await getSensitiveEntries(dictId)
    entries.value = result.data || []
  }
  catch {
    message.error('敏感词条加载失败')
  }
  finally {
    entryLoading.value = false
  }
}

function openCreateDictModal() {
  resetDictForm()
  showDictModal.value = true
}

function openEditDictModal(row: SensitiveDictItem) {
  editingDictId.value = row.id
  dictForm.name = row.name
  dictForm.scene = row.scene
  dictForm.description = row.description || ''
  dictForm.isBase = row.is_base
  showDictModal.value = true
}

function openCreateEntryModal() {
  resetEntryForm()
  showEntryModal.value = true
}

function openEditEntryModal(row: SensitiveEntryItem) {
  editingEntryId.value = row.id
  entryForm.word = row.word
  entryForm.enabled = row.enabled
  showEntryModal.value = true
}

async function handleSubmitDict() {
  if (!dictForm.name.trim() || !dictForm.scene.trim()) {
    message.warning('请填写词库名称和场景')
    return
  }

  dictSaving.value = true
  try {
    const payload = {
      name: dictForm.name.trim(),
      scene: dictForm.scene.trim(),
      description: dictForm.description.trim(),
      is_base: dictForm.isBase,
    }

    if (editingDictId.value) {
      await updateSensitiveDict(editingDictId.value, payload)
      message.success('敏感词库更新成功')
    }
    else {
      await createSensitiveDict(payload)
      message.success('敏感词库创建成功')
    }

    showDictModal.value = false
    resetDictForm()
    await loadDicts()
  }
  catch {
    message.error(editingDictId.value ? '敏感词库更新失败' : '敏感词库创建失败')
    if (editingDictId.value || dictForm.isBase)
      message.warning('若提示基础库冲突，请直接编辑现有基础敏感词库，而不是再创建一个新的基础库。')
  }
  finally {
    dictSaving.value = false
  }
}

async function handleDeleteDict(row: SensitiveDictItem) {
  const confirmed = await confirmDelete({
    entityType: '敏感词库',
    entityName: row.name,
    description: '删除后，该词库下的全部敏感词会一并删除，且无法恢复。',
  })
  if (!confirmed)
    return

  deletingDictId.value = row.id
  try {
    await deleteSensitiveDict(row.id)
    message.success('敏感词库已删除')
    if (currentDictId.value === row.id) {
      currentDictId.value = null
      entries.value = []
    }
    await loadDicts()
  }
  catch {
    message.error('敏感词库删除失败')
    if (row.is_base)
      message.warning('基础敏感词库受保护，不允许删除。')
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
  if (!entryForm.word.trim()) {
    message.warning('请填写敏感词')
    return
  }

  entrySaving.value = true
  try {
    const payload = {
      word: entryForm.word.trim(),
      enabled: entryForm.enabled,
    }

    if (editingEntryId.value) {
      await updateSensitiveEntry(currentDictId.value, editingEntryId.value, payload)
      message.success('敏感词更新成功')
    }
    else {
      await createSensitiveEntry(currentDictId.value, payload)
      message.success('敏感词创建成功')
    }

    showEntryModal.value = false
    resetEntryForm()
    await selectDict(currentDictId.value)
  }
  catch {
    message.error(editingEntryId.value ? '敏感词更新失败' : '敏感词创建失败')
  }
  finally {
    entrySaving.value = false
  }
}

async function handleDeleteEntry(row: SensitiveEntryItem) {
  if (!currentDictId.value)
    return

  const confirmed = await confirmDelete({
    entityType: '敏感词',
    entityName: row.word,
    description: '删除后，该敏感词将不再参与过滤。',
  })
  if (!confirmed)
    return

  deletingEntryId.value = row.id
  try {
    await deleteSensitiveEntry(currentDictId.value, row.id)
    message.success('敏感词已删除')
    await selectDict(currentDictId.value)
  }
  catch {
    message.error('敏感词删除失败')
  }
  finally {
    deletingEntryId.value = null
  }
}

onMounted(loadDicts)
</script>

<template>
  <div class="flex flex-1 min-h-0 flex-col gap-5">
    <div class="rounded-3 bg-[rgba(15,118,110,0.08)] px-4 py-3 text-sm leading-6 text-ink">
      <div class="font-600 text-[#0f766e]">
        基础敏感词库规则
      </div>
      <div class="mt-1 text-xs text-slate/75">
        基础敏感词库会默认叠加到每一个敏感词过滤节点，不需要在工作流里单独选择。基础库属于受保护资源，可编辑内容，但不允许删除。
      </div>
    </div>

    <div class="grid flex-1 min-h-0 grid-cols-1 gap-5 xl:grid-cols-[0.92fr_1.08fr]">
      <NCard class="card-main flex min-h-0 flex-col" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-sm font-600">
                敏感词库
              </div>
              <div class="mt-1 text-xs text-slate/70">
                基础库会自动叠加到每个敏感词过滤节点，场景库按节点单独选择。
              </div>
            </div>
            <div class="flex flex-wrap items-center gap-2">
              <NInput v-model:value="dictKeyword" clearable size="small" placeholder="搜索词库名称 / 场景 / 说明" class="w-full sm:!w-64" />
              <NSelect v-model:value="dictTypeFilter" size="small" :options="dictTypeOptions" class="w-full sm:!w-34" />
              <NSelect v-model:value="sceneFilter" size="small" :options="sceneOptions" class="w-full sm:!w-40" />
              <NButton quaternary size="small" @click="loadDicts">
                刷新
              </NButton>
              <NButton size="small" type="primary" color="#0f766e" @click="openCreateDictModal">
                新建词库
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
              <span class="text-sm font-600">{{ currentDict ? `${currentDict.name} 词条列表` : '敏感词列表' }}</span>
              <NTag v-if="currentDict" round size="small" :type="currentDict.is_base ? 'warning' : 'info'">
                {{ currentDict.is_base ? '基础库' : currentDict.scene }}
              </NTag>
              <NTag v-if="currentDict?.is_base" round size="small" type="success">
                默认叠加到所有节点
              </NTag>
            </div>
            <div class="flex items-center gap-2">
              <NInput v-model:value="entryKeyword" clearable size="small" placeholder="搜索敏感词" class="w-full sm:!w-56" />
              <NButton :disabled="!currentDictId" quaternary size="small" @click="currentDictId && selectDict(currentDictId)">
                刷新
              </NButton>
              <NButton :disabled="!currentDictId" quaternary size="small" @click="openCreateEntryModal">
                新增敏感词
              </NButton>
            </div>
          </div>
        </template>
        <div v-if="currentDict?.is_base" class="mb-3 rounded-2 bg-[rgba(245,158,11,0.1)] px-3 py-2 text-xs leading-5 text-[#9a6700]">
          当前查看的是基础敏感词库。它会自动参与所有敏感词过滤节点的执行，不允许删除，只建议维护通用高风险词和不文明词。
        </div>
        <NDataTable flex-height class="flex-1 min-h-0" :columns="entryColumns" :data="filteredEntries" :loading="entryLoading" :pagination="{ pageSize: 10 }" size="small" />
      </NCard>
    </div>

    <NModal v-model:show="showDictModal" preset="card" :title="dictModalTitle" class="modal-card max-w-160">
      <NForm :model="dictForm" label-placement="top">
        <NFormItem label="词库名称">
          <NInput v-model:value="dictForm.name" placeholder="如：政务投诉场景" />
        </NFormItem>
        <NFormItem label="场景">
          <NInput v-model:value="dictForm.scene" placeholder="如：政务 / 医疗 / 金融" />
        </NFormItem>
        <NFormItem label="说明">
          <NInput v-model:value="dictForm.description" type="textarea" :autosize="{ minRows: 3, maxRows: 5 }" placeholder="描述该词库适用的业务场景或过滤目标" />
        </NFormItem>
        <NFormItem label="词库类型">
          <div class="flex items-center gap-3 rounded-2 bg-white/70 px-3 py-3">
            <NSwitch v-model:value="dictForm.isBase" />
            <span class="text-sm text-ink">设为基础敏感词库</span>
          </div>
          <div class="mt-2 text-xs leading-5 text-slate/70">
            基础敏感词库会默认叠加到所有敏感词过滤节点，系统只允许保留一个基础库，且基础库不允许删除。
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

    <NModal v-model:show="showEntryModal" preset="card" :title="entryModalTitle" class="modal-card max-w-120">
      <NForm :model="entryForm" label-placement="top">
        <NFormItem label="所属词库">
          <NInput :value="currentDict?.name || ''" disabled />
        </NFormItem>
        <NFormItem label="敏感词">
          <NInput v-model:value="entryForm.word" placeholder="输入需要过滤的词" />
        </NFormItem>
        <NFormItem label="启用状态">
          <div class="flex items-center gap-3 rounded-2 bg-white/70 px-3 py-3">
            <NSwitch v-model:value="entryForm.enabled" />
            <span class="text-sm text-ink">启用该敏感词</span>
          </div>
        </NFormItem>
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

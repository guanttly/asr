<script setup lang="ts">
import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, reactive, ref } from 'vue'

import {
  createTermDict,
  createTermEntry,
  deleteTermDict,
  deleteTermEntry,
  getTermDicts,
  getTermEntries,
  updateTermDict,
  updateTermEntry,
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
  pinyin: string
}

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
const dicts = ref<DictItem[]>([])
const entries = ref<EntryItem[]>([])
const dictForm = reactive({
  name: '',
  domain: '',
})
const entryForm = reactive({
  correctTerm: '',
  wrongVariantsText: '',
  pinyin: '',
})

const currentDict = computed(() => dicts.value.find(item => item.id === currentDictId.value) || null)
const dictModalTitle = computed(() => editingDictId.value ? '编辑术语词库' : '新建术语词库')
const entryModalTitle = computed(() => editingEntryId.value ? '编辑词条' : '新增词条')

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
  { title: '拼音', key: 'pinyin' },
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

function resetDictForm() {
  editingDictId.value = null
  dictForm.name = ''
  dictForm.domain = ''
}

function resetEntryForm() {
  editingEntryId.value = null
  entryForm.correctTerm = ''
  entryForm.wrongVariantsText = ''
  entryForm.pinyin = ''
}

async function loadDicts() {
  loading.value = true
  try {
    const result = await getTermDicts({ offset: 0, limit: 100 })
    dicts.value = result.data.items

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
    message.error('术语词典加载失败')
  }
  finally {
    loading.value = false
  }
}

async function selectDict(dictId: number) {
  currentDictId.value = dictId
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

function openEditEntryModal(row: EntryItem) {
  editingEntryId.value = row.id
  entryForm.correctTerm = row.correct_term
  entryForm.wrongVariantsText = row.wrong_variants.join('\n')
  entryForm.pinyin = row.pinyin || ''
  showEntryModal.value = true
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
      pinyin: entryForm.pinyin.trim(),
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
              <span class="text-sm font-600">{{ currentDict ? `${currentDict.name} 条目列表` : '词条列表' }}</span>
              <NTag v-if="currentDict" round type="info" size="small">
                {{ currentDict.domain }}
              </NTag>
            </div>
            <div class="flex items-center gap-2">
              <NButton :disabled="!currentDictId" quaternary size="small" @click="currentDictId && selectDict(currentDictId)">
                刷新
              </NButton>
              <NButton :disabled="!currentDictId" quaternary size="small" @click="openCreateEntryModal">
                新增词条
              </NButton>
            </div>
          </div>
        </template>

        <NDataTable flex-height class="flex-1 min-h-0" :columns="entryColumns" :data="entries" :loading="entryLoading" :pagination="{ pageSize: 8 }" size="small" />
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

    <NModal v-model:show="showEntryModal" preset="card" :title="entryModalTitle" class="modal-card max-w-160">
      <NForm :model="entryForm" label-placement="top">
        <NFormItem label="所属词库">
          <NInput :value="currentDict?.name || ''" disabled />
        </NFormItem>
        <NFormItem label="标准术语">
          <NInput v-model:value="entryForm.correctTerm" placeholder="如：冠状动脉" />
        </NFormItem>
        <NFormItem label="拼音">
          <NInput v-model:value="entryForm.pinyin" placeholder="如：guan zhuang dong mai" />
        </NFormItem>
        <NFormItem label="误写变体">
          <NInput
            v-model:value="entryForm.wrongVariantsText"
            type="textarea"
            :autosize="{ minRows: 3, maxRows: 5 }"
            placeholder="每行一个，或使用逗号分隔"
          />
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

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NButton, NTag, useMessage } from 'naive-ui'

import { createTermDict, createTermEntry, getTermDicts, getTermEntries } from '@/api/terminology'

type DictItem = {
  id: number
  name: string
  domain: string
}

type EntryItem = {
  id: number
  correct_term: string
  wrong_variants: string[]
  pinyin: string
}

const message = useMessage()
const loading = ref(false)
const entryLoading = ref(false)
const showCreateModal = ref(false)
const showEntryModal = ref(false)
const currentDictId = ref<number | null>(null)
const dicts = ref<DictItem[]>([])
const entries = ref<EntryItem[]>([])
const createForm = reactive({
  name: '',
  domain: '',
})
const entryForm = reactive({
  correctTerm: '',
  wrongVariantsText: '',
})

const dictColumns = [
  { title: '词库名称', key: 'name' },
  { title: '领域', key: 'domain' },
  {
    title: '操作',
    key: 'actions',
    width: 96,
    render: (row: DictItem) => row.id === currentDictId.value
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
]

const currentDict = computed(() => dicts.value.find(item => item.id === currentDictId.value) || null)

async function loadDicts() {
  loading.value = true
  try {
    const result = await getTermDicts({ offset: 0, limit: 100 })
    dicts.value = result.data.items
    if (!currentDictId.value && dicts.value.length > 0)
      await selectDict(dicts.value[0].id)
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

async function handleCreateDict() {
  if (!createForm.name || !createForm.domain) {
    message.warning('请填写词库名称和领域')
    return
  }

  try {
    await createTermDict(createForm)
    message.success('词库创建成功')
    showCreateModal.value = false
    createForm.name = ''
    createForm.domain = ''
    await loadDicts()
  }
  catch {
    message.error('词库创建失败')
  }
}

async function handleCreateEntry() {
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

  try {
    await createTermEntry(currentDictId.value, {
      correct_term: entryForm.correctTerm.trim(),
      wrong_variants: wrongVariants,
    })
    message.success('词条创建成功')
    showEntryModal.value = false
    entryForm.correctTerm = ''
    entryForm.wrongVariantsText = ''
    await selectDict(currentDictId.value)
  }
  catch {
    message.error('词条创建失败')
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
              <NButton quaternary size="small" @click="loadDicts">刷新</NButton>
              <NButton size="small" type="primary" color="#0f766e" @click="showCreateModal = true">新建词库</NButton>
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
              <NTag v-if="currentDict" round type="info" size="small">{{ currentDict.domain }}</NTag>
            </div>
            <NButton :disabled="!currentDictId" quaternary size="small" @click="showEntryModal = true">新增词条</NButton>
          </div>
        </template>

        <NDataTable flex-height class="flex-1 min-h-0" :columns="entryColumns" :data="entries" :loading="entryLoading" :pagination="{ pageSize: 8 }" size="small" />
      </NCard>
    </div>

    <NModal v-model:show="showCreateModal" preset="card" title="新建术语词库" class="modal-card max-w-140">
      <NForm :model="createForm" label-placement="top">
        <NFormItem label="词库名称">
          <NInput v-model:value="createForm.name" placeholder="如：医疗查房" />
        </NFormItem>
        <NFormItem label="领域">
          <NInput v-model:value="createForm.domain" placeholder="如：医疗" />
        </NFormItem>
        <div class="flex justify-end gap-3">
          <NButton @click="showCreateModal = false">取消</NButton>
          <NButton type="primary" color="#0f766e" @click="handleCreateDict">创建</NButton>
        </div>
      </NForm>
    </NModal>

    <NModal v-model:show="showEntryModal" preset="card" title="新增词条" class="modal-card max-w-160">
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
        <div class="flex justify-end gap-3">
          <NButton @click="showEntryModal = false">取消</NButton>
          <NButton type="primary" color="#0f766e" @click="handleCreateEntry">创建</NButton>
        </div>
      </NForm>
    </NModal>
  </div>
</template>
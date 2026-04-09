<script setup lang="ts">
import type { DataTableColumns } from 'naive-ui'

import { NButton, useMessage } from 'naive-ui'
import { computed, h, onMounted, reactive, ref } from 'vue'

import { createTermRule, deleteTermRule, getTermDicts, getTermRules, updateTermRule } from '@/api/terminology'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'

interface DictItem {
  id: number
  name: string
  domain: string
}

interface RuleItem {
  id: number
  layer: number
  pattern: string
  replacement: string
  enabled: boolean
}

const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()
const dicts = ref<DictItem[]>([])
const currentDictId = ref<number | null>(null)
const rules = ref<RuleItem[]>([])
const loading = ref(false)
const creating = ref(false)
const showCreateModal = ref(false)
const editingRuleId = ref<number | null>(null)
const deletingRuleId = ref<number | null>(null)
const form = reactive({
  layer: 1,
  pattern: '',
  replacement: '',
  enabled: true,
})

const dictOptions = computed(() => dicts.value.map(item => ({ label: `${item.name} · ${item.domain}`, value: item.id })))
const modalTitle = computed(() => editingRuleId.value ? '编辑纠错规则' : '新增纠错规则')

const columns: DataTableColumns<RuleItem> = [
  { title: '层级', key: 'layer', render: (row: RuleItem) => `第 ${row.layer} 层` },
  { title: '匹配模式', key: 'pattern' },
  { title: '替换为', key: 'replacement' },
  { title: '状态', key: 'enabled', render: (row: RuleItem) => row.enabled ? '启用' : '停用' },
  {
    title: '操作',
    key: 'actions',
    width: 160,
    render: (row: RuleItem) => h('div', { class: 'flex items-center gap-2' }, [
      h(NButton, {
        text: true,
        size: 'small',
        onClick: () => openEditModal(row),
      }, { default: () => '编辑' }),
      h(NButton, {
        text: true,
        type: 'error',
        size: 'small',
        loading: deletingRuleId.value === row.id,
        onClick: () => handleDeleteRule(row),
      }, { default: () => '删除' }),
    ]),
  },
]

function resetForm() {
  editingRuleId.value = null
  form.layer = 1
  form.pattern = ''
  form.replacement = ''
  form.enabled = true
}

function openCreateModal() {
  resetForm()
  showCreateModal.value = true
}

function openEditModal(row: RuleItem) {
  editingRuleId.value = row.id
  form.layer = row.layer
  form.pattern = row.pattern
  form.replacement = row.replacement
  form.enabled = row.enabled
  showCreateModal.value = true
}

async function loadDicts() {
  try {
    const result = await getTermDicts({ offset: 0, limit: 100 })
    dicts.value = result.data.items
    if (!currentDictId.value && dicts.value.length > 0) {
      currentDictId.value = dicts.value[0].id
      await loadRules()
    }
  }
  catch {
    message.error('词典列表加载失败')
  }
}

async function loadRules() {
  if (!currentDictId.value) {
    rules.value = []
    return
  }

  loading.value = true
  try {
    const result = await getTermRules(currentDictId.value)
    rules.value = result.data
  }
  catch {
    message.error('规则加载失败')
  }
  finally {
    loading.value = false
  }
}

async function handleCreateRule() {
  if (!currentDictId.value) {
    message.warning('请先选择词库')
    return
  }
  if (!form.pattern || !form.replacement) {
    message.warning('请填写匹配模式和替换内容')
    return
  }

  creating.value = true
  try {
    const payload = {
      layer: form.layer,
      pattern: form.pattern,
      replacement: form.replacement,
      enabled: form.enabled,
    }

    if (editingRuleId.value) {
      await updateTermRule(currentDictId.value, editingRuleId.value, payload)
      message.success('规则更新成功')
    }
    else {
      await createTermRule(currentDictId.value, payload)
      message.success('规则创建成功')
    }

    showCreateModal.value = false
    resetForm()
    await loadRules()
  }
  catch {
    message.error(editingRuleId.value ? '规则更新失败' : '规则创建失败')
  }
  finally {
    creating.value = false
  }
}

async function handleDeleteRule(row: RuleItem) {
  if (!currentDictId.value)
    return

  const confirmed = await confirmDelete({
    entityType: '纠错规则',
    entityName: row.pattern,
    description: '删除后，这条规则将不再参与术语纠错链路。',
  })
  if (!confirmed)
    return

  deletingRuleId.value = row.id
  try {
    await deleteTermRule(currentDictId.value, row.id)
    message.success('规则已删除')
    await loadRules()
  }
  catch {
    message.error('规则删除失败')
  }
  finally {
    deletingRuleId.value = null
  }
}

onMounted(loadDicts)
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 gap-5">
    <NCard class="card-main flex flex-col min-h-0 flex-1" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div class="flex items-center gap-3">
            <span class="text-sm font-600">规则列表</span>
            <NSelect v-model:value="currentDictId" :options="dictOptions" placeholder="请选择词库" size="small" class="w-full sm:!w-64" @update:value="loadRules" />
          </div>
          <div class="flex items-center gap-2">
            <NButton quaternary size="small" @click="loadRules">
              刷新
            </NButton>
            <NButton type="primary" size="small" color="#0f766e" @click="openCreateModal">
              新增规则
            </NButton>
          </div>
        </div>
      </template>

      <div class="mb-4 grid gap-3 md:grid-cols-3 shrink-0">
        <div class="subtle-panel">
          <div class="text-sm font-600 text-ink">
            第一层
          </div>
          <div class="mt-2 text-sm text-slate">
            精确词典替换，适合高频稳定错误映射。
          </div>
        </div>
        <div class="subtle-panel">
          <div class="text-sm font-600 text-ink">
            第二层
          </div>
          <div class="mt-2 text-sm text-slate">
            编辑距离匹配，适合轻微近形误识别。
          </div>
        </div>
        <div class="subtle-panel">
          <div class="text-sm font-600 text-ink">
            第三层
          </div>
          <div class="mt-2 text-sm text-slate">
            拼音音近纠错，适合口语化术语场景。
          </div>
        </div>
      </div>

      <NDataTable flex-height class="flex-1 min-h-0" :columns="columns" :data="rules" :loading="loading" :pagination="{ pageSize: 10 }" size="small" />
    </NCard>

    <NModal v-model:show="showCreateModal" preset="card" :title="modalTitle" class="modal-card max-w-140">
      <NForm :model="form" label-placement="top">
        <NFormItem label="规则层级">
          <NSelect
            v-model:value="form.layer"
            :options="[
              { label: '第一层：精确替换', value: 1 },
              { label: '第二层：编辑距离', value: 2 },
              { label: '第三层：拼音音近', value: 3 },
            ]"
          />
        </NFormItem>
        <NFormItem label="匹配模式">
          <NInput v-model:value="form.pattern" placeholder="如：舒张亚" />
        </NFormItem>
        <NFormItem label="替换内容">
          <NInput v-model:value="form.replacement" placeholder="如：舒张压" />
        </NFormItem>
        <NFormItem label="启用状态">
          <NSwitch v-model:value="form.enabled" />
        </NFormItem>

        <div class="flex justify-end gap-3">
          <NButton @click="showCreateModal = false">
            取消
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="creating" @click="handleCreateRule">
            {{ editingRuleId ? '保存' : '创建' }}
          </NButton>
        </div>
      </NForm>
    </NModal>
  </div>
</template>

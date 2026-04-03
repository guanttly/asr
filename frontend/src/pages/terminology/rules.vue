<script setup lang="ts">
import { computed, onMounted, reactive, ref } from 'vue'
import { useMessage } from 'naive-ui'

import { createTermRule, getTermDicts, getTermRules } from '@/api/terminology'

type DictItem = {
  id: number
  name: string
  domain: string
}

type RuleItem = {
  id: number
  layer: number
  pattern: string
  replacement: string
  enabled: boolean
}

const message = useMessage()
const dicts = ref<DictItem[]>([])
const currentDictId = ref<number | null>(null)
const rules = ref<RuleItem[]>([])
const loading = ref(false)
const creating = ref(false)
const showCreateModal = ref(false)
const form = reactive({
  layer: 1,
  pattern: '',
  replacement: '',
  enabled: true,
})

const dictOptions = computed(() => dicts.value.map(item => ({ label: `${item.name} · ${item.domain}`, value: item.id })))

const columns = [
  { title: '层级', key: 'layer', render: (row: RuleItem) => `第 ${row.layer} 层` },
  { title: '匹配模式', key: 'pattern' },
  { title: '替换为', key: 'replacement' },
  { title: '状态', key: 'enabled', render: (row: RuleItem) => row.enabled ? '启用' : '停用' },
]

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
    await createTermRule(currentDictId.value, form)
    message.success('规则创建成功')
    showCreateModal.value = false
    form.layer = 1
    form.pattern = ''
    form.replacement = ''
    form.enabled = true
    await loadRules()
  }
  catch {
    message.error('规则创建失败')
  }
  finally {
    creating.value = false
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
            <NButton quaternary size="small" @click="loadRules">刷新</NButton>
            <NButton type="primary" size="small" color="#0f766e" @click="showCreateModal = true">新增规则</NButton>
          </div>
        </div>
      </template>

      <div class="mb-4 grid gap-3 md:grid-cols-3 shrink-0">
      <div class="subtle-panel">
        <div class="text-sm font-600 text-ink">第一层</div>
        <div class="mt-2 text-sm text-slate">精确词典替换，适合高频稳定错误映射。</div>
      </div>
      <div class="subtle-panel">
        <div class="text-sm font-600 text-ink">第二层</div>
        <div class="mt-2 text-sm text-slate">编辑距离匹配，适合轻微近形误识别。</div>
      </div>
      <div class="subtle-panel">
        <div class="text-sm font-600 text-ink">第三层</div>
        <div class="mt-2 text-sm text-slate">拼音音近纠错，适合口语化术语场景。</div>
      </div>
      </div>

      <NDataTable flex-height class="flex-1 min-h-0" :columns="columns" :data="rules" :loading="loading" :pagination="{ pageSize: 10 }" size="small" />
    </NCard>

    <NModal v-model:show="showCreateModal" preset="card" title="新增纠错规则" class="modal-card max-w-140">
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
          <NButton @click="showCreateModal = false">取消</NButton>
          <NButton type="primary" color="#0f766e" :loading="creating" @click="handleCreateRule">创建</NButton>
        </div>
      </NForm>
    </NModal>
  </div>
</template>
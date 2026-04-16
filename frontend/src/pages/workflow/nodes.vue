<script setup lang="ts">
import type { WorkflowNodeTypeInfo } from '@/api/workflow'

import { useMessage } from 'naive-ui'
import { computed, onMounted, reactive, ref, watch } from 'vue'

import { getSensitiveDicts } from '@/api/sensitive'
import { getTermDicts } from '@/api/terminology'
import { getNodeTypes, testNodeStream, updateNodeDefault } from '@/api/workflow'
import NodeDetailPanel from '@/components/NodeDetailPanel.vue'
import TextDiffPreview from '@/components/TextDiffPreview.vue'
import { buildNodeConfigOverrides, fallbackNodeDefaultConfig, formatConfigText, normalizeNodeConfig } from '@/utils/workflowNodeConfig'

interface DictOption {
  label: string
  value: number
}

interface RegexRule {
  pattern: string
  replacement: string
  enabled: boolean
}

const message = useMessage()
const loading = ref(false)
const saving = ref(false)
const testing = ref(false)
const showRawConfig = ref(false)
const keyword = ref('')
const roleFilter = ref<'all' | 'source' | 'transform' | 'sink'>('all')
const selectedNodeType = ref('')
const nodeTypes = ref<WorkflowNodeTypeInfo[]>([])
const termDictOptions = ref<DictOption[]>([])
const sensitiveDictOptions = ref<DictOption[]>([])
const defaultDraft = reactive({
  configText: '{}',
})
const nodeTestInput = ref('')
const nodeTestOutput = ref('')
const nodeTestDetail = ref<Record<string, unknown> | string | null>(null)
const nodeTestAudioInputRef = ref<HTMLInputElement | null>(null)
const nodeTestAudioFile = ref<File | null>(null)
const audioFileAccept = 'audio/*,.wav,.mp3,.m4a,.aac,.flac,.ogg,.opus,.webm'

const filteredNodeTypes = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  return nodeTypes.value.filter((item) => {
    if (roleFilter.value !== 'all' && item.role !== roleFilter.value)
      return false
    if (!value)
      return true
    return item.label.toLowerCase().includes(value)
      || item.type.toLowerCase().includes(value)
      || (item.description || '').toLowerCase().includes(value)
  })
})

const selectedNode = computed(() => nodeTypes.value.find(item => item.type === selectedNodeType.value) || null)
const selectedConfig = computed<Record<string, any>>(() => {
  if (!selectedNode.value)
    return {}
  return normalizeNodeConfig(selectedNode.value.type, parseConfigSafe(defaultDraft.configText), fallbackNodeDefaultConfig(selectedNode.value.type))
})
const selectedRegexRules = computed<RegexRule[]>(() => Array.isArray(selectedConfig.value.rules) ? selectedConfig.value.rules as RegexRule[] : [])
const selectedNodeNeedsAudio = computed(() => ['batch_asr', 'realtime_asr', 'speaker_diarize'].includes(selectedNode.value?.type || ''))
const selectedNodePreviewMode = computed<'diff' | 'plain' | 'markdown'>(() => {
  if (selectedNode.value?.type === 'meeting_summary')
    return 'markdown'
  return selectedNodeNeedsAudio.value ? 'plain' : 'diff'
})
const nodeTestInputPreview = computed(() => selectedNodeNeedsAudio.value
  ? (nodeTestAudioFile.value ? `已上传音频：${nodeTestAudioFile.value.name}` : '尚未选择音频文件')
  : nodeTestInput.value)
const draftChanged = computed(() => {
  if (!selectedNode.value)
    return false
  return normalizeConfigText(defaultDraft.configText) !== normalizeConfigText(buildStoredOverrideText(selectedNode.value))
})
const roleFilterOptions = computed(() => [
  { label: '全部', value: 'all' },
  { label: '源', value: 'source' },
  { label: '处理', value: 'transform' },
  { label: '输出', value: 'sink' },
])

function buildMeetingSummaryChunkPreview(detail: unknown) {
  if (!detail || typeof detail !== 'object')
    return ''
  const chunkOutputs = Array.isArray((detail as Record<string, unknown>).chunk_outputs)
    ? (detail as Record<string, unknown>).chunk_outputs as Array<Record<string, unknown>>
    : []
  if (!chunkOutputs.length)
    return ''
  return chunkOutputs
    .map((chunk, index) => {
      const title = typeof chunk.title === 'string' && chunk.title.trim() ? chunk.title.trim() : `片段 ${index + 1}`
      const output = typeof chunk.output === 'string' ? chunk.output.trim() : ''
      if (!output)
        return ''
      return `## ${title}\n\n${output}`
    })
    .filter(Boolean)
    .join('\n\n')
}

function nodeTypeAlias(type: string) {
  const aliases: Record<string, string> = {
    legacy_text: '文本输入节点',
    batch_asr: '批量转写节点',
    realtime_asr: '实时转写节点',
    term_correction: '术语纠正节点',
    filler_filter: '语气词过滤节点',
    sensitive_filter: '敏感词过滤节点',
    llm_correction: '大模型纠错节点',
    speaker_diarize: '说话人分离节点',
    custom_regex: '正则处理节点',
    meeting_summary: '会议纪要节点',
  }
  return aliases[type] || '系统节点'
}

function parseConfig(text: string) {
  const value = text.trim()
  if (!value)
    return {}
  return JSON.parse(value) as Record<string, unknown>
}

function parseConfigSafe(text: string) {
  try {
    return parseConfig(text)
  }
  catch {
    return {}
  }
}

function normalizeConfigText(text: string) {
  return JSON.stringify(parseConfigSafe(text), null, 2)
}

function buildStoredOverrideText(node: WorkflowNodeTypeInfo) {
  const config = buildNodeConfigOverrides(node.type, (node.default_config || {}) as Record<string, unknown>, fallbackNodeDefaultConfig(node.type))
  return formatConfigText(config)
}

function resetDraft() {
  if (!selectedNode.value) {
    defaultDraft.configText = '{}'
    return
  }
  defaultDraft.configText = buildStoredOverrideText(selectedNode.value)
}

function replaceSelectedConfig(nextConfig: Record<string, unknown>) {
  if (!selectedNode.value)
    return
  defaultDraft.configText = formatConfigText(buildNodeConfigOverrides(selectedNode.value.type, nextConfig, fallbackNodeDefaultConfig(selectedNode.value.type)))
}

function updateSelectedConfig(patch: Record<string, unknown>) {
  replaceSelectedConfig({
    ...selectedConfig.value,
    ...patch,
  })
}

function listToText(value: unknown) {
  if (!Array.isArray(value))
    return ''
  return value.map(item => String(item).trim()).filter(Boolean).join('\n')
}

function textToList(value: string) {
  return value
    .split(/[\n,，]/)
    .map(item => item.trim())
    .filter(Boolean)
}

function updateRegexRule(index: number, patch: Partial<RegexRule>) {
  const rules = [...(selectedConfig.value.rules || [])]
  if (!rules[index])
    return
  rules[index] = { ...rules[index], ...patch }
  updateSelectedConfig({ rules })
}

function addRegexRule() {
  const rules = [...(selectedConfig.value.rules || [])]
  rules.push({ pattern: '', replacement: '', enabled: true })
  updateSelectedConfig({ rules })
}

function removeRegexRule(index: number) {
  const rules = [...(selectedConfig.value.rules || [])]
  rules.splice(index, 1)
  updateSelectedConfig({ rules: rules.length > 0 ? rules : [{ pattern: '', replacement: '', enabled: true }] })
}

function roleLabel(role?: string) {
  const map: Record<string, string> = {
    source: '源节点',
    transform: '处理节点',
    sink: '输出节点',
  }
  return map[role || ''] || '节点'
}

function formatFileSize(size?: number) {
  if (!size)
    return '0 B'
  if (size < 1024)
    return `${size} B`
  if (size < 1024 * 1024)
    return `${(size / 1024).toFixed(1)} KB`
  return `${(size / 1024 / 1024).toFixed(1)} MB`
}

function openNodeTestAudioPicker() {
  nodeTestAudioInputRef.value?.click()
}

function clearNodeTestAudioFile() {
  nodeTestAudioFile.value = null
}

function handleNodeTestAudioSelected(event: Event) {
  const input = event.target as HTMLInputElement
  nodeTestAudioFile.value = input.files?.[0] || null
  input.value = ''
}

async function loadTermDictOptions() {
  try {
    const result = await getTermDicts({ offset: 0, limit: 100 })
    termDictOptions.value = (result.data.items || []).map((item: { id: number, name: string, domain: string }) => ({
      label: `${item.name} / ${item.domain}`,
      value: item.id,
    }))
  }
  catch {
    message.warning('术语词库加载失败，术语纠正节点仍可用 JSON 编辑')
  }
}

async function loadSensitiveDictOptions() {
  try {
    const result = await getSensitiveDicts({ offset: 0, limit: 100 })
    sensitiveDictOptions.value = (result.data.items || [])
      .filter((item: { id: number, name: string, scene: string, is_base: boolean }) => !item.is_base)
      .map((item: { id: number, name: string, scene: string }) => ({
        label: `${item.name} / ${item.scene}`,
        value: item.id,
      }))
  }
  catch {
    message.warning('敏感词库加载失败，敏感词过滤节点仍可用 JSON 编辑')
  }
}

async function loadNodeTypeList() {
  loading.value = true
  try {
    const result = await getNodeTypes()
    nodeTypes.value = result.data || []
    if (!selectedNodeType.value && nodeTypes.value.length > 0)
      selectedNodeType.value = nodeTypes.value[0].type
    else if (selectedNodeType.value && !nodeTypes.value.some(item => item.type === selectedNodeType.value))
      selectedNodeType.value = nodeTypes.value[0]?.type || ''
    resetDraft()
  }
  catch {
    message.error('节点类型加载失败')
  }
  finally {
    loading.value = false
  }
}

async function handleSaveDefaults() {
  if (!selectedNode.value) {
    message.warning('请先选择一个节点类型')
    return
  }

  let rawConfig: Record<string, unknown>
  let config: Record<string, unknown>
  try {
    rawConfig = parseConfig(defaultDraft.configText)
    const effectiveConfig = normalizeNodeConfig(selectedNode.value.type, rawConfig, fallbackNodeDefaultConfig(selectedNode.value.type))
    config = buildNodeConfigOverrides(selectedNode.value.type, effectiveConfig, fallbackNodeDefaultConfig(selectedNode.value.type))
  }
  catch (error) {
    message.error(`默认配置 JSON 不合法：${error instanceof Error ? error.message : '未知错误'}`)
    return
  }

  saving.value = true
  try {
    const result = await updateNodeDefault(selectedNode.value.type, { config })
    const index = nodeTypes.value.findIndex(item => item.type === selectedNode.value?.type)
    if (index >= 0)
      nodeTypes.value[index] = result.data
    resetDraft()
    message.success('节点默认配置已保存')
  }
  catch {
    message.error('节点默认配置保存失败')
  }
  finally {
    saving.value = false
  }
}

async function handleTestNode() {
  if (!selectedNode.value) {
    message.warning('请先选择一个节点类型')
    return
  }
  if (selectedNodeNeedsAudio.value && !nodeTestAudioFile.value) {
    message.warning('请先上传音频文件')
    return
  }

  testing.value = true
  try {
    nodeTestOutput.value = ''
    nodeTestDetail.value = { status: 'starting', message: '节点测试已开始' }
    const rawConfig = parseConfig(defaultDraft.configText)
    const effectiveConfig = normalizeNodeConfig(selectedNode.value.type, rawConfig, fallbackNodeDefaultConfig(selectedNode.value.type))
    const payload = selectedNodeNeedsAudio.value
      ? (() => {
          const formData = new FormData()
          formData.append('node_type', selectedNode.value!.type)
          formData.append('config', JSON.stringify(effectiveConfig))
          formData.append('file', nodeTestAudioFile.value!)
          return formData
        })()
      : {
          node_type: selectedNode.value.type,
          config: effectiveConfig,
          input_text: nodeTestInput.value,
        }
    let finished = false
    await testNodeStream(payload, {
      onEvent(event) {
        if (event.type === 'status') {
          const nextDetail = event.detail ?? {
            status: 'streaming',
            message: event.message || '节点执行中',
          }
          nodeTestDetail.value = nextDetail
          if (selectedNode.value?.type === 'meeting_summary') {
            const preview = buildMeetingSummaryChunkPreview(nextDetail)
            if (preview)
              nodeTestOutput.value = preview
          }
          return
        }
        if (event.type === 'delta') {
          nodeTestOutput.value = event.output_text || `${nodeTestOutput.value}${event.delta || ''}`
          nodeTestDetail.value = typeof nodeTestDetail.value === 'object' && nodeTestDetail.value
            ? {
                ...nodeTestDetail.value,
                status: 'streaming',
                message: 'LLM 正在生成输出',
              }
            : {
                status: 'streaming',
                message: 'LLM 正在生成输出',
              }
          return
        }
        finished = true
        nodeTestOutput.value = event.output_text || ''
        nodeTestDetail.value = event.detail ?? event.error ?? null
      },
    })
    if (!finished)
      nodeTestDetail.value = null
  }
  catch (error) {
    nodeTestOutput.value = ''
    nodeTestDetail.value = error instanceof Error ? error.message : '节点测试失败'
    message.error('节点测试失败')
  }
  finally {
    testing.value = false
  }
}

watch(selectedNodeType, () => {
  resetDraft()
  nodeTestOutput.value = ''
  nodeTestDetail.value = null
  nodeTestInput.value = ''
  nodeTestAudioFile.value = null
})

onMounted(async () => {
  await Promise.all([loadNodeTypeList(), loadTermDictOptions(), loadSensitiveDictOptions()])
})
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-col gap-5 overflow-x-hidden overflow-y-auto pr-1">
    <NCard class="card-main shrink-0">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <div>
            <div class="text-sm font-700 text-ink">
              节点管理
            </div>
            <div class="mt-1 text-xs text-slate/70">
              统一维护节点类型默认配置。工作流里的单个节点只需要填局部差异，例如只覆写提示词，LLM endpoint 会自动继承这里的默认值。
            </div>
          </div>
          <div class="flex flex-wrap items-center gap-2">
            <NInput v-model:value="keyword" clearable size="small" placeholder="搜索节点类型 / 描述" class="w-full sm:!w-64" />
            <NButton quaternary size="small" @click="loadNodeTypeList">
              刷新
            </NButton>
          </div>
        </div>
      </template>

      <div class="grid gap-3 md:grid-cols-4">
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            节点类型
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ nodeTypes.length }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            源节点
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ nodeTypes.filter(item => item.role === 'source').length }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            处理节点
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ nodeTypes.filter(item => item.role === 'transform').length }}
          </div>
        </div>
        <div class="subtle-panel m-0">
          <div class="text-xs text-slate/70">
            当前筛选
          </div>
          <div class="mt-1 text-lg font-700 text-ink">
            {{ filteredNodeTypes.length }}
          </div>
        </div>
      </div>
    </NCard>

    <div class="grid items-stretch gap-5 xl:grid-cols-[minmax(280px,320px)_minmax(0,1fr)]">
      <NCard class="card-main" content-style="padding:0 0 16px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3 px-1">
            <div>
              <div class="text-sm font-600 text-ink">
                节点类型目录
              </div>
              <div class="mt-1 text-xs text-slate/70">
                选择一个节点类型进入默认配置与单节点调试。
              </div>
            </div>
            <div class="role-segmented" role="tablist" aria-label="节点角色筛选">
              <button
                v-for="option in roleFilterOptions"
                :key="option.value"
                type="button"
                class="role-segmented__button"
                :class="{ 'is-active': roleFilter === option.value }"
                :aria-selected="roleFilter === option.value"
                @click="roleFilter = option.value as typeof roleFilter"
              >
                {{ option.label }}
              </button>
            </div>
          </div>
        </template>

        <NSpin :show="loading" class="node-list-spin px-4">
          <div class="node-list-scroll grid gap-2 pb-1">
            <button
              v-for="item in filteredNodeTypes"
              :key="item.type"
              type="button"
              class="node-type-card"
              :class="{ 'is-active': selectedNodeType === item.type }"
              @click="selectedNodeType = item.type"
            >
              <div class="flex items-center justify-between gap-2">
                <div class="text-left">
                  <div class="text-sm font-700 text-ink">
                    {{ item.label }}
                  </div>
                  <div class="mt-1 text-[11px] text-slate/70">
                    {{ nodeTypeAlias(item.type) }}
                  </div>
                </div>
                <NTag size="small" round :bordered="false" :type="item.role === 'source' ? 'info' : 'default'">
                  {{ roleLabel(item.role) }}
                </NTag>
              </div>
              <div class="mt-2 text-left text-xs leading-5 text-slate/75">
                {{ item.description || '暂无说明。' }}
              </div>
            </button>
          </div>
        </NSpin>
      </NCard>

      <NSpin :show="loading" class="node-detail-spin self-stretch">
        <div v-if="selectedNode" class="grid gap-5 pr-1">
          <NCard class="card-main shrink-0">
            <template #header>
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <div class="flex items-center gap-2 text-sm font-700 text-ink">
                    {{ selectedNode.label }}
                    <NTag size="small" round :bordered="false" :type="selectedNode.role === 'source' ? 'info' : 'default'">
                      {{ roleLabel(selectedNode.role) }}
                    </NTag>
                    <NTag v-if="draftChanged" size="small" round :bordered="false" type="warning">
                      未保存
                    </NTag>
                  </div>
                  <div class="mt-1 text-xs text-slate/70">
                    {{ selectedNode.description || '当前节点暂无描述。' }}
                  </div>
                </div>
                <div class="flex items-center gap-2">
                  <NButton size="small" :disabled="!draftChanged" @click="resetDraft">
                    撤销修改
                  </NButton>
                  <NButton text size="small" @click="showRawConfig = !showRawConfig">
                    {{ showRawConfig ? '隐藏 JSON' : '高级 JSON' }}
                  </NButton>
                  <NButton size="small" type="primary" color="#0f766e" :loading="saving" @click="handleSaveDefaults">
                    保存默认配置
                  </NButton>
                </div>
              </div>
            </template>

            <div class="grid gap-3 rounded-2.5 bg-[#fbfdff] p-4">
              <div class="text-sm font-600 text-ink">
                默认配置
              </div>
              <div class="text-xs leading-6 text-slate/75">
                这里维护节点类型级的基线配置。工作流节点若未显式填写某项，会自动继承这里的默认值。
              </div>

              <template v-if="selectedNode.type === 'term_correction'">
                <div class="grid gap-2">
                  <div class="text-xs text-slate/70">
                    默认术语词库
                  </div>
                  <NSelect
                    :value="selectedConfig.dict_id || null"
                    clearable
                    :options="termDictOptions"
                    placeholder="选择默认词库"
                    @update:value="updateSelectedConfig({ dict_id: $event || 0 })"
                  />
                </div>
              </template>

              <template v-else-if="selectedNode.type === 'filler_filter'">
                <div class="grid gap-3">
                  <div>
                    <div class="text-xs text-slate/70">
                      默认过滤词
                    </div>
                    <NInput
                      :value="listToText(selectedConfig.filter_words)"
                      type="textarea"
                      :autosize="{ minRows: 4, maxRows: 8 }"
                      placeholder="每行一个，或用逗号分隔"
                      @update:value="updateSelectedConfig({ filter_words: textToList($event) })"
                    />
                  </div>
                  <div>
                    <div class="text-xs text-slate/70">
                      自定义补充词
                    </div>
                    <NInput
                      :value="listToText(selectedConfig.custom_words)"
                      type="textarea"
                      :autosize="{ minRows: 3, maxRows: 6 }"
                      placeholder="补充业务口语词"
                      @update:value="updateSelectedConfig({ custom_words: textToList($event) })"
                    />
                  </div>
                </div>
              </template>

              <template v-else-if="selectedNode.type === 'sensitive_filter'">
                <div class="grid gap-3">
                  <div>
                    <div class="text-xs text-slate/70">
                      场景敏感词库
                    </div>
                    <NSelect
                      :value="selectedConfig.dict_id || null"
                      clearable
                      :options="sensitiveDictOptions"
                      placeholder="不选则仅使用基础敏感词库"
                      @update:value="updateSelectedConfig({ dict_id: $event || 0 })"
                    />
                    <div class="mt-2 text-[11px] leading-5 text-slate/65">
                      基础敏感词库会自动参与过滤，这里只选择额外叠加的场景词库。
                    </div>
                  </div>
                  <div>
                    <div class="text-xs text-slate/70">
                      自定义补充词
                    </div>
                    <NInput
                      :value="listToText(selectedConfig.custom_words)"
                      type="textarea"
                      :autosize="{ minRows: 3, maxRows: 6 }"
                      placeholder="每行一个，补充当前节点专用的敏感词"
                      @update:value="updateSelectedConfig({ custom_words: textToList($event) })"
                    />
                  </div>
                  <div>
                    <div class="text-xs text-slate/70">
                      替换文本
                    </div>
                    <NInput
                      :value="selectedConfig.replacement"
                      placeholder="例如：[已过滤]"
                      @update:value="updateSelectedConfig({ replacement: $event || '[已过滤]' })"
                    />
                  </div>
                </div>
              </template>

              <template v-else-if="selectedNode.type === 'llm_correction'">
                <div class="grid gap-3">
                  <div class="grid gap-3 lg:grid-cols-2">
                    <NInput :value="selectedConfig.endpoint" placeholder="默认 LLM Endpoint" @update:value="updateSelectedConfig({ endpoint: $event })" />
                    <NInput :value="selectedConfig.model" placeholder="默认模型名" @update:value="updateSelectedConfig({ model: $event })" />
                  </div>
                  <div class="text-xs leading-6 text-slate/75">
                    例如：http://192.168.200.182:9888、http://192.168.200.182:9888/v1，或完整的 OpenAI 兼容 /chat/completions 地址。
                  </div>
                  <NInput :value="selectedConfig.api_key" type="password" show-password-on="click" placeholder="默认 API Key，可留空" @update:value="updateSelectedConfig({ api_key: $event })" />
                  <div class="grid gap-3 lg:grid-cols-3">
                    <NInputNumber :value="selectedConfig.temperature" :min="0" :max="2" :step="0.1" @update:value="updateSelectedConfig({ temperature: $event ?? 0.3 })" />
                    <NInputNumber :value="selectedConfig.max_tokens" :min="1" :step="256" @update:value="updateSelectedConfig({ max_tokens: $event ?? 4096 })" />
                    <div class="flex items-center gap-2 rounded-2 bg-white px-3 py-2.5">
                      <span class="text-xs text-slate">允许 Markdown 输出</span>
                      <NSwitch :value="selectedConfig.allow_markdown" @update:value="updateSelectedConfig({ allow_markdown: $event })" />
                    </div>
                  </div>
                  <NInput
                    :value="selectedConfig.prompt_template"
                    type="textarea"
                    :autosize="{ minRows: 6, maxRows: 12 }"
                    placeholder="默认 Prompt 模板，使用 {{TEXT}} 作为原文占位符"
                    @update:value="updateSelectedConfig({ prompt_template: $event })"
                  />
                </div>
              </template>

              <template v-else-if="selectedNode.type === 'speaker_diarize'">
                <div class="grid gap-3">
                  <NInput :value="selectedConfig.service_url" placeholder="默认说话人分离服务 URL" @update:value="updateSelectedConfig({ service_url: $event })" />
                  <div class="flex flex-wrap gap-6 rounded-2 bg-white/70 px-3 py-3">
                    <label class="flex items-center gap-2 text-sm text-ink">
                      <NSwitch :value="selectedConfig.enable_voiceprint_match" @update:value="updateSelectedConfig({ enable_voiceprint_match: $event })" />
                      <span>启用声纹匹配</span>
                    </label>
                    <label class="flex items-center gap-2 text-sm text-ink">
                      <NSwitch :value="selectedConfig.fail_on_error" @update:value="updateSelectedConfig({ fail_on_error: $event })" />
                      <span>失败时中断工作流</span>
                    </label>
                  </div>
                </div>
              </template>

              <template v-else-if="selectedNode.type === 'meeting_summary'">
                <div class="grid gap-3">
                  <div class="grid gap-3 lg:grid-cols-2">
                    <NInput :value="selectedConfig.endpoint" placeholder="默认摘要 LLM Endpoint" @update:value="updateSelectedConfig({ endpoint: $event })" />
                    <NInput :value="selectedConfig.model" placeholder="默认摘要模型" @update:value="updateSelectedConfig({ model: $event })" />
                  </div>
                  <NInput :value="selectedConfig.api_key" type="password" show-password-on="click" placeholder="默认 API Key，可留空" @update:value="updateSelectedConfig({ api_key: $event })" />
                  <div class="grid gap-3 lg:grid-cols-2">
                    <NInputNumber :value="selectedConfig.max_tokens" :min="1" :step="1024" @update:value="updateSelectedConfig({ max_tokens: $event ?? 65536 })" />
                    <NSelect
                      :value="selectedConfig.output_format || 'markdown'"
                      :options="[
                        { label: 'Markdown', value: 'markdown' },
                        { label: 'Plain Text', value: 'text' },
                      ]"
                      @update:value="updateSelectedConfig({ output_format: $event || 'markdown' })"
                    />
                  </div>
                  <NInput
                    :value="selectedConfig.prompt_template"
                    type="textarea"
                    :autosize="{ minRows: 6, maxRows: 12 }"
                    placeholder="默认会议纪要 Prompt 模板"
                    @update:value="updateSelectedConfig({ prompt_template: $event })"
                  />
                </div>
              </template>

              <template v-else-if="selectedNode.type === 'custom_regex'">
                <div class="grid gap-3">
                  <div v-for="(rule, index) in selectedRegexRules" :key="index" class="rounded-2 bg-white p-3">
                    <div class="flex items-center justify-between gap-2">
                      <div class="text-sm font-600 text-ink">
                        默认规则 {{ index + 1 }}
                      </div>
                      <div class="flex items-center gap-2">
                        <span class="text-xs text-slate">启用</span>
                        <NSwitch :value="rule.enabled" @update:value="updateRegexRule(index, { enabled: $event })" />
                        <NButton text size="small" type="error" @click="removeRegexRule(index)">
                          删除
                        </NButton>
                      </div>
                    </div>
                    <div class="mt-3 grid gap-3 lg:grid-cols-2">
                      <NInput :value="rule.pattern" placeholder="正则表达式" @update:value="updateRegexRule(index, { pattern: $event })" />
                      <NInput :value="rule.replacement" placeholder="替换文本" @update:value="updateRegexRule(index, { replacement: $event })" />
                    </div>
                  </div>
                  <div>
                    <NButton quaternary size="small" @click="addRegexRule">
                      新增规则
                    </NButton>
                  </div>
                </div>
              </template>

              <template v-else>
                <div class="rounded-2 bg-white/70 px-3 py-3 text-xs leading-6 text-slate/75">
                  该节点当前没有专门的结构化默认配置项，可直接使用下方 JSON 编辑，或仅在此页面做单节点测试。
                </div>
              </template>
            </div>

            <NInput
              v-if="showRawConfig"
              v-model:value="defaultDraft.configText"
              type="textarea"
              :autosize="{ minRows: 10, maxRows: 20 }"
              placeholder="使用 JSON 编辑节点默认配置"
              class="mt-3"
            />
          </NCard>

          <NCard class="card-main" content-style="display:flex;flex-direction:column;">
            <template #header>
              <span class="text-sm font-600">单节点调试</span>
            </template>
            <div class="grid gap-3">
              <div class="text-xs leading-6 text-slate/75">
                当前测试会直接使用右侧面板里的默认配置草稿，你可以先调通再决定是否保存。
              </div>
              <template v-if="selectedNodeNeedsAudio">
                <div class="rounded-2 bg-white p-3">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <div class="text-sm font-600 text-ink">
                        测试音频
                      </div>
                      <div class="text-xs text-slate/60">
                        {{ nodeTestAudioFile ? `${nodeTestAudioFile.name} · ${formatFileSize(nodeTestAudioFile.size)}` : '支持 WAV、MP3、M4A、AAC、FLAC、OGG、OPUS、WEBM。' }}
                      </div>
                    </div>
                    <div class="flex items-center gap-2">
                      <NButton size="small" @click="openNodeTestAudioPicker">
                        {{ nodeTestAudioFile ? '重新选择' : '上传音频' }}
                      </NButton>
                      <NButton v-if="nodeTestAudioFile" size="small" quaternary @click="clearNodeTestAudioFile">
                        清空
                      </NButton>
                    </div>
                  </div>
                </div>
              </template>
              <NInput v-else v-model:value="nodeTestInput" type="textarea" :autosize="{ minRows: 5, maxRows: 10 }" placeholder="输入测试文本，验证当前节点默认配置下的输出。" />
              <div class="flex justify-end">
                <NButton size="small" type="primary" color="#0f766e" :loading="testing" @click="handleTestNode">
                  测试当前节点
                </NButton>
              </div>
              <div class="grid gap-3 lg:grid-cols-2">
                <TextDiffPreview :mode="selectedNodePreviewMode" :before-text="nodeTestInputPreview" :after-text="nodeTestOutput" :before-label="selectedNodeNeedsAudio ? '上传音频' : '测试输入'" after-label="节点输出" />
                <div class="rounded-2 bg-white p-3">
                  <div class="text-xs text-slate/70">
                    详细信息
                  </div>
                  <div class="mt-2">
                    <NodeDetailPanel :detail="nodeTestDetail" empty-label="节点执行细节会显示在这里。" />
                  </div>
                </div>
              </div>
            </div>
          </NCard>
        </div>
      </NSpin>
    </div>

    <input ref="nodeTestAudioInputRef" type="file" :accept="audioFileAccept" class="hidden" @change="handleNodeTestAudioSelected">
  </div>
</template>

<style scoped>
.role-segmented {
  display: flex;
  align-items: center;
  gap: 4px;
  border-radius: 999px;
  background: rgba(240, 244, 248, 0.92);
  padding: 4px;
}

.role-segmented__button {
  border: 0;
  border-radius: 999px;
  background: transparent;
  color: #5d6b7c;
  padding: 7px 14px;
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  cursor: pointer;
  transition: background-color 0.18s ease, color 0.18s ease, box-shadow 0.18s ease;
}

.role-segmented__button:hover {
  color: #0f766e;
}

.role-segmented__button.is-active {
  background: #0f766e;
  color: #fff;
  box-shadow: 0 8px 18px rgba(15, 118, 110, 0.22);
}

.node-list-spin :deep(.n-spin-content) {
  min-height: 0;
}

.node-detail-spin :deep(.n-spin-content) {
  min-height: 0;
}

.node-type-card {
  width: 100%;
  border: 1px solid rgba(148, 163, 184, 0.18);
  border-radius: 18px;
  background: rgba(251, 253, 255, 0.96);
  padding: 14px;
  transition: border-color 0.18s ease, background-color 0.18s ease, box-shadow 0.18s ease;
}

.node-type-card:hover {
  border-color: rgba(15, 118, 110, 0.28);
  box-shadow: 0 12px 30px rgba(15, 23, 42, 0.06);
}

.node-type-card.is-active {
  border-color: rgba(15, 118, 110, 0.48);
  background: rgba(240, 253, 250, 0.92);
  box-shadow: 0 14px 30px rgba(15, 118, 110, 0.08);
}
</style>

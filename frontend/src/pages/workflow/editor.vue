<script setup lang="ts">
import type { ComponentPublicInstance } from 'vue'

import { useMessage } from 'naive-ui'
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref, watch } from 'vue'
import { useRoute, useRouter } from 'vue-router'

import { getFillerDicts } from '@/api/filler'
import { getSensitiveDicts } from '@/api/sensitive'
import { getTermDicts } from '@/api/terminology'
import { getVoiceCommandDicts } from '@/api/voiceCommands'
import { createWorkflow, deleteWorkflow, executeWorkflow, getNodeTypes, getWorkflow, getWorkflows, testNodeStream, updateWorkflow, updateWorkflowNodes } from '@/api/workflow'
import iconArrowRight from '@/assets/icons/icon-arrow-right.svg?raw'
import iconChevronDown from '@/assets/icons/icon-chevron-down.svg?raw'
import iconChevronUp from '@/assets/icons/icon-chevron-up.svg?raw'
import iconCircleDot from '@/assets/icons/icon-circle-dot.svg?raw'
import iconMinus from '@/assets/icons/icon-minus.svg?raw'
import iconPencil from '@/assets/icons/icon-pencil.svg?raw'
import iconPlus from '@/assets/icons/icon-plus.svg?raw'
import iconSort from '@/assets/icons/icon-sort.svg?raw'
import iconX from '@/assets/icons/icon-x.svg?raw'
import NodeDetailPanel from '@/components/NodeDetailPanel.vue'
import TextDiffPreview from '@/components/TextDiffPreview.vue'
import { useConfirmActionDialog } from '@/composables/useConfirmActionDialog'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'
import { buildNodeConfigOverrides, formatConfigText, getNodeDefaultConfig, normalizeNodeConfig } from '@/utils/workflowNodeConfig'
import { getWorkflowTemplateMeta } from '@/utils/workflowTemplateMeta'

interface NodeTypeOption {
  type: string
  label: string
  role?: 'source' | 'transform' | 'sink'
  description?: string
  default_config?: Record<string, unknown>
}

interface DictOption {
  label: string
  value: number
}

interface TemplateOption {
  label: string
  value: number
  description?: string
  workflow_type?: 'legacy' | 'batch_transcription' | 'realtime_transcription' | 'meeting' | 'voice_control'
  source_kind?: 'legacy_text' | 'batch_asr' | 'realtime_asr' | 'voice_wake'
  target_kind?: 'transcript' | 'meeting_summary' | 'voice_command'
  is_legacy?: boolean
}

interface RegexRule {
  pattern: string
  replacement: string
  enabled: boolean
}

interface WorkflowExecutionNodeResult {
  id?: number
  node_type: string
  label: string
  position: number
  input_text?: string
  output_text?: string
  status: string
  detail?: Record<string, unknown> | string | null
  duration_ms?: number
}

interface WorkflowExecutionResult {
  id?: number
  status: string
  final_text?: string
  error_message?: string
  node_results?: WorkflowExecutionNodeResult[]
}

interface EditableNode {
  id?: number
  node_type: string
  enabled: boolean
  position: number
  configText: string
  is_fixed?: boolean
}

interface EditableNodeDraft {
  enabled: boolean
  configText: string
}

const route = useRoute()
const router = useRouter()
const message = useMessage()
const confirmAction = useConfirmActionDialog()
const confirmDelete = useDeleteConfirmDialog()
const workflowId = computed(() => Number(route.params.id))
const loading = ref(false)
const saving = ref(false)
const savingCurrentNode = ref(false)
const savingAsNew = ref(false)
const deletingWorkflow = ref(false)
const testingNode = ref(false)
const executing = ref(false)
const nodeType = ref<string | null>(null)
const selectedIndex = ref(0)
const showRawConfig = ref(false)
const importingTemplate = ref(false)
const showSaveAsDialog = ref(false)
const templateLoading = ref(false)
const templatePreviewLoading = ref(false)
const showAllPreviewNodes = ref(false)
const showChangedPreviewOnly = ref(false)
const highlightedNodeIndex = ref<number | null>(null)
const nodeTypes = ref<NodeTypeOption[]>([])
const termDictOptions = ref<DictOption[]>([])
const fillerDictOptions = ref<DictOption[]>([])
const sensitiveDictOptions = ref<DictOption[]>([])
const voiceCommandDictOptions = ref<DictOption[]>([])
const templateOptions = ref<TemplateOption[]>([])
const selectedTemplateId = ref<number | null>(null)
const templatePreviewNodes = ref<EditableNode[]>([])
const templatePreviewName = ref('')
const nodes = ref<EditableNode[]>([])
const nodeRowRefs = ref<Array<HTMLElement | null>>([])
const nodeDraft = reactive<EditableNodeDraft>({
  enabled: true,
  configText: '{}',
})
const sourceWorkflowName = ref('')
const nodeTestInput = ref('')
const nodeTestOutput = ref('')
const nodeTestDetail = ref<Record<string, unknown> | string | null>(null)
const nodeTestAudioInputRef = ref<HTMLInputElement | null>(null)
const nodeTestAudioFile = ref<File | null>(null)
const executeInput = ref('')
const executeOutput = ref('')
const executeAudioInputRef = ref<HTMLInputElement | null>(null)
const executeAudioFile = ref<File | null>(null)
const executeResult = ref<WorkflowExecutionResult | null>(null)
const workflow = reactive({
  name: '',
  description: '',
  workflow_type: 'legacy' as 'legacy' | 'batch_transcription' | 'realtime_transcription' | 'meeting' | 'voice_control',
  source_kind: 'legacy_text' as 'legacy_text' | 'batch_asr' | 'realtime_asr' | 'voice_wake',
  target_kind: 'transcript' as 'transcript' | 'meeting_summary' | 'voice_command',
  is_legacy: true,
  validation_message: '',
  is_published: false,
  owner_type: '',
  source_id: null as number | null,
})
const saveAsForm = reactive({
  name: '',
  description: '',
})

const selectedTemplateDescription = computed(() => templateOptions.value.find(item => item.value === selectedTemplateId.value)?.description || '')
const selectedTemplateMeta = computed(() => {
  const option = templateOptions.value.find(item => item.value === selectedTemplateId.value)
  return getWorkflowTemplateMeta(option?.label, option?.description)
})
const currentPreviewEntries = computed(() => nodes.value.map((item, index) => ({ item, index })))
const templatePreviewEntries = computed(() => templatePreviewNodes.value.map((item, index) => ({ item, index })))
const filteredCurrentPreviewEntries = computed(() => showChangedPreviewOnly.value
  ? currentPreviewEntries.value.filter(entry => previewDiffBadges('current', entry.item, entry.index).length > 0)
  : currentPreviewEntries.value)
const filteredTemplatePreviewEntries = computed(() => showChangedPreviewOnly.value
  ? templatePreviewEntries.value.filter(entry => previewDiffBadges('template', entry.item, entry.index).length > 0)
  : templatePreviewEntries.value)
const visibleCurrentPreviewEntries = computed(() => showAllPreviewNodes.value ? filteredCurrentPreviewEntries.value : filteredCurrentPreviewEntries.value.slice(0, 6))
const visibleTemplatePreviewEntries = computed(() => showAllPreviewNodes.value ? filteredTemplatePreviewEntries.value : filteredTemplatePreviewEntries.value.slice(0, 6))
const addableNodeOptions = computed(() => nodeTypes.value
  .filter(item => item.role === 'transform')
  .map(item => ({ label: item.label, value: item.type })))
const selectedNode = computed(() => nodes.value[selectedIndex.value] || null)
const selectedNodeIsFixed = computed(() => Boolean(selectedNode.value?.is_fixed))
const hasFixedTailNode = computed(() => Boolean(nodes.value[nodes.value.length - 1]?.is_fixed))

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

function buildSpeakerDiarizePreview(detail: unknown) {
  if (!detail || typeof detail !== 'object')
    return ''
  const segments = Array.isArray((detail as Record<string, unknown>).segments)
    ? (detail as Record<string, unknown>).segments as Array<Record<string, unknown>>
    : []
  if (!segments.length)
    return ''
  return segments
    .map((segment, index) => {
      const speaker = String(segment.speaker ?? segment.Speaker ?? segment.speaker_id ?? segment.SpeakerID ?? `speaker_${index + 1}`)
      const start = segment.start_time ?? segment.StartTime ?? '-'
      const end = segment.end_time ?? segment.EndTime ?? '-'
      return `[${speaker} ${start}s-${end}s]`
    })
    .join('\n')
}

function buildNodeTestOutputPreview(nodeType: string | undefined, outputText: string | undefined, detail: unknown) {
  if (outputText && outputText.trim())
    return outputText
  if (nodeType === 'meeting_summary')
    return buildMeetingSummaryChunkPreview(detail)
  if (nodeType === 'speaker_diarize')
    return buildSpeakerDiarizePreview(detail)
  return ''
}
const templateComparison = computed(() => {
  const currentCounts = countNodeTypes(nodes.value)
  const templateCounts = countNodeTypes(templatePreviewNodes.value)
  const types = new Set([...Object.keys(currentCounts), ...Object.keys(templateCounts)])

  let added = 0
  let removed = 0
  for (const type of types) {
    const current = currentCounts[type] || 0
    const target = templateCounts[type] || 0
    if (target > current)
      added += target - current
    else if (current > target)
      removed += current - target
  }

  let reordered = 0
  let toggled = 0
  let configChanged = 0
  const comparableLength = Math.min(nodes.value.length, templatePreviewNodes.value.length)
  for (let index = 0; index < comparableLength; index += 1) {
    const current = nodes.value[index]
    const target = templatePreviewNodes.value[index]
    if (current.node_type !== target.node_type) {
      reordered += 1
    }
    else {
      if (current.enabled !== target.enabled)
        toggled += 1
      if (normalizeConfigText(current.configText) !== normalizeConfigText(target.configText))
        configChanged += 1
    }
  }

  return {
    added,
    removed,
    reordered,
    toggled,
    configChanged,
    changed: added + removed + reordered + toggled + configChanged,
  }
})
const selectedConfig = computed<Record<string, any>>(() => {
  if (!selectedNode.value)
    return {}

  const current = parseConfigSafe(nodeDraft.configText)
  return normalizeNodeConfig(selectedNode.value.node_type, current, getNodeDefaultConfig(selectedNode.value.node_type, nodeTypes.value))
})
const selectedRegexRules = computed<RegexRule[]>(() => Array.isArray(selectedConfig.value.rules) ? selectedConfig.value.rules as RegexRule[] : [])
const selectedNodeHasDraftChanges = computed(() => {
  if (!selectedNode.value)
    return false
  return selectedNode.value.enabled !== nodeDraft.enabled
    || normalizeConfigForCompare(selectedNode.value.configText) !== normalizeConfigForCompare(nodeDraft.configText)
})
let highlightTimer: ReturnType<typeof setTimeout> | null = null
const audioFileAccept = 'audio/*,.wav,.mp3,.m4a,.aac,.flac,.ogg,.opus,.webm'

function prettyJSON(value: unknown) {
  return JSON.stringify(value, null, 2)
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
  return prettyJSON(parseConfigSafe(text))
}

function normalizeConfigForCompare(text: string) {
  const trimmed = text.trim()
  if (!trimmed)
    return '{}'
  try {
    return prettyJSON(parseConfig(trimmed))
  }
  catch {
    return trimmed
  }
}

function resetSelectedNodeDraft() {
  if (!selectedNode.value) {
    nodeDraft.enabled = true
    nodeDraft.configText = '{}'
    return
  }

  nodeDraft.enabled = selectedNode.value.enabled
  nodeDraft.configText = selectedNode.value.configText
}

function previewDiffBadges(side: 'current' | 'template', item: EditableNode, index: number) {
  const counterpart = side === 'current' ? templatePreviewNodes.value[index] : nodes.value[index]
  if (!counterpart) {
    return [side === 'current' ? '当前独有' : '模板新增']
  }

  const badges: string[] = []
  if (item.node_type !== counterpart.node_type) {
    badges.push('顺序不同')
    return badges
  }
  if (item.enabled !== counterpart.enabled)
    badges.push('开关不同')
  if (normalizeConfigText(item.configText) !== normalizeConfigText(counterpart.configText))
    badges.push('配置不同')
  return badges
}

function focusNodeByIndex(index: number) {
  if (index < 0 || index >= nodes.value.length) {
    message.info('当前工作流中没有可定位的对应节点')
    return
  }
  if (!trySelectNode(index))
    return
  highlightedNodeIndex.value = index
  if (highlightTimer)
    clearTimeout(highlightTimer)
  highlightTimer = setTimeout(() => {
    highlightedNodeIndex.value = null
  }, 1600)
  void nextTick(() => {
    nodeRowRefs.value[index]?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  })
}

function previewNodeClickable(side: 'current' | 'template', index: number) {
  if (side === 'current')
    return index >= 0 && index < nodes.value.length
  return index >= 0 && index < nodes.value.length
}

function handlePreviewNodeClick(side: 'current' | 'template', index: number) {
  if (!previewNodeClickable(side, index)) {
    message.info('当前工作流中没有可定位的对应节点')
    return
  }
  focusNodeByIndex(index)
}

function ensureNoPendingNodeChanges(actionLabel: string) {
  if (!selectedNodeHasDraftChanges.value)
    return true

  message.warning(`当前节点有未保存修改，请先保存当前节点或取消修改，再${actionLabel}`)
  return false
}

function trySelectNode(index: number) {
  if (index === selectedIndex.value)
    return true
  if (!ensureNoPendingNodeChanges('切换节点'))
    return false

  selectedIndex.value = index
  resetSelectedNodeDraft()
  return true
}

function setNodeRowRef(index: number, element: Element | ComponentPublicInstance | null) {
  if (element instanceof HTMLElement) {
    nodeRowRefs.value[index] = element
    return
  }
  if (element && '$el' in element && element.$el instanceof HTMLElement) {
    nodeRowRefs.value[index] = element.$el
    return
  }
  nodeRowRefs.value[index] = null
}

function replaceSelectedConfig(nextConfig: Record<string, unknown>) {
  if (!selectedNode.value)
    return
  const overrides = buildNodeConfigOverrides(selectedNode.value.node_type, nextConfig, getNodeDefaultConfig(selectedNode.value.node_type, nodeTypes.value))
  nodeDraft.configText = formatConfigText(overrides)
}

function updateSelectedConfig(patch: Record<string, unknown>) {
  if (!selectedNode.value)
    return
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

function sanitizeText(value?: string) {
  if (!value)
    return ''
  return value
    .replace(/language\s+[a-z_-]+<asr_text>/gi, '')
    .replace(/<\/?asr_text>/gi, '')
    .replace(/<\|[^>]+\|>/g, '')
    .replace(/\u00A0/g, ' ')
    .trim()
}

function formatExecutionStatus(value?: string) {
  const map: Record<string, string> = {
    pending: '待执行',
    running: '执行中',
    completed: '已完成',
    failed: '失败',
    success: '成功',
    skipped: '跳过',
  }
  return map[value || ''] || value || '-'
}

function isAudioDrivenNodeType(type?: string | null) {
  return type === 'batch_asr' || type === 'realtime_asr' || type === 'speaker_diarize'
}

function previewModeForNodeType(type?: string | null): 'diff' | 'plain' | 'markdown' {
  if (type === 'meeting_summary')
    return 'markdown'
  return isAudioDrivenNodeType(type) ? 'plain' : 'diff'
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

const selectedNodeNeedsAudio = computed(() => isAudioDrivenNodeType(selectedNode.value?.node_type))
const firstEnabledNode = computed(() => nodes.value.find(item => item.enabled) || null)
const workflowNeedsAudio = computed(() => isAudioDrivenNodeType(firstEnabledNode.value?.node_type))
const nodeTestInputPreview = computed(() => selectedNodeNeedsAudio.value
  ? (nodeTestAudioFile.value ? `已上传音频：${nodeTestAudioFile.value.name}` : '尚未选择音频文件')
  : nodeTestInput.value)

const nodeTestHint = computed(() => {
  if (!selectedNode.value)
    return ''
  if (selectedNode.value.node_type === 'speaker_diarize')
    return '上传音频后会直接调用说话人分离服务，并返回当前节点输出。'
  if (selectedNode.value.node_type === 'batch_asr' || selectedNode.value.node_type === 'realtime_asr')
    return '上传音频后会先做一次短音频识别，返回当前源节点的识别结果。'
  if (selectedNode.value.node_type === 'voice_wake')
    return '输入一段转写文本，验证是否能命中唤醒词、同音词，以及能否正确提取尾随指令。'
  return '输入样本文本，验证当前节点输出。'
})
const workflowTestHint = computed(() => {
  if (!workflowNeedsAudio.value)
    return '输入整条工作流测试文本。'
  if (firstEnabledNode.value?.node_type === 'speaker_diarize')
    return '首个节点依赖音频输入，执行时会上传音频并直接进入工作流。'
  return '首个节点是音频源，执行时会先识别音频，再串行执行后续工作流。'
})

function workflowTypeLabel(value?: typeof workflow.workflow_type) {
  const map: Record<string, string> = {
    legacy: '旧版文本后处理',
    batch_transcription: '批量转写',
    realtime_transcription: '实时语音识别',
    meeting: '会议纪要',
    voice_control: '语音控制',
  }
  return map[value || ''] || value || '-'
}

function sourceKindLabel(value?: typeof workflow.source_kind) {
  const map: Record<string, string> = {
    legacy_text: '旧版文本输入',
    batch_asr: '非实时语音转写',
    realtime_asr: '实时语音转写',
    voice_wake: '唤醒词识别',
  }
  return map[value || ''] || value || '-'
}

function targetKindLabel(value?: typeof workflow.target_kind) {
  const map: Record<string, string> = {
    transcript: '整理后文本',
    meeting_summary: '会议纪要',
    voice_command: '控制指令结果',
  }
  return map[value || ''] || value || '-'
}

function textToList(value: string) {
  return value
    .split(/[\n,，]/)
    .map(item => item.trim())
    .filter(Boolean)
}

function publishStatusMeta() {
  if (workflow.owner_type === 'system') {
    return workflow.is_published
      ? { label: '已上架', type: 'success', toggleLabel: '上架' }
      : { label: '未上架', type: 'default', toggleLabel: '上架' }
  }

  return workflow.is_published
    ? { label: '已发布', type: 'success', toggleLabel: '发布' }
    : { label: '草稿', type: 'warning', toggleLabel: '发布' }
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

function syncPositions() {
  nodes.value = nodes.value.map((item, index) => ({
    ...item,
    position: index + 1,
  }))
}

function mapWorkflowNodes(items: any[] = []): EditableNode[] {
  return items.map((item: any, index: number) => ({
    id: item.id,
    node_type: item.node_type,
    enabled: item.enabled,
    position: item.position || index + 1,
    configText: formatConfigText(item.config || {}),
    is_fixed: Boolean(item.is_fixed),
  }))
}

function isTemplateCompatible(option: TemplateOption) {
  return option.workflow_type === workflow.workflow_type
    && option.source_kind === workflow.source_kind
    && option.target_kind === workflow.target_kind
    && Boolean(option.is_legacy) === workflow.is_legacy
}

function addNodeInsertIndex() {
  if (hasFixedTailNode.value)
    return Math.max(nodes.value.length - 1, 0)
  return nodes.value.length
}

function canMoveNode(index: number, delta: number) {
  const current = nodes.value[index]
  if (!current || current.is_fixed)
    return false
  const target = index + delta
  if (target < 0 || target >= nodes.value.length)
    return false
  return !nodes.value[target]?.is_fixed
}

function buildNodePayload() {
  const payload = []
  for (const item of nodes.value) {
    const rawConfig = parseConfig(item.configText)
    payload.push({
      node_type: item.node_type,
      position: item.position,
      enabled: item.enabled,
      config: buildNodeConfigOverrides(item.node_type, rawConfig, getNodeDefaultConfig(item.node_type, nodeTypes.value)),
    })
  }
  return payload
}

function buildNodePayloadWithCurrentDraft() {
  return nodes.value.map((item, index) => {
    const isSelected = index === selectedIndex.value && selectedNode.value
    const enabled = isSelected ? nodeDraft.enabled : item.enabled
    const configText = isSelected ? nodeDraft.configText : item.configText
    const rawConfig = parseConfig(configText)

    return {
      node_type: item.node_type,
      position: item.position,
      enabled,
      config: buildNodeConfigOverrides(item.node_type, rawConfig, getNodeDefaultConfig(item.node_type, nodeTypes.value)),
    }
  })
}

function applySavedWorkflowState(saved: any) {
  applySavedWorkflowProfile(saved)
  if (Array.isArray(saved?.nodes)) {
    const currentSelected = selectedNode.value
    const nextNodes = mapWorkflowNodes(saved.nodes)
    nodes.value = nextNodes
    if (!nextNodes.length) {
      selectedIndex.value = 0
      resetSelectedNodeDraft()
      return
    }

    if (currentSelected) {
      const nextIndex = nextNodes.findIndex(item => item.position === currentSelected.position && item.node_type === currentSelected.node_type)
      selectedIndex.value = nextIndex >= 0 ? nextIndex : Math.min(selectedIndex.value, nextNodes.length - 1)
    }
    else {
      selectedIndex.value = Math.min(selectedIndex.value, nextNodes.length - 1)
    }
  }
  resetSelectedNodeDraft()
}

function applySavedWorkflowProfile(saved: any) {
  workflow.workflow_type = saved?.workflow_type || workflow.workflow_type
  workflow.source_kind = saved?.source_kind || workflow.source_kind
  workflow.target_kind = saved?.target_kind || workflow.target_kind
  workflow.is_legacy = Boolean(saved?.is_legacy)
  workflow.validation_message = saved?.validation_message || ''
}

function openSaveAsDialog() {
  saveAsForm.name = `${workflow.name.trim() || '未命名工作流'} (副本)`
  saveAsForm.description = workflow.description.trim()
  showSaveAsDialog.value = true
}

function openSourceWorkflow() {
  if (!workflow.source_id) {
    message.info('当前工作流没有来源记录')
    return
  }
  router.push(`/workflows/${workflow.source_id}`)
}

async function loadSourceWorkflowInfo() {
  if (!workflow.source_id) {
    sourceWorkflowName.value = ''
    return
  }
  try {
    const result = await getWorkflow(workflow.source_id)
    sourceWorkflowName.value = result.data?.name || ''
  }
  catch {
    sourceWorkflowName.value = ''
  }
}

function countNodeTypes(items: EditableNode[]) {
  const counts: Record<string, number> = {}
  for (const item of items)
    counts[item.node_type] = (counts[item.node_type] || 0) + 1
  return counts
}

function labelFor(type: string) {
  return nodeTypes.value.find(item => item.type === type)?.label || type
}

function nodeAccentColor(_type: string): string {
  return '#0f766e'
}

function handleAddNode() {
  if (!ensureNoPendingNodeChanges('添加节点'))
    return
  if (!nodeType.value) {
    message.warning('请选择节点类型')
    return
  }
  const insertIndex = addNodeInsertIndex()
  nodes.value.splice(insertIndex, 0, {
    node_type: nodeType.value,
    enabled: true,
    position: insertIndex + 1,
    configText: '{}',
  })
  syncPositions()
  selectedIndex.value = insertIndex
  resetSelectedNodeDraft()
}

function handleRemoveNode(index: number) {
  if (!ensureNoPendingNodeChanges('删除节点'))
    return
  if (nodes.value[index]?.is_fixed) {
    message.warning('固化节点不能删除')
    return
  }
  nodes.value.splice(index, 1)
  syncPositions()
  selectedIndex.value = Math.max(0, Math.min(selectedIndex.value, nodes.value.length - 1))
  resetSelectedNodeDraft()
}

function moveNode(index: number, delta: number) {
  if (!ensureNoPendingNodeChanges('调整节点顺序'))
    return
  if (!canMoveNode(index, delta)) {
    message.warning('固化节点不能移动，普通节点也不能跨过固化边界')
    return
  }
  const target = index + delta
  const next = [...nodes.value]
  const current = next[index]
  next[index] = next[target]
  next[target] = current
  nodes.value = next
  syncPositions()
  selectedIndex.value = target
  resetSelectedNodeDraft()
}

async function loadPage() {
  if (!workflowId.value) {
    message.error('无效的工作流 ID')
    router.push('/workflows')
    return
  }

  loading.value = true
  try {
    const [workflowResult, typeResult] = await Promise.all([
      getWorkflow(workflowId.value),
      getNodeTypes(),
    ])
    const wf = workflowResult.data
    workflow.name = wf.name || ''
    workflow.description = wf.description || ''
    workflow.workflow_type = wf.workflow_type || 'legacy'
    workflow.source_kind = wf.source_kind || 'legacy_text'
    workflow.target_kind = wf.target_kind || 'transcript'
    workflow.is_legacy = Boolean(wf.is_legacy)
    workflow.validation_message = wf.validation_message || ''
    workflow.is_published = Boolean(wf.is_published)
    workflow.owner_type = wf.owner_type || ''
    workflow.source_id = typeof wf.source_id === 'number' ? wf.source_id : null
    nodes.value = mapWorkflowNodes(wf.nodes || [])
    nodeTypes.value = typeResult.data || []
    selectedTemplateId.value = workflow.source_id
    if (nodes.value.length > 0)
      selectedIndex.value = 0
    resetSelectedNodeDraft()
  }
  catch {
    message.error('工作流加载失败')
  }
  finally {
    loading.value = false
  }
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
    message.warning('术语词库加载失败，术语纠正节点仍可手动填写 JSON 配置')
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
    message.warning('敏感词库加载失败，敏感词节点仍可手动填写 JSON 配置')
  }
}

async function loadFillerDictOptions() {
  try {
    const result = await getFillerDicts({ offset: 0, limit: 100 })
    fillerDictOptions.value = (result.data.items || [])
      .filter((item: { id: number, name: string, scene: string, is_base: boolean }) => !item.is_base)
      .map((item: { id: number, name: string, scene: string }) => ({
        label: `${item.name} / ${item.scene}`,
        value: item.id,
      }))
  }
  catch {
    message.warning('语气词库加载失败，语气词节点仍可手动填写 JSON 配置')
  }
}

async function loadVoiceCommandDictOptions() {
  try {
    const result = await getVoiceCommandDicts({ offset: 0, limit: 100 })
    voiceCommandDictOptions.value = (result.data.items || [])
      .filter((item: { id: number, name: string, group_key: string, is_base: boolean }) => !item.is_base)
      .map((item: { id: number, name: string, group_key: string }) => ({
        label: `${item.name} / ${item.group_key}`,
        value: item.id,
      }))
  }
  catch {
    message.warning('控制指令组加载失败，语音控制节点仍可手动填写 JSON 配置')
  }
}

async function loadTemplateOptions() {
  templateLoading.value = true
  try {
    const result = await getWorkflows({ offset: 0, limit: 100, scope: 'system' })
    templateOptions.value = (result.data.items || [])
      .map((item: any) => ({
        label: item.name,
        value: item.id,
        description: item.description || '',
        workflow_type: item.workflow_type,
        source_kind: item.source_kind,
        target_kind: item.target_kind,
        is_legacy: Boolean(item.is_legacy),
      }))
      .filter((item: TemplateOption) => isTemplateCompatible(item))
    if (!templateOptions.value.some(item => item.value === selectedTemplateId.value))
      selectedTemplateId.value = null
    if (!selectedTemplateId.value && templateOptions.value.length > 0)
      selectedTemplateId.value = templateOptions.value[0].value
  }
  catch {
    message.warning('系统模板加载失败，仍可手动编辑节点')
  }
  finally {
    templateLoading.value = false
  }
}

async function loadTemplatePreview() {
  if (!selectedTemplateId.value) {
    templatePreviewName.value = ''
    templatePreviewNodes.value = []
    return
  }

  templatePreviewLoading.value = true
  try {
    const result = await getWorkflow(selectedTemplateId.value)
    templatePreviewName.value = result.data.name || ''
    templatePreviewNodes.value = mapWorkflowNodes(result.data.nodes || [])
    showAllPreviewNodes.value = false
    showChangedPreviewOnly.value = false
  }
  catch {
    templatePreviewName.value = ''
    templatePreviewNodes.value = []
    message.warning('模板预览加载失败')
  }
  finally {
    templatePreviewLoading.value = false
  }
}

async function handleImportTemplate() {
  if (!ensureNoPendingNodeChanges('导入模板节点'))
    return
  if (!selectedTemplateId.value) {
    message.warning('请先选择一个系统模板')
    return
  }

  if (templatePreviewLoading.value) {
    message.warning('模板预览还在加载，请稍后再试')
    return
  }

  const templateName = templatePreviewName.value || selectedTemplateId.value
  const confirmed = await confirmAction({
    title: '确认导入模板节点',
    message: `将使用模板「${templateName}」覆盖当前节点链路。`,
    description: `仅替换节点顺序、启用状态和配置，不会改动工作流名称、描述和发布状态。新增 ${templateComparison.value.added} · 移除 ${templateComparison.value.removed} · 顺序变化 ${templateComparison.value.reordered} · 开关变化 ${templateComparison.value.toggled} · 配置变化 ${templateComparison.value.configChanged}。`,
    positiveText: '确认替换节点',
    positiveType: 'warning',
  })
  if (!confirmed)
    return

  await confirmImportTemplate()
}

async function confirmImportTemplate() {
  if (!selectedTemplateId.value) {
    message.warning('请先选择一个系统模板')
    return
  }

  importingTemplate.value = true
  try {
    let nextNodes = templatePreviewNodes.value
    let nextTemplateName = templatePreviewName.value

    if (!nextTemplateName) {
      const result = await getWorkflow(selectedTemplateId.value)
      nextTemplateName = result.data.name || ''
      nextNodes = mapWorkflowNodes(result.data.nodes || [])
    }

    nodes.value = nextNodes.map(item => ({
      node_type: item.node_type,
      enabled: item.enabled,
      position: item.position,
      configText: item.configText,
      is_fixed: item.is_fixed,
    }))
    syncPositions()
    selectedIndex.value = 0
    nodeTestOutput.value = ''
    nodeTestDetail.value = null
    executeOutput.value = ''
    executeResult.value = null
    workflow.source_id = selectedTemplateId.value
    message.success(`已从模板「${nextTemplateName || selectedTemplateId.value}」重置当前节点流程`)
    resetSelectedNodeDraft()
  }
  catch {
    message.error('系统模板导入失败')
  }
  finally {
    importingTemplate.value = false
  }
}

async function handleSave(showToast = true) {
  if (!ensureNoPendingNodeChanges('保存工作流'))
    return false
  if (!workflow.name.trim()) {
    message.warning('请填写工作流名称')
    return false
  }

  let payload = []
  try {
    payload = buildNodePayload()
  }
  catch (error) {
    message.error(`节点配置 JSON 不合法：${error instanceof Error ? error.message : '未知错误'}`)
    return false
  }

  saving.value = true
  try {
    await updateWorkflow(workflowId.value, {
      name: workflow.name.trim(),
      description: workflow.description.trim(),
      is_published: workflow.is_published,
    })
    const saved = await updateWorkflowNodes(workflowId.value, payload)
    applySavedWorkflowState(saved.data)
    if (showToast)
      message.success('工作流已保存')
    return true
  }
  catch {
    message.error('工作流保存失败')
    return false
  }
  finally {
    saving.value = false
  }
}

async function handleSaveAsNewWorkflow() {
  if (!ensureNoPendingNodeChanges('另存为新工作流'))
    return
  if (!saveAsForm.name.trim()) {
    message.warning('请填写新工作流名称')
    return
  }

  let payload = []
  try {
    payload = buildNodePayload()
  }
  catch (error) {
    message.error(`节点配置 JSON 不合法：${error instanceof Error ? error.message : '未知错误'}`)
    return
  }

  savingAsNew.value = true
  try {
    const created = await createWorkflow({
      name: saveAsForm.name.trim(),
      description: saveAsForm.description.trim(),
      source_id: workflowId.value,
      workflow_type: workflow.workflow_type === 'legacy' ? undefined : workflow.workflow_type,
    })
    const nextId = created.data?.id
    if (!nextId)
      throw new Error('新工作流 ID 缺失')

    await updateWorkflow(nextId, {
      name: saveAsForm.name.trim(),
      description: saveAsForm.description.trim(),
      is_published: workflow.is_published,
    })
    await updateWorkflowNodes(nextId, payload)

    showSaveAsDialog.value = false
    message.success('已另存为新工作流')
    router.push(`/workflows/${nextId}`)
  }
  catch {
    message.error('另存为新工作流失败')
  }
  finally {
    savingAsNew.value = false
  }
}

async function handlePublishedChange(nextValue: boolean) {
  if (!nextValue && workflow.is_published) {
    const label = workflow.owner_type === 'system' ? '系统模板' : '工作流'
    const name = workflow.name.trim() || `#${workflowId.value}`
    const confirmed = await confirmAction({
      title: `确认取消发布${label}`,
      message: `准备将${label}「${name}」标记为未发布。`,
      description: '当前修改只会更新编辑器状态，点击“保存工作流”后才会真正生效。取消发布后，这条内容会从可发布工作流视图中隐藏。',
      positiveText: '继续取消发布',
      positiveType: 'warning',
    })
    if (!confirmed)
      return
  }

  workflow.is_published = nextValue
}

async function handleDeleteWorkflow() {
  if (!ensureNoPendingNodeChanges('删除工作流'))
    return
  if (!workflowId.value) {
    message.error('无效的工作流 ID')
    return
  }

  const label = workflow.owner_type === 'system' ? '系统模板' : '工作流'
  const name = workflow.name.trim() || `#${workflowId.value}`
  const confirmed = await confirmDelete({
    entityType: label,
    entityName: name,
    description: workflow.owner_type === 'system'
      ? '删除后，这条系统模板将从模板列表中移除，已有副本不会自动删除。'
      : '删除后，这条工作流及其节点配置会一并移除，已有执行记录不会自动回滚。',
  })
  if (!confirmed)
    return

  deletingWorkflow.value = true
  try {
    await deleteWorkflow(workflowId.value)
    message.success(`${label}已删除`)
    router.push('/workflows')
  }
  catch {
    message.error(`${label}删除失败`)
  }
  finally {
    deletingWorkflow.value = false
  }
}

async function handleSaveCurrentNode() {
  if (!selectedNode.value) {
    message.warning('请先选择一个节点')
    return
  }

  let payload = []
  try {
    payload = buildNodePayloadWithCurrentDraft()
  }
  catch (error) {
    message.error(`当前节点配置 JSON 不合法：${error instanceof Error ? error.message : '未知错误'}`)
    return
  }

  savingCurrentNode.value = true
  try {
    const saved = await updateWorkflowNodes(workflowId.value, payload)
    applySavedWorkflowState(saved.data)
    message.success('当前节点已保存')
  }
  catch (error) {
    console.error('保存当前节点失败', error)
    message.error('当前节点保存失败')
  }
  finally {
    savingCurrentNode.value = false
  }
}

function handleCancelCurrentNodeChanges() {
  if (!selectedNode.value)
    return
  resetSelectedNodeDraft()
  message.info('已撤销当前节点的未保存修改')
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

function openExecuteAudioPicker() {
  executeAudioInputRef.value?.click()
}

function clearExecuteAudioFile() {
  executeAudioFile.value = null
}

function handleExecuteAudioSelected(event: Event) {
  const input = event.target as HTMLInputElement
  executeAudioFile.value = input.files?.[0] || null
  input.value = ''
}

async function handleTestNode() {
  if (!selectedNode.value) {
    message.warning('请先选择一个节点')
    return
  }
  if (selectedNodeNeedsAudio.value && !nodeTestAudioFile.value) {
    message.warning('请先上传音频文件')
    return
  }
  testingNode.value = true
  try {
    nodeTestOutput.value = ''
    nodeTestDetail.value = { status: 'starting', message: '节点测试已开始' }
    const payload = selectedNodeNeedsAudio.value
      ? (() => {
          const formData = new FormData()
          formData.append('node_type', selectedNode.value!.node_type)
          formData.append('config', JSON.stringify(parseConfig(nodeDraft.configText)))
          formData.append('file', nodeTestAudioFile.value!)
          return formData
        })()
      : {
          node_type: selectedNode.value.node_type,
          config: parseConfig(nodeDraft.configText),
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
          const preview = buildNodeTestOutputPreview(selectedNode.value?.node_type, nodeTestOutput.value, nextDetail)
          if (preview)
            nodeTestOutput.value = preview
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
        nodeTestDetail.value = event.detail ?? event.error ?? nodeTestDetail.value
        nodeTestOutput.value = buildNodeTestOutputPreview(selectedNode.value?.node_type, event.output_text, nodeTestDetail.value)
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
    testingNode.value = false
  }
}

async function handleExecuteWorkflow() {
  const ok = await handleSave(false)
  if (!ok)
    return
  if (workflowNeedsAudio.value && !executeAudioFile.value) {
    message.warning('请先上传音频文件')
    return
  }

  executing.value = true
  executeResult.value = null
  try {
    const payload = workflowNeedsAudio.value
      ? (() => {
          const formData = new FormData()
          formData.append('file', executeAudioFile.value!)
          return formData
        })()
      : {
          input_text: executeInput.value,
        }
    const result = await executeWorkflow(workflowId.value, payload)
    executeOutput.value = result.data.final_text || ''
    executeResult.value = result.data
    message.success(result.data.status === 'failed' ? '工作流执行完成，但有节点失败' : '工作流执行完成')
  }
  catch {
    executeOutput.value = ''
    executeResult.value = null
    message.error('工作流执行失败')
  }
  finally {
    executing.value = false
  }
}

onMounted(async () => {
  await loadPage()
  await Promise.all([loadTermDictOptions(), loadFillerDictOptions(), loadSensitiveDictOptions(), loadVoiceCommandDictOptions(), loadTemplateOptions()])
})

onBeforeUnmount(() => {
  if (highlightTimer)
    clearTimeout(highlightTimer)
})

watch(selectedTemplateId, () => {
  void loadTemplatePreview()
})

watch(() => workflow.source_id, () => {
  void loadSourceWorkflowInfo()
})

watch(() => [workflow.workflow_type, workflow.source_kind, workflow.target_kind, workflow.is_legacy], () => {
  void loadTemplateOptions()
})

watch(selectedIndex, () => {
  resetSelectedNodeDraft()
})
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-col gap-5 overflow-x-hidden overflow-y-auto pr-1">
    <NSpin :show="loading" class="min-h-0 flex-1">
      <div class="flex min-h-full min-w-0 flex-col gap-5 pb-1">
        <NCard class="card-main shrink-0" content-style="padding: 16px 20px;">
          <template #header>
            <div class="flex flex-col gap-3">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <span class="text-sm font-700 text-ink">工作流基础信息</span>
                <div class="flex items-center gap-2">
                  <NButton quaternary size="small" @click="router.push('/workflows')">
                    返回列表
                  </NButton>
                  <NButton size="small" type="error" secondary :loading="deletingWorkflow" @click="handleDeleteWorkflow">
                    删除{{ workflow.owner_type === 'system' ? '模板' : '工作流' }}
                  </NButton>
                  <NButton size="small" @click="openSaveAsDialog">
                    另存为
                  </NButton>
                  <NButton size="small" type="primary" color="#0f766e" :loading="saving" @click="handleSave()">
                    保存
                  </NButton>
                </div>
              </div>
              <div class="flex flex-wrap items-center gap-1.5">
                <NTag size="small" round :bordered="false" :type="publishStatusMeta().type as any">
                  {{ publishStatusMeta().label }}
                </NTag>
                <NTag size="small" round :bordered="false" :type="workflow.is_legacy ? 'warning' : 'info'">
                  {{ workflowTypeLabel(workflow.workflow_type) }}
                </NTag>
                <NTag size="small" round :bordered="false">
                  {{ sourceKindLabel(workflow.source_kind) }} → {{ targetKindLabel(workflow.target_kind) }}
                </NTag>
                <NTag size="small" round :bordered="false" :type="workflow.owner_type === 'system' ? 'success' : 'default'">
                  {{ workflow.owner_type === 'system' ? '系统模板' : '用户工作流' }}
                </NTag>
              </div>
            </div>
          </template>

          <!-- 基本信息 -->
          <div class="flex items-center gap-3">
            <NInput v-model:value="workflow.name" placeholder="工作流名称" class="flex-1" />
            <NInput v-model:value="workflow.description" placeholder="用途说明" class="flex-1" />
            <div class="flex flex-shrink-0 items-center gap-2 rounded-2 bg-mist/50 px-3 py-1.5">
              <span class="text-xs text-slate">{{ publishStatusMeta().toggleLabel }}</span>
              <NSwitch :value="workflow.is_published" size="small" @update:value="handlePublishedChange" />
            </div>
          </div>

          <!-- 模板导入 -->
          <div class="mt-4 rounded-2.5 border border-gray-200/50 bg-[#fbfdff] p-4">
            <div class="flex flex-wrap items-center gap-2">
              <span class="text-xs font-600 text-ink">从系统模板重置节点</span>
              <NButton v-if="workflow.source_id" text size="tiny" type="primary" @click="openSourceWorkflow">
                来源：{{ sourceWorkflowName || `#${workflow.source_id}` }}
              </NButton>
            </div>
            <div v-if="workflow.validation_message" class="mt-2 text-xs leading-5" :class="workflow.is_legacy ? 'text-amber-700' : 'text-slate/70'">
              {{ workflow.validation_message }}
            </div>
            <div class="mt-2.5 flex items-center gap-2">
              <NSelect
                v-model:value="selectedTemplateId"
                clearable
                :loading="templateLoading"
                :options="templateOptions"
                placeholder="选择系统模板"
                class="flex-1"
                size="small"
              />
              <NButton size="small" type="primary" color="#0f766e" :loading="importingTemplate" @click="handleImportTemplate">
                导入
              </NButton>
            </div>
            <div class="mt-2 text-xs leading-5 text-slate/70">
              {{ selectedTemplateDescription || '仅替换节点顺序、开关和配置，不会覆盖名称和发布状态。' }}
            </div>
            <div v-if="selectedTemplateMeta.scenarios.length" class="mt-2 flex flex-wrap gap-1.5">
              <span
                v-for="scenario in selectedTemplateMeta.scenarios"
                :key="scenario"
                class="rounded-full bg-white px-2 py-0.5 text-[11px] text-slate"
              >
                {{ scenario }}
              </span>
            </div>
            <div v-if="selectedTemplateMeta.summary" class="mt-2 rounded-2 bg-white/70 px-3 py-2 text-xs leading-5 text-slate/80">
              {{ selectedTemplateMeta.summary }}
            </div>
          </div>

          <!-- 导入预览 -->
          <div v-if="selectedTemplateId" class="mt-3 rounded-2.5 border border-gray-200/50 bg-[#fbfdff] p-4">
            <div v-if="templatePreviewLoading" class="text-[11px] text-slate/60">
              加载中...
            </div>

            <div v-else class="flex flex-wrap items-center justify-between gap-2">
              <div class="flex flex-wrap items-center gap-1.5">
                <span v-if="templatePreviewName" class="inline-flex items-center rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-slate/70">
                  {{ templatePreviewName }}
                </span>
                <span class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-slate">
                  <span class="inline-flex h-2.5 w-2.5 text-slate/60" v-html="iconPlus" /> {{ templateComparison.added }}
                </span>
                <span class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-slate">
                  <span class="inline-flex h-2.5 w-2.5 text-slate/60" v-html="iconMinus" /> {{ templateComparison.removed }}
                </span>
                <span class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-slate">
                  <span class="inline-flex h-2.5 w-2.5 text-slate/60" v-html="iconSort" /> {{ templateComparison.reordered }}
                </span>
                <span class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-slate">
                  <span class="inline-flex h-2.5 w-2.5 text-slate/60" v-html="iconCircleDot" /> {{ templateComparison.toggled }}
                </span>
                <span class="inline-flex items-center gap-1 rounded-full border border-gray-200 bg-white px-2 py-0.5 text-[11px] text-slate">
                  <span class="inline-flex h-2.5 w-2.5 text-slate/60" v-html="iconPencil" /> {{ templateComparison.configChanged }}
                </span>
              </div>
              <div class="flex items-center gap-2 text-[11px] text-slate/60">
                <span>{{ templatePreviewNodes.length }} 个模板节点</span>
                <NButton text size="tiny" @click="showChangedPreviewOnly = !showChangedPreviewOnly">
                  {{ showChangedPreviewOnly ? '全部' : '仅变化' }}
                </NButton>
                <NButton text size="tiny" @click="showAllPreviewNodes = !showAllPreviewNodes">
                  {{ showAllPreviewNodes ? '收起' : '展开' }}
                </NButton>
              </div>
            </div>

            <div v-if="selectedTemplateId" class="mt-3 grid gap-4 xl:grid-cols-[1fr_32px_1fr]">
              <!-- 当前流程 -->
              <div>
                <div class="mb-2 flex items-center gap-2 text-xs font-600 text-ink">
                  <span class="inline-block h-2 w-2 rounded-full bg-slate/40" />
                  当前流程
                  <span class="text-slate/50 font-400">{{ nodes.length }} 个节点</span>
                </div>
                <div v-if="nodes.length === 0" class="rounded-2.5 bg-mist/30 px-4 py-6 text-center text-xs text-slate">
                  当前工作流还没有节点。
                </div>
                <div v-else class="space-y-1.5">
                  <button
                    v-for="entry in visibleCurrentPreviewEntries"
                    :key="`current-${entry.item.position}-${entry.item.node_type}`"
                    type="button"
                    class="group flex w-full items-center gap-2.5 rounded-2 border bg-white px-3 py-2 text-left transition-all duration-150 hover:shadow-sm"
                    :class="[
                      selectedIndex === entry.index ? 'border-teal/40 shadow-sm' : 'border-gray-200/60 hover:border-gray-300',
                      previewDiffBadges('current', entry.item, entry.index).length > 0 ? 'border-l-3 border-l-teal-400/60' : 'border-l-3 border-l-gray-200',
                    ]"
                    @click="handlePreviewNodeClick('current', entry.index)"
                  >
                    <span
                      class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full text-[10px] font-700 text-white"
                      :style="{ backgroundColor: nodeAccentColor(entry.item.node_type) }"
                    >
                      {{ entry.item.position }}
                    </span>
                    <span class="min-w-0 flex-1 truncate text-[13px] text-ink">{{ labelFor(entry.item.node_type) }}</span>
                    <div class="flex items-center gap-1.5">
                      <span
                        v-for="badge in previewDiffBadges('current', entry.item, entry.index)"
                        :key="`current-${entry.item.position}-${badge}`"
                        class="rounded-full bg-mist px-1.5 py-0.5 text-[10px] leading-tight text-slate"
                      >
                        {{ badge }}
                      </span>
                      <span class="inline-block h-1.5 w-1.5 rounded-full" :class="entry.item.enabled ? 'bg-teal-500' : 'bg-gray-300'" />
                    </div>
                  </button>
                  <div v-if="showChangedPreviewOnly && filteredCurrentPreviewEntries.length === 0" class="px-2 py-3 text-center text-xs text-slate">
                    当前流程没有检测到变化项。
                  </div>
                  <div v-else-if="!showAllPreviewNodes && filteredCurrentPreviewEntries.length > 6" class="px-2 pt-1 text-center text-xs text-slate/60">
                    还有 {{ filteredCurrentPreviewEntries.length - 6 }} 个节点未展开
                  </div>
                </div>
              </div>

              <!-- 中间箭头 -->
              <div class="hidden items-center justify-center xl:flex">
                <span class="inline-flex h-5 w-5 text-slate/30" v-html="iconArrowRight" />
              </div>

              <!-- 模板流程 -->
              <div>
                <div class="mb-2 flex items-center gap-2 text-xs font-600 text-ink">
                  <span class="inline-block h-2 w-2 rounded-full bg-teal-500" />
                  模板流程
                  <span class="text-slate/50 font-400">{{ templatePreviewNodes.length }} 个节点</span>
                </div>
                <div v-if="!templatePreviewNodes.length" class="rounded-2.5 bg-mist/30 px-4 py-6 text-center text-xs text-slate">
                  当前模板没有可预览的节点。
                </div>
                <div v-else class="space-y-1.5">
                  <button
                    v-for="entry in visibleTemplatePreviewEntries"
                    :key="`template-${entry.item.position}-${entry.item.node_type}`"
                    type="button"
                    class="group flex w-full items-center gap-2.5 rounded-2 border bg-white px-3 py-2 text-left transition-all duration-150"
                    :class="[
                      previewNodeClickable('template', entry.index) ? 'hover:border-gray-300 hover:shadow-sm cursor-pointer' : 'opacity-60 cursor-not-allowed',
                      previewDiffBadges('template', entry.item, entry.index).length > 0 ? 'border-l-3 border-l-teal-400/60 border-gray-200/60' : 'border-l-3 border-l-gray-200 border-gray-200/60',
                    ]"
                    @click="handlePreviewNodeClick('template', entry.index)"
                  >
                    <span
                      class="flex h-5 w-5 flex-shrink-0 items-center justify-center rounded-full text-[10px] font-700 text-white"
                      :style="{ backgroundColor: nodeAccentColor(entry.item.node_type) }"
                    >
                      {{ entry.item.position }}
                    </span>
                    <span class="min-w-0 flex-1 truncate text-[13px] text-ink">{{ labelFor(entry.item.node_type) }}</span>
                    <div class="flex items-center gap-1.5">
                      <span
                        v-for="badge in previewDiffBadges('template', entry.item, entry.index)"
                        :key="`template-${entry.item.position}-${badge}`"
                        class="rounded-full bg-mist px-1.5 py-0.5 text-[10px] leading-tight text-slate"
                      >
                        {{ badge }}
                      </span>
                      <span class="inline-block h-1.5 w-1.5 rounded-full" :class="entry.item.enabled ? 'bg-teal-500' : 'bg-gray-300'" />
                    </div>
                  </button>
                  <div v-if="showChangedPreviewOnly && filteredTemplatePreviewEntries.length === 0" class="px-2 py-3 text-center text-xs text-slate">
                    模板流程没有额外变化项。
                  </div>
                  <div v-else-if="!showAllPreviewNodes && filteredTemplatePreviewEntries.length > 6" class="px-2 pt-1 text-center text-xs text-slate/60">
                    还有 {{ filteredTemplatePreviewEntries.length - 6 }} 个节点未展开
                  </div>
                </div>
              </div>
            </div>
          </div>
        </NCard>

        <div class="grid min-h-0 flex-1 gap-5 xl:grid-cols-[280px_minmax(360px,1fr)_minmax(320px,0.9fr)]">
          <NCard class="card-main min-h-0 overflow-hidden" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;">
            <template #header>
              <span class="text-sm font-600">添加节点</span>
            </template>
            <div class="grid gap-3">
              <NSelect v-model:value="nodeType" :options="addableNodeOptions" placeholder="选择节点类型" />
              <div class="text-xs leading-5 text-slate">
                当前场景的首尾节点已固化，这里只允许添加中间处理节点。
              </div>
              <NButton type="primary" color="#0f766e" @click="handleAddNode">
                添加到流程
              </NButton>
            </div>
            <div class="mt-4 flex-1 min-h-0 overflow-y-auto space-y-2 p-1">
              <div v-if="nodes.length === 0" class="rounded-2.5 bg-mist/40 px-4 py-8 text-center text-sm text-slate">
                当前没有节点。<br>先从上方选择一个节点类型加入流程。
              </div>
              <div
                v-for="(item, index) in nodes"
                :key="`${item.node_type}-${index}`"
                :ref="(element) => setNodeRowRef(index, element)"
                class="group relative cursor-pointer overflow-hidden rounded-2.5 border transition-all duration-200"
                :class="[
                  selectedIndex === index
                    ? 'border-teal/50 bg-white shadow-[0_2px_8px_rgba(15,118,110,0.10)]'
                    : 'border-gray-200/60 bg-white hover:border-gray-300 hover:shadow-[0_1px_4px_rgba(0,0,0,0.06)]',
                  highlightedNodeIndex === index ? 'ring-2 ring-teal/30' : '',
                ]"
                @click="trySelectNode(index)"
              >
                <div
                  class="absolute inset-y-0 left-0 w-[3px] transition-opacity duration-200"
                  :class="selectedIndex === index ? 'opacity-100' : 'opacity-0 group-hover:opacity-60'"
                  :style="{ backgroundColor: nodeAccentColor(item.node_type) }"
                />
                <div class="flex items-center gap-3 px-3.5 py-3">
                  <div
                    class="flex h-6 w-6 flex-shrink-0 items-center justify-center rounded-full text-[11px] font-700 text-white"
                    :style="{ backgroundColor: nodeAccentColor(item.node_type) }"
                  >
                    {{ index + 1 }}
                  </div>
                  <div class="min-w-0 flex-1">
                    <div class="truncate text-[13px] font-600 leading-tight text-ink">
                      {{ labelFor(item.node_type) }}
                    </div>
                    <div class="mt-0.5 flex flex-wrap items-center gap-1.5 text-[11px] leading-tight" :class="item.enabled ? 'text-teal-600/70' : 'text-slate/50'">
                      <span class="inline-block h-1.5 w-1.5 rounded-full" :class="item.enabled ? 'bg-teal-500' : 'bg-gray-300'" />
                      {{ item.enabled ? '已启用' : '已禁用' }}
                      <span v-if="item.is_fixed" class="rounded-full bg-amber-50 px-1.5 py-0.5 text-[10px] text-amber-700">
                        固化节点
                      </span>
                    </div>
                  </div>
                  <div class="flex items-center gap-0.5 opacity-0 transition-opacity duration-150 group-hover:opacity-100" :class="selectedIndex === index ? '!opacity-100' : ''" @click.stop>
                    <NButton text size="tiny" :disabled="!canMoveNode(index, -1)" class="!px-1" @click="moveNode(index, -1)">
                      <span class="inline-flex h-3.5 w-3.5" v-html="iconChevronUp" />
                    </NButton>
                    <NButton text size="tiny" :disabled="!canMoveNode(index, 1)" class="!px-1" @click="moveNode(index, 1)">
                      <span class="inline-flex h-3.5 w-3.5" v-html="iconChevronDown" />
                    </NButton>
                    <NButton text size="tiny" type="error" :disabled="item.is_fixed" class="!px-1" @click="handleRemoveNode(index)">
                      <span class="inline-flex h-3.5 w-3.5" v-html="iconX" />
                    </NButton>
                  </div>
                </div>
              </div>
            </div>
          </NCard>

          <NCard class="card-main min-h-0 overflow-hidden" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;">
            <template #header>
              <span class="text-sm font-600">节点配置</span>
            </template>
            <div v-if="selectedNode" class="flex min-h-0 flex-1 flex-col gap-4">
              <div class="flex items-center justify-between rounded-2.5 bg-mist/60 px-4 py-3">
                <div>
                  <div class="flex items-center gap-2 text-sm font-700 text-ink">
                    {{ labelFor(selectedNode.node_type) }}
                    <NTag v-if="selectedNodeIsFixed" size="small" round :bordered="false" type="warning">
                      固化节点
                    </NTag>
                    <NTag v-if="selectedNodeHasDraftChanges" size="small" round :bordered="false" type="warning">
                      未保存
                    </NTag>
                  </div>
                  <div class="text-xs text-slate">
                    第 {{ selectedNode.position }} 步
                  </div>
                  <div v-if="selectedNodeIsFixed" class="mt-1 text-xs text-amber-700">
                    该节点用于固定工作流的入口或出口场景，不能删除、移动或禁用，但仍可调整配置。
                  </div>
                </div>
                <NSwitch v-model:value="nodeDraft.enabled" :disabled="selectedNodeIsFixed" />
              </div>
              <div class="grid gap-3 rounded-2.5 bg-[#fbfdff] p-4">
                <div class="flex items-center justify-between gap-3">
                  <div class="text-sm font-600 text-ink">
                    结构化配置
                  </div>
                  <div class="flex items-center gap-2">
                    <NButton text size="small" @click="showRawConfig = !showRawConfig">
                      {{ showRawConfig ? '隐藏 JSON' : '高级 JSON' }}
                    </NButton>
                    <NButton size="small" :disabled="!selectedNodeHasDraftChanges" @click="handleCancelCurrentNodeChanges">
                      取消修改
                    </NButton>
                    <NButton size="small" type="primary" color="#0f766e" :loading="savingCurrentNode" :disabled="!selectedNodeHasDraftChanges || savingCurrentNode" @click="handleSaveCurrentNode">
                      保存当前节点
                    </NButton>
                  </div>
                </div>
                <div class="text-xs leading-6 text-slate/70">
                  当前节点只需要填写局部覆写项。未填写的字段会自动继承节点管理里的默认配置。
                  <NButton text size="small" type="primary" class="ml-1" @click="router.push('/workflows/nodes')">
                    去节点管理
                  </NButton>
                </div>

                <template v-if="selectedNode.node_type === 'term_correction'">
                  <div class="grid gap-2">
                    <div class="text-xs text-slate/70">
                      术语词库
                    </div>
                    <NSelect
                      :value="selectedConfig.dict_id || null"
                      clearable
                      :options="termDictOptions"
                      placeholder="选择用于术语纠正的词库"
                      @update:value="updateSelectedConfig({ dict_id: $event || 0 })"
                    />
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'filler_filter'">
                  <div class="grid gap-3">
                    <div>
                      <div class="text-xs text-slate/70">
                        场景语气词库
                      </div>
                      <NSelect
                        :value="selectedConfig.dict_id || null"
                        clearable
                        :options="fillerDictOptions"
                        placeholder="不选则仅使用基础语气词库"
                        @update:value="updateSelectedConfig({ dict_id: $event || 0 })"
                      />
                      <div class="mt-2 text-[11px] leading-5 text-slate/65">
                        基础语气词库会自动参与过滤，这里只选择当前工作流节点额外叠加的场景词库。
                      </div>
                    </div>
                    <div>
                      <div class="text-xs text-slate/70">
                        自定义过滤词
                      </div>
                      <NInput
                        :value="listToText(selectedConfig.custom_words)"
                        type="textarea"
                        :autosize="{ minRows: 3, maxRows: 6 }"
                        placeholder="补充业务口语词，例如：然后呢、是这样的"
                        @update:value="updateSelectedConfig({ custom_words: textToList($event) })"
                      />
                    </div>
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'sensitive_filter'">
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
                        基础敏感词库会自动参与过滤，这里只选择当前工作流节点额外叠加的场景词库。
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

                <template v-else-if="selectedNode.node_type === 'llm_correction'">
                  <div class="grid gap-3">
                    <div class="grid gap-3 lg:grid-cols-2">
                      <NInput :value="selectedConfig.endpoint" placeholder="LLM Endpoint，可填 base URL、带 /v1 的 base URL 或完整 /chat/completions" @update:value="updateSelectedConfig({ endpoint: $event })" />
                      <NInput :value="selectedConfig.model" placeholder="模型名，如 qwen3-4b" @update:value="updateSelectedConfig({ model: $event })" />
                    </div>
                    <div class="text-xs leading-6 text-slate/75">
                      示例：http://192.168.200.182:9888、http://192.168.200.182:9888/v1，或 https://dashscope.aliyuncs.com/compatible-mode/v1。
                    </div>
                    <NInput :value="selectedConfig.api_key" type="password" show-password-on="click" placeholder="API Key，可留空" @update:value="updateSelectedConfig({ api_key: $event })" />
                    <div class="grid gap-3 lg:grid-cols-2">
                      <NInputNumber :value="selectedConfig.temperature" :min="0" :max="2" :step="0.1" @update:value="updateSelectedConfig({ temperature: $event ?? 0.3 })" />
                      <NInputNumber :value="selectedConfig.max_tokens" :min="1" :step="256" @update:value="updateSelectedConfig({ max_tokens: $event ?? 4096 })" />
                    </div>
                    <NInput
                      :value="selectedConfig.prompt_template"
                      type="textarea"
                      :autosize="{ minRows: 6, maxRows: 12 }"
                      placeholder="Prompt 模板，使用 {{TEXT}} 作为原文占位符"
                      @update:value="updateSelectedConfig({ prompt_template: $event })"
                    />
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'voice_intent'">
                  <div class="grid gap-3">
                    <div class="grid gap-3 lg:grid-cols-2">
                      <NInput :value="selectedConfig.endpoint" placeholder="LLM Endpoint，可填 base URL、带 /v1 的 base URL 或完整 /chat/completions" @update:value="updateSelectedConfig({ endpoint: $event })" />
                      <NInput :value="selectedConfig.model" placeholder="模型名，如 qwen3-4b" @update:value="updateSelectedConfig({ model: $event })" />
                    </div>
                    <div class="text-xs leading-6 text-slate/75">
                      该节点用于语音控制意图识别，建议配专用分类 Prompt，并通过控制指令库限定有效分组，而不是复用普通纠错节点配置。
                    </div>
                    <NInput :value="selectedConfig.api_key" type="password" show-password-on="click" placeholder="API Key，可留空" @update:value="updateSelectedConfig({ api_key: $event })" />
                    <div class="grid gap-3 lg:grid-cols-3">
                      <NInputNumber :value="selectedConfig.temperature" :min="0" :max="2" :step="0.1" @update:value="updateSelectedConfig({ temperature: $event ?? 0 })" />
                      <NInputNumber :value="selectedConfig.max_tokens" :min="1" :step="64" @update:value="updateSelectedConfig({ max_tokens: $event ?? 512 })" />
                      <div class="flex items-center gap-2 rounded-2 bg-white px-3 py-2.5">
                        <span class="text-xs text-slate">自动附加基础指令组</span>
                        <NSwitch :value="selectedConfig.include_base" @update:value="updateSelectedConfig({ include_base: $event })" />
                      </div>
                    </div>
                    <div>
                      <div class="text-xs text-slate/70">
                        额外有效分组
                      </div>
                      <NSelect
                        :value="selectedConfig.dict_ids || []"
                        multiple
                        filterable
                        clearable
                        :options="voiceCommandDictOptions"
                        placeholder="选择当前节点允许命中的额外控制指令组"
                        @update:value="updateSelectedConfig({ dict_ids: Array.isArray($event) ? $event : [] })"
                      />
                      <div class="mt-2 text-[11px] leading-5 text-slate/65">
                        基础组按开关自动叠加，这里只选择需要额外开放给当前 workflow 的控制分组。
                      </div>
                    </div>
                    <NInput
                      :value="selectedConfig.prompt_template"
                      type="textarea"
                      :autosize="{ minRows: 6, maxRows: 12 }"
                      placeholder="Prompt 模板，支持 {{TEXT}}、{{COMMAND_LIBRARY}}、{{EXTRA_PROMPT}} 占位符"
                      @update:value="updateSelectedConfig({ prompt_template: $event })"
                    />
                    <NInput
                      :value="selectedConfig.extra_prompt"
                      type="textarea"
                      :autosize="{ minRows: 3, maxRows: 6 }"
                      placeholder="可选附加提示：补充当前控制流程的限制、优先级或禁用项"
                      @update:value="updateSelectedConfig({ extra_prompt: $event })"
                    />
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'voice_wake'">
                  <div class="grid gap-3">
                    <div class="rounded-2 bg-white px-3 py-3 text-xs leading-6 text-slate/80">
                      该节点是语音控制工作流的固定源节点。它不会调用 ASR，而是对桌面端上传的转写文本做唤醒词识别；命中后会把剩余文本继续传给 voice_intent 节点。
                    </div>
                    <div>
                      <div class="text-xs text-slate/70">
                        唤醒词列表
                      </div>
                      <NInput
                        :value="listToText(selectedConfig.wake_words)"
                        type="textarea"
                        :autosize="{ minRows: 3, maxRows: 6 }"
                        placeholder="每行一个正式唤醒词，例如：你好小鲨"
                        @update:value="updateSelectedConfig({ wake_words: textToList($event) })"
                      />
                    </div>
                    <div>
                      <div class="text-xs text-slate/70">
                        同音 / 易错识别词
                      </div>
                      <NInput
                        :value="listToText(selectedConfig.homophone_words)"
                        type="textarea"
                        :autosize="{ minRows: 4, maxRows: 8 }"
                        placeholder="每行一个，例如：你好小莎、你好小沙、你好小善"
                        @update:value="updateSelectedConfig({ homophone_words: textToList($event) })"
                      />
                      <div class="mt-2 text-[11px] leading-5 text-slate/65">
                        这里建议填写 ASR 日志里出现过的同音误识别词。命中任一候选后，节点会自动截掉唤醒词，只把后面的真实控制指令交给 voice_intent。
                      </div>
                    </div>
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'speaker_diarize'">
                  <div class="grid gap-3">
                    <div class="text-xs leading-6 text-slate/75">
                      当前默认复用系统已配置的 Speaker Analysis Service。启用声纹匹配时，也会直接复用同一服务中的声纹库。
                    </div>
                    <div class="flex flex-wrap gap-6 rounded-2 bg-white/60 px-3 py-3">
                      <label class="flex items-center gap-2 text-sm text-ink">
                        <NSwitch :value="selectedConfig.enable_voiceprint_match" @update:value="updateSelectedConfig({ enable_voiceprint_match: $event })" />
                        <span>启用声纹匹配</span>
                      </label>
                      <label class="flex items-center gap-2 text-sm text-ink">
                        <NSwitch :value="selectedConfig.fail_on_error" @update:value="updateSelectedConfig({ fail_on_error: $event })" />
                        <span>失败时中断工作流</span>
                      </label>
                    </div>
                    <div class="text-xs leading-6 text-slate/75">
                      建议默认关闭“失败时中断工作流”。这样分离服务不可用时会跳过该节点，但不会阻断后面的会议摘要生成。
                    </div>
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'meeting_summary'">
                  <div class="grid gap-3">
                    <div class="grid gap-3 lg:grid-cols-2">
                      <NInput :value="selectedConfig.endpoint" placeholder="摘要 LLM Endpoint，可填 base URL、带 /v1 的 base URL 或完整 /chat/completions；留空则使用内置摘要器" @update:value="updateSelectedConfig({ endpoint: $event })" />
                      <NInput :value="selectedConfig.model" placeholder="摘要模型名" @update:value="updateSelectedConfig({ model: $event })" />
                    </div>
                    <div class="text-xs leading-6 text-slate/75">
                      示例：http://192.168.200.182:9888、http://192.168.200.182:9888/v1，或 https://dashscope.aliyuncs.com/compatible-mode/v1。
                    </div>
                    <NInput :value="selectedConfig.api_key" type="password" show-password-on="click" placeholder="API Key，可留空" @update:value="updateSelectedConfig({ api_key: $event })" />
                    <NInputNumber :value="selectedConfig.max_tokens" :min="1" :step="1024" @update:value="updateSelectedConfig({ max_tokens: $event ?? 65536 })" />
                    <NSelect
                      :value="selectedConfig.output_format || 'markdown'"
                      :options="[
                        { label: 'Markdown', value: 'markdown' },
                        { label: 'Plain Text', value: 'text' },
                      ]"
                      @update:value="updateSelectedConfig({ output_format: $event || 'markdown' })"
                    />
                    <NInput
                      :value="selectedConfig.prompt_template"
                      type="textarea"
                      :autosize="{ minRows: 6, maxRows: 12 }"
                      placeholder="会议纪要 Prompt 模板，使用 {{TEXT}} 作为转写文本占位符"
                      @update:value="updateSelectedConfig({ prompt_template: $event })"
                    />
                  </div>
                </template>

                <template v-else-if="selectedNode.node_type === 'custom_regex'">
                  <div class="grid gap-3">
                    <div v-for="(rule, index) in selectedRegexRules" :key="index" class="rounded-2 bg-white p-3">
                      <div class="flex items-center justify-between gap-2">
                        <div class="text-sm font-600 text-ink">
                          规则 {{ index + 1 }}
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
              </div>

              <NInput
                v-if="showRawConfig"
                v-model:value="nodeDraft.configText"
                type="textarea"
                :autosize="{ minRows: 10, maxRows: 20 }"
                placeholder="使用 JSON 配置当前节点"
              />
              <div class="grid min-h-0 flex-1 gap-3 rounded-2.5 bg-[#fbfdff] p-4">
                <div class="text-sm font-600 text-ink">
                  单节点测试
                </div>
                <div class="text-xs leading-6 text-slate/70">
                  {{ nodeTestHint }}
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
                <NInput v-else v-model:value="nodeTestInput" type="textarea" :autosize="{ minRows: 5, maxRows: 10 }" placeholder="输入样本文本，验证当前节点输出。" />
                <div class="flex justify-end">
                  <NButton size="small" type="primary" color="#0f766e" :loading="testingNode" @click="handleTestNode">
                    测试当前节点
                  </NButton>
                </div>
                <div class="grid gap-3 lg:grid-cols-2">
                  <TextDiffPreview :mode="previewModeForNodeType(selectedNode?.node_type)" :before-text="nodeTestInputPreview" :after-text="nodeTestOutput" :before-label="selectedNodeNeedsAudio ? '上传音频' : '测试输入'" after-label="节点输出" />
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
            </div>
            <div v-else class="flex flex-1 items-center justify-center text-sm text-slate">
              请先从左侧选择一个节点。
            </div>
          </NCard>

          <NCard class="card-main min-h-0 overflow-hidden" content-style="display:flex;flex-direction:column;min-height:0;padding:0 20px 20px;">
            <template #header>
              <span class="text-sm font-600">整条工作流测试</span>
            </template>
            <div class="flex min-h-0 flex-1 flex-col gap-3">
              <div class="text-xs leading-6 text-slate/70">
                {{ workflowTestHint }}
              </div>
              <template v-if="workflowNeedsAudio">
                <div class="rounded-2 bg-white p-3">
                  <div class="flex flex-wrap items-center justify-between gap-2">
                    <div>
                      <div class="text-sm font-600 text-ink">
                        工作流输入音频
                      </div>
                      <div class="text-xs text-slate/60">
                        {{ executeAudioFile ? `${executeAudioFile.name} · ${formatFileSize(executeAudioFile.size)}` : '根据首节点自动切换为音频输入模式。' }}
                      </div>
                    </div>
                    <div class="flex items-center gap-2">
                      <NButton size="small" @click="openExecuteAudioPicker">
                        {{ executeAudioFile ? '重新选择' : '上传音频' }}
                      </NButton>
                      <NButton v-if="executeAudioFile" size="small" quaternary @click="clearExecuteAudioFile">
                        清空
                      </NButton>
                    </div>
                  </div>
                </div>
              </template>
              <NInput v-else v-model:value="executeInput" type="textarea" :autosize="{ minRows: 9, maxRows: 14 }" placeholder="输入整条工作流测试文本。" />
              <div class="flex justify-end">
                <NButton type="primary" color="#0f766e" :loading="executing" @click="handleExecuteWorkflow">
                  保存并执行
                </NButton>
              </div>
              <div class="flex-1 min-h-0 overflow-y-auto rounded-2.5 bg-[#fbfdff] p-4">
                <div class="text-xs text-slate/70">
                  最终输出
                </div>
                <div class="mt-2 whitespace-pre-wrap text-sm leading-6 text-ink">
                  {{ executeOutput || '执行结果会显示在这里。' }}
                </div>

                <div v-if="executeResult?.error_message" class="mt-4 rounded-2 bg-red-50 p-3 text-sm text-red-600">
                  {{ executeResult.error_message }}
                </div>

                <div v-if="executeResult?.node_results?.length" class="mt-4 grid gap-3">
                  <div class="text-xs text-slate/70">
                    节点执行过程
                  </div>
                  <div v-for="node in executeResult.node_results" :key="`${node.position}-${node.label}`" class="rounded-2 bg-white p-3">
                    <div class="flex flex-wrap items-center justify-between gap-2">
                      <div class="text-sm font-700 text-ink">
                        {{ node.position }}. {{ node.label || node.node_type }}
                      </div>
                      <div class="text-xs text-slate">
                        {{ formatExecutionStatus(node.status) }} · {{ node.duration_ms || 0 }} ms
                      </div>
                    </div>
                    <div class="mt-3 grid gap-3 xl:grid-cols-2">
                      <TextDiffPreview :mode="previewModeForNodeType(node.node_type)" :before-text="sanitizeText(node.input_text)" :after-text="sanitizeText(node.output_text)" />
                    </div>
                    <div class="mt-3">
                      <NodeDetailPanel :detail="node.detail" empty-label="当前节点没有 detail 信息。" />
                    </div>
                  </div>
                </div>
              </div>
            </div>
          </NCard>
        </div>
      </div>
    </NSpin>

    <NModal v-model:show="showSaveAsDialog" preset="card" title="另存为新工作流" class="modal-card max-w-xl">
      <div class="grid gap-4">
        <NInput v-model:value="saveAsForm.name" placeholder="新工作流名称" />
        <NInput v-model:value="saveAsForm.description" type="textarea" :autosize="{ minRows: 4, maxRows: 8 }" placeholder="新工作流用途说明" />
        <div class="rounded-2.5 bg-mist/60 p-4 text-xs leading-6 text-slate">
          会把当前编辑器里的节点顺序、启用状态和配置一并保存到新工作流，适合先复制再继续试验改动。
        </div>
        <div class="flex justify-end gap-2">
          <NButton @click="showSaveAsDialog = false">
            取消
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="savingAsNew" @click="handleSaveAsNewWorkflow">
            创建副本
          </NButton>
        </div>
      </div>
    </NModal>

    <input
      ref="nodeTestAudioInputRef"
      type="file"
      :accept="audioFileAccept"
      class="hidden"
      @change="handleNodeTestAudioSelected"
    >
    <input
      ref="executeAudioInputRef"
      type="file"
      :accept="audioFileAccept"
      class="hidden"
      @change="handleExecuteAudioSelected"
    >
  </div>
</template>

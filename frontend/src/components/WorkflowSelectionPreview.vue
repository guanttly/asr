<script setup lang="ts">
import type { WorkflowPreviewItem } from '@/types/workflow'

import { computed } from 'vue'

const props = withDefaults(defineProps<{
  workflow?: WorkflowPreviewItem | null
  loading?: boolean
  emptyTitle?: string
  emptyDescription?: string
}>(), {
  workflow: null,
  loading: false,
  emptyTitle: '未选择工作流',
  emptyDescription: '选择一条工作流后，这里会显示它的适用场景、摘要说明和节点链路。',
})

const nodeLabelMap: Record<string, string> = {
  term_correction: '术语纠正',
  filler_filter: '语气词过滤',
  llm_correction: 'LLM 纠错',
  speaker_diarize: '说话人分离',
  meeting_summary: '会议纪要生成',
  custom_regex: '自定义正则替换',
}

const workflowTypeLabelMap: Record<string, string> = {
  legacy: '旧版文本后处理',
  batch_transcription: '批量转写',
  realtime_transcription: '实时语音识别',
  meeting: '会议纪要',
}

const sourceKindLabelMap: Record<string, string> = {
  legacy_text: '旧版文本输入',
  batch_asr: '非实时语音转写',
  realtime_asr: '实时语音转写',
}

const targetKindLabelMap: Record<string, string> = {
  transcript: '整理后文本',
  meeting_summary: '会议纪要',
}

const scenarioLabels = computed(() => {
  const workflow = props.workflow
  if (!workflow)
    return []

  const labels = []
  if (workflow.workflow_type)
    labels.push(workflowTypeLabelMap[workflow.workflow_type] || workflow.workflow_type)
  if (workflow.source_kind)
    labels.push(sourceKindLabelMap[workflow.source_kind] || workflow.source_kind)
  if (workflow.target_kind)
    labels.push(targetKindLabelMap[workflow.target_kind] || workflow.target_kind)
  if (workflow.is_legacy)
    labels.push('Legacy')
  return Array.from(new Set(labels))
})

const visibleNodes = computed(() => {
  const nodes = (props.workflow?.nodes || []).filter(node => node.enabled !== false)
  if (nodes.length === 0)
    return ['暂未配置启用节点']
  return nodes.slice(0, 5).map(node => node.label || nodeLabelMap[node.node_type || ''] || node.node_type || `节点 ${node.id}`)
})
const hiddenNodeCount = computed(() => Math.max(((props.workflow?.nodes || []).filter(node => node.enabled !== false).length) - 5, 0))
const workflowSummary = computed(() => {
  if (props.workflow?.validation_message)
    return props.workflow.validation_message
  return props.workflow?.description || '适合作为转写后处理的工作流起点。'
})
</script>

<template>
  <div class="rounded-2.5 bg-[#fbfdff] p-4">
    <div v-if="loading" class="text-sm text-slate">
      正在加载工作流预览...
    </div>

    <div v-else-if="workflow" class="grid gap-3">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0 flex-1">
          <div class="truncate text-sm font-700 text-ink">
            {{ workflow.label }}
          </div>
          <div class="mt-1 line-clamp-2 text-xs leading-6 text-slate">
            {{ workflow.description || '当前工作流未填写额外说明。' }}
          </div>
        </div>
        <span class="rounded-full bg-mist/80 px-2 py-0.5 text-[11px] text-slate">
          {{ workflow.owner_type === 'system' ? '系统模板' : '用户工作流' }}
        </span>
      </div>

      <div class="flex flex-wrap gap-1.5">
        <span
          v-for="scenario in scenarioLabels"
          :key="`${workflow.id}-${scenario}`"
          class="rounded-full bg-mist/80 px-2 py-0.5 text-[11px] text-slate"
          :class="scenario === 'Legacy' ? 'bg-amber-50 text-amber-700' : ''"
        >
          {{ scenario }}
        </span>
      </div>

      <div class="rounded-2 bg-white/80 px-3 py-2 text-xs leading-6" :class="workflow.is_legacy ? 'text-amber-700' : 'text-slate'">
        {{ workflowSummary }}
      </div>

      <div class="flex flex-wrap gap-1.5">
        <span
          v-for="label in visibleNodes"
          :key="`${workflow.id}-${label}`"
          class="rounded-full border border-gray-200/60 bg-white px-2 py-0.5 text-[11px] text-slate"
          :class="label === '暂未配置启用节点' ? '!bg-amber-50 !text-amber-700 !border-amber-200/60' : ''"
        >
          {{ label }}
        </span>
        <span v-if="hiddenNodeCount > 0" class="rounded-full border border-gray-200/60 bg-white px-2 py-0.5 text-[11px] text-slate">
          +{{ hiddenNodeCount }}
        </span>
      </div>

      <div class="text-xs text-slate">
        共 {{ workflow.nodes?.filter(node => node.enabled !== false).length || 0 }} 个启用节点
      </div>
    </div>

    <div v-else class="grid gap-2">
      <div class="text-sm font-600 text-ink">
        {{ emptyTitle }}
      </div>
      <div class="text-xs leading-6 text-slate">
        {{ emptyDescription }}
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed } from 'vue'

const props = defineProps<{
  detail?: Record<string, unknown> | string | null
  emptyLabel?: string
}>()

const parsedDetail = computed<Record<string, any> | null>(() => {
  if (!props.detail)
    return null
  if (typeof props.detail === 'string') {
    try {
      return JSON.parse(props.detail)
    }
    catch {
      return { raw_text: props.detail }
    }
  }
  return props.detail as Record<string, any>
})

const fallbackEntries = computed(() => {
  const detail = parsedDetail.value
  if (!detail)
    return []
  return Object.entries(detail).filter(([key]) => ![
    'layer1',
    'layer2',
    'layer3',
    'removed_words',
    'applied_rules',
    'segments',
    'segments_count',
    'prompt_tokens',
    'completion_tokens',
    'model',
    'model_version',
    'source',
    'warning',
    'words_used',
    'raw_text',
  ].includes(key))
})

function stringify(value: unknown) {
  if (value == null)
    return '-'
  if (typeof value === 'string')
    return value
  if (typeof value === 'number' || typeof value === 'boolean')
    return String(value)
  return JSON.stringify(value, null, 2)
}

function asList(value: unknown) {
  return Array.isArray(value) ? value : []
}
</script>

<template>
  <div class="node-detail-panel">
    <div v-if="!parsedDetail" class="node-detail-empty">
      {{ emptyLabel || '当前节点没有 detail 信息。' }}
    </div>

    <template v-else>
      <div v-if="parsedDetail.warning" class="node-detail-warning">
        {{ stringify(parsedDetail.warning) }}
      </div>

      <div v-if="parsedDetail.raw_text" class="node-detail-block">
        <div class="node-detail-title">
          原始文本
        </div>
        <div class="node-detail-pre">
          {{ stringify(parsedDetail.raw_text) }}
        </div>
      </div>

      <div v-if="asList(parsedDetail.layer1).length || asList(parsedDetail.layer2).length || asList(parsedDetail.layer3).length" class="node-detail-grid">
        <div v-if="asList(parsedDetail.layer1).length" class="node-detail-block">
          <div class="node-detail-title">
            第一层纠正
          </div>
          <ul class="node-detail-list">
            <li v-for="(item, index) in asList(parsedDetail.layer1)" :key="`layer1-${index}`">
              {{ stringify(item) }}
            </li>
          </ul>
        </div>
        <div v-if="asList(parsedDetail.layer2).length" class="node-detail-block">
          <div class="node-detail-title">
            第二层纠正
          </div>
          <ul class="node-detail-list">
            <li v-for="(item, index) in asList(parsedDetail.layer2)" :key="`layer2-${index}`">
              {{ stringify(item) }}
            </li>
          </ul>
        </div>
        <div v-if="asList(parsedDetail.layer3).length" class="node-detail-block">
          <div class="node-detail-title">
            第三层纠正
          </div>
          <ul class="node-detail-list">
            <li v-for="(item, index) in asList(parsedDetail.layer3)" :key="`layer3-${index}`">
              {{ stringify(item) }}
            </li>
          </ul>
        </div>
      </div>

      <div v-if="asList(parsedDetail.removed_words).length" class="node-detail-block">
        <div class="node-detail-title">
          过滤词
        </div>
        <div class="node-detail-meta">
          共使用 {{ stringify(parsedDetail.words_used) }} 个候选词
        </div>
        <ul class="node-detail-list">
          <li v-for="(item, index) in asList(parsedDetail.removed_words)" :key="`removed-${index}`">
            {{ stringify(item) }}
          </li>
        </ul>
      </div>

      <div v-if="asList(parsedDetail.applied_rules).length" class="node-detail-block">
        <div class="node-detail-title">
          命中规则
        </div>
        <div class="node-detail-table">
          <div class="node-detail-table-row node-detail-table-head">
            <div>Pattern</div>
            <div>Replacement</div>
            <div>Matches</div>
          </div>
          <div v-for="(item, index) in asList(parsedDetail.applied_rules)" :key="`rule-${index}`" class="node-detail-table-row">
            <div>{{ stringify(item.pattern) }}</div>
            <div>{{ stringify(item.replacement) }}</div>
            <div>{{ stringify(item.matches) }}</div>
          </div>
        </div>
      </div>

      <div v-if="parsedDetail.model || parsedDetail.model_version || parsedDetail.prompt_tokens || parsedDetail.completion_tokens || parsedDetail.source" class="node-detail-grid">
        <div v-if="parsedDetail.model || parsedDetail.model_version" class="node-detail-block">
          <div class="node-detail-title">
            模型信息
          </div>
          <div v-if="parsedDetail.model" class="node-detail-kv">
            模型：{{ stringify(parsedDetail.model) }}
          </div>
          <div v-if="parsedDetail.model_version" class="node-detail-kv">
            版本：{{ stringify(parsedDetail.model_version) }}
          </div>
          <div v-if="parsedDetail.source" class="node-detail-kv">
            来源：{{ stringify(parsedDetail.source) }}
          </div>
        </div>
        <div v-if="parsedDetail.prompt_tokens || parsedDetail.completion_tokens" class="node-detail-block">
          <div class="node-detail-title">
            Token 用量
          </div>
          <div v-if="parsedDetail.prompt_tokens" class="node-detail-kv">
            Prompt：{{ stringify(parsedDetail.prompt_tokens) }}
          </div>
          <div v-if="parsedDetail.completion_tokens" class="node-detail-kv">
            Completion：{{ stringify(parsedDetail.completion_tokens) }}
          </div>
        </div>
      </div>

      <div v-if="asList(parsedDetail.segments).length" class="node-detail-block">
        <div class="node-detail-title">
          说话人分段
        </div>
        <div class="node-detail-meta">
          共 {{ stringify(parsedDetail.segments_count) }} 段
        </div>
        <ul class="node-detail-list">
          <li v-for="(segment, index) in asList(parsedDetail.segments)" :key="`segment-${index}`">
            {{ stringify(segment.speaker || segment.Speaker) }} · {{ stringify(segment.start_time ?? segment.StartTime) }} - {{ stringify(segment.end_time ?? segment.EndTime) }}
          </li>
        </ul>
      </div>

      <div v-if="fallbackEntries.length" class="node-detail-block">
        <div class="node-detail-title">
          其他信息
        </div>
        <div v-for="([key, value]) in fallbackEntries" :key="key" class="node-detail-kv">
          {{ key }}：{{ stringify(value) }}
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.node-detail-panel {
  display: grid;
  gap: 12px;
}

.node-detail-grid {
  display: grid;
  gap: 12px;
}

@media (min-width: 1024px) {
  .node-detail-grid {
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }
}

.node-detail-block {
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.88);
  padding: 12px;
}

.node-detail-title {
  margin-bottom: 8px;
  color: #16202c;
  font-size: 12px;
  font-weight: 600;
}

.node-detail-meta,
.node-detail-kv,
.node-detail-empty {
  color: #5d6b7c;
  font-size: 12px;
  line-height: 1.7;
}

.node-detail-list {
  margin: 0;
  padding-left: 16px;
  color: #16202c;
  font-size: 13px;
  line-height: 1.7;
}

.node-detail-pre {
  white-space: pre-wrap;
  word-break: break-word;
  color: #16202c;
  font-size: 13px;
  line-height: 1.7;
}

.node-detail-table {
  display: grid;
  gap: 6px;
}

.node-detail-table-row {
  display: grid;
  grid-template-columns: minmax(0, 1.2fr) minmax(0, 1.2fr) 72px;
  gap: 10px;
  align-items: start;
  font-size: 12px;
  color: #16202c;
}

.node-detail-table-head {
  color: #5d6b7c;
  font-weight: 600;
}

.node-detail-warning {
  border-radius: 12px;
  background: rgba(245, 158, 11, 0.12);
  color: #92400e;
  padding: 12px;
  font-size: 12px;
  line-height: 1.7;
}
</style>

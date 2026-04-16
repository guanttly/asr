<script setup lang="ts">
import MarkdownIt from 'markdown-it'
import { computed, ref, watch } from 'vue'

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
    'masked_words',
    'matched_words',
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
    'chunk_outputs',
    'active_chunk_index',
  ].includes(key))
})

const markdownRenderer = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})

const chunkOutputs = computed<Array<Record<string, any>>>(() => Array.isArray(parsedDetail.value?.chunk_outputs) ? parsedDetail.value.chunk_outputs as Array<Record<string, any>> : [])
const currentChunkTab = ref(0)
const renderedCurrentChunkHtml = computed(() => {
  const current = chunkOutputs.value[currentChunkTab.value]
  const output = typeof current?.output === 'string' ? current.output.trim() : ''
  if (!output)
    return ''
  return markdownRenderer.render(output)
})
const currentChunkPrompt = computed(() => {
  const current = chunkOutputs.value[currentChunkTab.value]
  return typeof current?.prompt === 'string' ? current.prompt.trim() : ''
})

watch(
  () => ({
    length: chunkOutputs.value.length,
    active: Number(parsedDetail.value?.active_chunk_index ?? 0),
  }),
  ({ length, active }) => {
    if (!length) {
      currentChunkTab.value = 0
      return
    }
    currentChunkTab.value = active > 0 ? Math.min(active - 1, length - 1) : length - 1
  },
  { immediate: true },
)

const detailLabelMap: Record<string, string> = {
  status: '状态',
  message: '进度',
  mode: '模式',
  duration: '时长',
  chunk_count: '分片数',
  input_runes: '输入字数',
  streamed: '流式输出',
  max_tokens: '最大输出 Token',
  allow_markdown: '允许 Markdown',
  normalized_markdown: '检测到 Markdown',
}

const detailValueMap: Record<string, Record<string, string>> = {
  status: {
    starting: '已开始',
    streaming: '处理中',
    completed: '已完成',
    success: '成功',
    failed: '失败',
  },
  mode: {
    audio_source: '音频源节点',
  },
}

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

function localizeFallbackKey(key: string) {
  return detailLabelMap[key] || key
}

function localizeFallbackValue(key: string, value: unknown) {
  if (typeof value === 'boolean')
    return value ? '是' : '否'
  if (typeof value === 'string') {
    const mapped = detailValueMap[key]?.[value]
    if (mapped)
      return mapped
  }
  return stringify(value)
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

      <div v-if="asList(parsedDetail.masked_words).length" class="node-detail-block">
        <div class="node-detail-title">
          敏感词命中
        </div>
        <div class="node-detail-meta">
          共使用 {{ stringify(parsedDetail.words_used) }} 个候选词
        </div>
        <ul class="node-detail-list">
          <li v-for="(item, index) in asList(parsedDetail.masked_words)" :key="`masked-${index}`">
            {{ stringify(item) }}
          </li>
        </ul>
      </div>

      <div v-if="asList(parsedDetail.matched_words).length" class="node-detail-block">
        <div class="node-detail-title">
          命中来源
        </div>
        <div class="node-detail-table node-detail-table--matched-words">
          <div class="node-detail-table-row node-detail-table-row--matched node-detail-table-head">
            <div>词</div>
            <div>来源</div>
            <div>命中</div>
          </div>
          <div v-for="(item, index) in asList(parsedDetail.matched_words)" :key="`matched-word-${index}`" class="node-detail-table-row node-detail-table-row--matched">
            <div>{{ stringify(item.word) }}</div>
            <div>{{ Array.isArray(item.sources) ? item.sources.join(' + ') : stringify(item.sources) }}</div>
            <div>{{ stringify(item.matches) }}</div>
          </div>
        </div>
      </div>

      <div v-if="asList(parsedDetail.applied_rules).length" class="node-detail-block">
        <div class="node-detail-title">
          命中规则
        </div>
        <div class="node-detail-table">
          <div class="node-detail-table-row node-detail-table-head">
            <div>匹配规则</div>
            <div>替换文本</div>
            <div>命中次数</div>
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
            输入：{{ stringify(parsedDetail.prompt_tokens) }}
          </div>
          <div v-if="parsedDetail.completion_tokens" class="node-detail-kv">
            输出：{{ stringify(parsedDetail.completion_tokens) }}
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

      <div v-if="chunkOutputs.length" class="node-detail-block">
        <div class="node-detail-title">
          分片输出
        </div>
        <div class="node-detail-tabs">
          <button
            v-for="(chunk, index) in chunkOutputs"
            :key="`${chunk.title || 'chunk'}-${index}`"
            type="button"
            class="node-detail-tab"
            :class="{ 'is-active': currentChunkTab === index }"
            @click="currentChunkTab = index"
          >
            {{ chunk.title || `片段 ${index + 1}` }}
          </button>
        </div>
        <div v-if="chunkOutputs[currentChunkTab]?.input_runes" class="node-detail-meta">
          当前片段输入字数：{{ stringify(chunkOutputs[currentChunkTab].input_runes) }}
        </div>
        <div v-if="currentChunkPrompt" class="node-detail-prompt-block">
          <div class="node-detail-subtitle">
            本片段提示词
          </div>
          <div class="node-detail-pre">
            {{ currentChunkPrompt }}
          </div>
        </div>
        <div v-if="renderedCurrentChunkHtml" class="node-detail-markdown" v-html="renderedCurrentChunkHtml" />
        <div v-else class="node-detail-empty">
          当前片段结果暂未返回。
        </div>
      </div>

      <div v-if="fallbackEntries.length" class="node-detail-block">
        <div class="node-detail-title">
          其他信息
        </div>
        <div v-for="([key, value]) in fallbackEntries" :key="key" class="node-detail-kv">
          {{ localizeFallbackKey(key) }}：{{ localizeFallbackValue(key, value) }}
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

.node-detail-tabs {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 10px;
}

.node-detail-tab {
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 999px;
  background: rgba(248, 250, 252, 0.95);
  color: #5d6b7c;
  cursor: pointer;
  font-size: 12px;
  font-weight: 600;
  line-height: 1;
  padding: 7px 12px;
  transition: all 0.18s ease;
}

.node-detail-tab.is-active {
  border-color: rgba(15, 118, 110, 0.28);
  background: rgba(240, 253, 250, 0.92);
  color: #0f766e;
}

.node-detail-title {
  margin-bottom: 8px;
  color: #16202c;
  font-size: 12px;
  font-weight: 600;
}

.node-detail-subtitle {
  margin-bottom: 8px;
  color: #475569;
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

.node-detail-table-row--matched {
  grid-template-columns: minmax(0, 0.8fr) minmax(0, 1.8fr) 72px;
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

.node-detail-prompt-block {
  margin-bottom: 12px;
  border-radius: 10px;
  background: rgba(248, 250, 252, 0.9);
  padding: 10px;
}

.node-detail-markdown {
  color: #16202c;
  font-size: 13px;
  line-height: 1.7;
}

.node-detail-markdown :deep(h1),
.node-detail-markdown :deep(h2),
.node-detail-markdown :deep(h3),
.node-detail-markdown :deep(h4) {
  margin: 0 0 10px;
  color: #16202c;
  font-weight: 700;
  line-height: 1.45;
}

.node-detail-markdown :deep(h1) {
  font-size: 20px;
}

.node-detail-markdown :deep(h2) {
  font-size: 17px;
}

.node-detail-markdown :deep(h3),
.node-detail-markdown :deep(h4) {
  font-size: 15px;
}

.node-detail-markdown :deep(p),
.node-detail-markdown :deep(ul),
.node-detail-markdown :deep(ol),
.node-detail-markdown :deep(blockquote),
.node-detail-markdown :deep(pre),
.node-detail-markdown :deep(table) {
  margin: 0 0 10px;
}

.node-detail-markdown :deep(ul),
.node-detail-markdown :deep(ol) {
  padding-left: 18px;
}

.node-detail-markdown :deep(li) {
  margin-bottom: 4px;
}

.node-detail-markdown :deep(blockquote) {
  border-left: 3px solid rgba(15, 118, 110, 0.22);
  margin-left: 0;
  padding-left: 12px;
  color: #4c5b6c;
}

.node-detail-markdown :deep(code) {
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.06);
  padding: 2px 5px;
  font-size: 12px;
}

.node-detail-markdown :deep(pre) {
  overflow-x: auto;
  border-radius: 10px;
  background: rgba(15, 23, 42, 0.92);
  padding: 12px;
  color: #e2e8f0;
}

.node-detail-markdown :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

.node-detail-markdown :deep(table) {
  width: 100%;
  border-collapse: collapse;
}

.node-detail-markdown :deep(th),
.node-detail-markdown :deep(td) {
  border: 1px solid rgba(148, 163, 184, 0.2);
  padding: 8px 10px;
  text-align: left;
  vertical-align: top;
}

.node-detail-markdown :deep(th) {
  background: rgba(248, 250, 252, 0.95);
}
</style>

<script setup lang="ts">
import MarkdownIt from 'markdown-it'
import { computed } from 'vue'

import { buildTextDiff } from '@/utils/textDiff'

const props = withDefaults(defineProps<{
  beforeText?: string
  afterText?: string
  beforeLabel?: string
  afterLabel?: string
  emptyLabel?: string
  mode?: 'diff' | 'plain' | 'markdown'
}>(), {
  beforeText: '',
  afterText: '',
  beforeLabel: '输入',
  afterLabel: '输出',
  emptyLabel: '暂无可展示文本',
  mode: 'diff',
})

const markdownRenderer = new MarkdownIt({
  html: false,
  linkify: true,
  breaks: true,
})
const diff = computed(() => buildTextDiff(props.beforeText || '', props.afterText || ''))
const showDiffMeta = computed(() => props.mode === 'diff')
const showMarkdown = computed(() => props.mode === 'markdown')
const renderedBeforeHtml = computed(() => props.beforeText ? markdownRenderer.render(props.beforeText) : '')
const renderedAfterHtml = computed(() => props.afterText ? markdownRenderer.render(props.afterText) : '')
</script>

<template>
  <div class="diff-preview">
    <div v-if="showDiffMeta" class="diff-meta">
      <span>{{ diff.changed ? '已检测到差异' : '输入输出一致' }}</span>
      <span v-if="diff.changed">+{{ diff.addedCount }} / -{{ diff.removedCount }}</span>
    </div>

    <div class="diff-grid">
      <div class="diff-pane">
        <div class="diff-title">
          {{ beforeLabel }}
        </div>
        <div v-if="!beforeText" class="diff-empty">
          {{ emptyLabel }}
        </div>
        <div v-else-if="showMarkdown" class="diff-markdown" v-html="renderedBeforeHtml" />
        <div v-else-if="showDiffMeta" class="diff-body">
          <span
            v-for="(segment, index) in diff.beforeSegments"
            :key="`${beforeLabel}-${index}`"
            class="diff-segment"
            :class="`diff-segment--${segment.kind}`"
          >{{ segment.text }}</span>
        </div>
        <div v-else class="diff-body">
          {{ beforeText }}
        </div>
      </div>

      <div class="diff-pane">
        <div class="diff-title">
          {{ afterLabel }}
        </div>
        <div v-if="!afterText" class="diff-empty">
          {{ emptyLabel }}
        </div>
        <div v-else-if="showMarkdown" class="diff-markdown" v-html="renderedAfterHtml" />
        <div v-else-if="showDiffMeta" class="diff-body">
          <span
            v-for="(segment, index) in diff.afterSegments"
            :key="`${afterLabel}-${index}`"
            class="diff-segment"
            :class="`diff-segment--${segment.kind}`"
          >{{ segment.text }}</span>
        </div>
        <div v-else class="diff-body">
          {{ afterText }}
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.diff-preview {
  display: grid;
  gap: 12px;
}

.diff-meta {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  color: #5d6b7c;
  font-size: 12px;
}

.diff-grid {
  display: grid;
  gap: 12px;
}

@media (min-width: 1024px) {
  .diff-grid {
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
  }
}

.diff-pane {
  border-radius: 12px;
  background: rgba(255, 255, 255, 0.88);
  padding: 12px;
}

.diff-title {
  color: #5d6b7c;
  font-size: 12px;
  margin-bottom: 8px;
}

.diff-body,
.diff-empty {
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 13px;
  line-height: 1.65;
  color: #16202c;
}

.diff-empty {
  color: #748395;
}

.diff-markdown {
  color: #16202c;
  font-size: 13px;
  line-height: 1.7;
}

.diff-markdown :deep(h1),
.diff-markdown :deep(h2),
.diff-markdown :deep(h3),
.diff-markdown :deep(h4) {
  margin: 0 0 10px;
  color: #16202c;
  font-weight: 700;
  line-height: 1.45;
}

.diff-markdown :deep(h1) {
  font-size: 20px;
}

.diff-markdown :deep(h2) {
  font-size: 17px;
}

.diff-markdown :deep(h3),
.diff-markdown :deep(h4) {
  font-size: 15px;
}

.diff-markdown :deep(p),
.diff-markdown :deep(ul),
.diff-markdown :deep(ol),
.diff-markdown :deep(blockquote),
.diff-markdown :deep(pre),
.diff-markdown :deep(table) {
  margin: 0 0 10px;
}

.diff-markdown :deep(ul),
.diff-markdown :deep(ol) {
  padding-left: 18px;
}

.diff-markdown :deep(li) {
  margin-bottom: 4px;
}

.diff-markdown :deep(blockquote) {
  border-left: 3px solid rgba(15, 118, 110, 0.22);
  margin-left: 0;
  padding-left: 12px;
  color: #4c5b6c;
}

.diff-markdown :deep(code) {
  border-radius: 6px;
  background: rgba(15, 23, 42, 0.06);
  padding: 2px 5px;
  font-size: 12px;
}

.diff-markdown :deep(pre) {
  overflow-x: auto;
  border-radius: 10px;
  background: rgba(15, 23, 42, 0.92);
  padding: 12px;
  color: #e2e8f0;
}

.diff-markdown :deep(pre code) {
  background: transparent;
  padding: 0;
  color: inherit;
}

.diff-markdown :deep(table) {
  width: 100%;
  border-collapse: collapse;
}

.diff-markdown :deep(th),
.diff-markdown :deep(td) {
  border: 1px solid rgba(148, 163, 184, 0.2);
  padding: 8px 10px;
  text-align: left;
  vertical-align: top;
}

.diff-markdown :deep(th) {
  background: rgba(248, 250, 252, 0.95);
}

.diff-markdown :deep(hr) {
  border: 0;
  border-top: 1px solid rgba(148, 163, 184, 0.2);
  margin: 14px 0;
}

.diff-segment--added {
  background: rgba(16, 185, 129, 0.18);
  color: #065f46;
}

.diff-segment--removed {
  background: rgba(239, 68, 68, 0.14);
  color: #b91c1c;
  text-decoration: line-through;
}
</style>

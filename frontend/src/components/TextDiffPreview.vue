<script setup lang="ts">
import { computed } from 'vue'

import { buildTextDiff } from '@/utils/textDiff'

const props = withDefaults(defineProps<{
  beforeText?: string
  afterText?: string
  beforeLabel?: string
  afterLabel?: string
  emptyLabel?: string
}>(), {
  beforeText: '',
  afterText: '',
  beforeLabel: '输入',
  afterLabel: '输出',
  emptyLabel: '暂无可展示文本',
})

const diff = computed(() => buildTextDiff(props.beforeText || '', props.afterText || ''))
</script>

<template>
  <div class="diff-preview">
    <div class="diff-meta">
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
        <div v-else class="diff-body">
          <span
            v-for="(segment, index) in diff.beforeSegments"
            :key="`${beforeLabel}-${index}`"
            class="diff-segment"
            :class="`diff-segment--${segment.kind}`"
          >{{ segment.text }}</span>
        </div>
      </div>

      <div class="diff-pane">
        <div class="diff-title">
          {{ afterLabel }}
        </div>
        <div v-if="!afterText" class="diff-empty">
          {{ emptyLabel }}
        </div>
        <div v-else class="diff-body">
          <span
            v-for="(segment, index) in diff.afterSegments"
            :key="`${afterLabel}-${index}`"
            class="diff-segment"
            :class="`diff-segment--${segment.kind}`"
          >{{ segment.text }}</span>
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

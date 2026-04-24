<script setup lang="ts">
import { onBeforeUnmount, watch } from 'vue'
import { useConfirm } from '@/composables/useConfirm'

const { state, resolveConfirm } = useConfirm()

function handleKey(event: KeyboardEvent) {
  if (!state.visible)
    return
  if (event.key === 'Escape') {
    event.preventDefault()
    resolveConfirm(false)
  }
  else if (event.key === 'Enter') {
    event.preventDefault()
    resolveConfirm(true)
  }
}

watch(() => state.visible, (visible) => {
  if (visible)
    window.addEventListener('keydown', handleKey)
  else
    window.removeEventListener('keydown', handleKey)
})

onBeforeUnmount(() => {
  window.removeEventListener('keydown', handleKey)
})
</script>

<template>
  <Transition name="confirm-fade">
    <div v-if="state.visible" class="confirm-mask" @click.self="resolveConfirm(false)">
      <div class="confirm-dialog" :class="`tone-${state.tone}`">
        <header class="confirm-head">
          <span class="confirm-title">{{ state.title }}</span>
        </header>
        <div class="confirm-body">
          <p class="confirm-message">{{ state.message }}</p>
        </div>
        <footer class="confirm-foot">
          <button class="confirm-btn ghost" @click="resolveConfirm(false)">{{ state.cancelText }}</button>
          <button class="confirm-btn primary" :class="`tone-${state.tone}`" @click="resolveConfirm(true)">
            {{ state.confirmText }}
          </button>
        </footer>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.confirm-mask {
  position: fixed;
  inset: 0;
  z-index: 9999;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(6px);
}

.confirm-dialog {
  width: min(320px, calc(100vw - 32px));
  border-radius: 18px;
  background: #ffffff;
  box-shadow: 0 24px 48px rgba(15, 23, 42, 0.18);
  overflow: hidden;
  transform: translateY(0);
  animation: confirm-pop 0.18s ease;
}

@keyframes confirm-pop {
  from { transform: translateY(8px) scale(0.98); opacity: 0; }
  to { transform: translateY(0) scale(1); opacity: 1; }
}

.confirm-head {
  padding: 16px 18px 6px;
}

.confirm-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}

.confirm-body {
  padding: 4px 18px 16px;
}

.confirm-message {
  font-size: 13px;
  line-height: 1.7;
  color: #475569;
  white-space: pre-line;
}

.confirm-foot {
  display: flex;
  gap: 8px;
  padding: 10px 14px 14px;
  justify-content: flex-end;
}

.confirm-btn {
  border: 0;
  border-radius: 999px;
  padding: 7px 18px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: transform 0.12s ease, box-shadow 0.12s ease, background 0.18s ease, color 0.18s ease;
}

.confirm-btn:active {
  transform: scale(0.97);
}

.confirm-btn.ghost {
  background: rgba(148, 163, 184, 0.16);
  color: #334155;
}

.confirm-btn.ghost:hover {
  background: rgba(148, 163, 184, 0.26);
}

.confirm-btn.primary {
  color: #ffffff;
  background: #0f766e;
}

.confirm-btn.primary:hover {
  background: #0d5f59;
}

.confirm-btn.primary.tone-danger {
  background: linear-gradient(135deg, #ef4444, #dc2626);
}

.confirm-btn.primary.tone-danger:hover {
  filter: brightness(0.96);
}

.confirm-btn.primary.tone-default {
  background: #334155;
}

.confirm-fade-enter-active,
.confirm-fade-leave-active {
  transition: opacity 0.18s ease;
}

.confirm-fade-enter-from,
.confirm-fade-leave-to {
  opacity: 0;
}
</style>

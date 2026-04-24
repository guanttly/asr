<script setup lang="ts">
import { computed, nextTick, ref, watch } from 'vue'
import {
  createSensitiveDict,
  createSensitiveEntry,
  createTermDict,
  createTermEntry,
  listSensitiveDicts,
  listTermDicts,
  type SensitiveDict,
  type TermDict,
} from '@/utils/dictionaries'

type DictKind = 'term' | 'sensitive'

const props = defineProps<{
  visible: boolean
  kind: DictKind
  defaultText: string
}>()

const emit = defineEmits<{
  (e: 'close'): void
  (e: 'success', payload: { kind: DictKind, dictName: string, value: string }): void
}>()

const dicts = ref<Array<TermDict | SensitiveDict>>([])
const dictsLoading = ref(false)
const selectedDictId = ref<number | null>(null)
const showCreateForm = ref(false)
const submitting = ref(false)
const errorText = ref('')

const newDictName = ref('')
const newDictTag = ref('')

const correctTerm = ref('')
const wrongVariantsText = ref('')
const pinyin = ref('')

const sensitiveWord = ref('')

const kindLabel = computed(() => props.kind === 'term' ? '术语词库' : '敏感词库')
const tagLabel = computed(() => props.kind === 'term' ? '领域' : '场景')
const tagPlaceholder = computed(() => props.kind === 'term' ? '例如：医疗 / 法律 / 通用' : '例如：通用 / 客服 / 内部')

async function loadDicts() {
  dictsLoading.value = true
  errorText.value = ''
  try {
    if (props.kind === 'term') {
      const result = await listTermDicts()
      dicts.value = result.items
    }
    else {
      const result = await listSensitiveDicts()
      dicts.value = result.items
    }
    if (dicts.value.length > 0 && selectedDictId.value == null)
      selectedDictId.value = dicts.value[0].id
    if (dicts.value.length === 0)
      showCreateForm.value = true
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '加载词库失败'
  }
  finally {
    dictsLoading.value = false
  }
}

function resetForm(text: string) {
  errorText.value = ''
  showCreateForm.value = false
  selectedDictId.value = dicts.value[0]?.id ?? null
  newDictName.value = ''
  newDictTag.value = ''
  if (props.kind === 'term') {
    correctTerm.value = text.trim()
    wrongVariantsText.value = ''
    pinyin.value = ''
  }
  else {
    sensitiveWord.value = text.trim()
  }
}

watch(() => props.visible, async (visible) => {
  if (!visible)
    return
  resetForm(props.defaultText)
  await loadDicts()
  await nextTick()
})

async function ensureDict(): Promise<{ id: number, name: string } | null> {
  if (showCreateForm.value || dicts.value.length === 0) {
    const name = newDictName.value.trim()
    const tag = newDictTag.value.trim()
    if (!name) {
      errorText.value = `请输入${kindLabel.value}名称`
      return null
    }
    if (!tag) {
      errorText.value = `请输入${tagLabel.value}`
      return null
    }
    if (props.kind === 'term') {
      const created = await createTermDict({ name, domain: tag })
      dicts.value.push(created)
      selectedDictId.value = created.id
      return { id: created.id, name: created.name }
    }
    else {
      const created = await createSensitiveDict({ name, scene: tag, is_base: false })
      dicts.value.push(created)
      selectedDictId.value = created.id
      return { id: created.id, name: created.name }
    }
  }
  const target = dicts.value.find(item => item.id === selectedDictId.value)
  if (!target) {
    errorText.value = '请选择词库'
    return null
  }
  return { id: target.id, name: target.name }
}

async function handleSubmit() {
  if (submitting.value)
    return
  errorText.value = ''
  submitting.value = true
  try {
    const dict = await ensureDict()
    if (!dict)
      return

    if (props.kind === 'term') {
      const correct = correctTerm.value.trim()
      if (!correct) {
        errorText.value = '请输入正确术语'
        return
      }
      const variants = wrongVariantsText.value
        .split(/[,，;；\n]/)
        .map(item => item.trim())
        .filter(Boolean)
      await createTermEntry(dict.id, {
        correct_term: correct,
        wrong_variants: variants,
        pinyin: pinyin.value.trim() || undefined,
      })
      emit('success', { kind: 'term', dictName: dict.name, value: correct })
    }
    else {
      const word = sensitiveWord.value.trim()
      if (!word) {
        errorText.value = '请输入敏感词'
        return
      }
      await createSensitiveEntry(dict.id, { word, enabled: true })
      emit('success', { kind: 'sensitive', dictName: dict.name, value: word })
    }
    emit('close')
  }
  catch (error) {
    errorText.value = error instanceof Error ? error.message : '提交失败'
  }
  finally {
    submitting.value = false
  }
}

function handleCancel() {
  if (submitting.value)
    return
  emit('close')
}
</script>

<template>
  <Transition name="dict-fade">
    <div v-if="visible" class="dict-mask" @click.self="handleCancel">
      <div class="dict-dialog">
        <header class="dict-head">
          <span class="dict-title">收录到{{ kindLabel }}</span>
          <button class="dict-close" type="button" @click="handleCancel">×</button>
        </header>

        <div class="dict-body">
          <div v-if="errorText" class="dict-alert">{{ errorText }}</div>

          <div class="dict-section">
            <div class="dict-section-head">
              <span class="dict-section-title">选择词库</span>
              <button
                v-if="!showCreateForm && dicts.length > 0"
                class="dict-link"
                type="button"
                @click="showCreateForm = true"
              >
                + 新建词库
              </button>
              <button
                v-else-if="showCreateForm && dicts.length > 0"
                class="dict-link"
                type="button"
                @click="showCreateForm = false"
              >
                选择已有
              </button>
            </div>

            <div v-if="dictsLoading" class="dict-loading">正在加载词库列表...</div>

            <div v-else-if="!showCreateForm" class="dict-pill-list">
              <button
                v-for="item in dicts"
                :key="item.id"
                type="button"
                class="dict-pill"
                :class="{ active: selectedDictId === item.id }"
                @click="selectedDictId = item.id"
              >
                <span class="dict-pill-name">{{ item.name }}</span>
                <span class="dict-pill-tag">{{ kind === 'term' ? (item as TermDict).domain : (item as SensitiveDict).scene }}</span>
              </button>
            </div>

            <div v-if="showCreateForm || dicts.length === 0" class="dict-create">
              <label class="dict-field">
                <span>词库名称</span>
                <input v-model="newDictName" type="text" placeholder="例如：科室通用术语">
              </label>
              <label class="dict-field">
                <span>{{ tagLabel }}</span>
                <input v-model="newDictTag" type="text" :placeholder="tagPlaceholder">
              </label>
            </div>
          </div>

          <div class="dict-section">
            <div class="dict-section-head">
              <span class="dict-section-title">{{ kind === 'term' ? '词条内容' : '敏感词内容' }}</span>
            </div>
            <template v-if="kind === 'term'">
              <label class="dict-field">
                <span>正确术语</span>
                <input v-model="correctTerm" type="text" placeholder="规范写法">
              </label>
              <label class="dict-field">
                <span>易错变体（可多个，用 , 或换行分隔）</span>
                <textarea v-model="wrongVariantsText" rows="2" placeholder="模型常错写法，例如：左室肥后\n左心室肥后" />
              </label>
              <label class="dict-field">
                <span>拼音（可选）</span>
                <input v-model="pinyin" type="text" placeholder="例如：zuǒ xīn shì féi hòu">
              </label>
            </template>
            <template v-else>
              <label class="dict-field">
                <span>敏感词</span>
                <input v-model="sensitiveWord" type="text" placeholder="需要被屏蔽或高亮的词汇">
              </label>
            </template>
          </div>
        </div>

        <footer class="dict-foot">
          <button class="dict-btn ghost" type="button" :disabled="submitting" @click="handleCancel">取消</button>
          <button class="dict-btn primary" type="button" :disabled="submitting" @click="handleSubmit">
            {{ submitting ? '提交中...' : '提交收录' }}
          </button>
        </footer>
      </div>
    </div>
  </Transition>
</template>

<style scoped>
.dict-mask {
  position: fixed;
  inset: 0;
  z-index: 9000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: rgba(15, 23, 42, 0.42);
  backdrop-filter: blur(6px);
}

.dict-dialog {
  width: min(360px, calc(100vw - 32px));
  max-height: calc(100vh - 32px);
  border-radius: 18px;
  background: #ffffff;
  box-shadow: 0 24px 48px rgba(15, 23, 42, 0.2);
  display: flex;
  flex-direction: column;
  overflow: hidden;
  animation: dict-pop 0.2s ease;
}

@keyframes dict-pop {
  from { transform: translateY(8px) scale(0.98); opacity: 0; }
  to { transform: translateY(0) scale(1); opacity: 1; }
}

.dict-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 14px 18px;
  border-bottom: 1px solid rgba(226, 232, 240, 0.7);
}

.dict-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
}

.dict-close {
  border: 0;
  background: transparent;
  cursor: pointer;
  font-size: 18px;
  color: #94a3b8;
  line-height: 1;
}

.dict-close:hover {
  color: #475569;
}

.dict-body {
  padding: 14px 18px;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 14px;
}

.dict-alert {
  padding: 8px 12px;
  font-size: 12px;
  color: #b91c1c;
  background: rgba(254, 242, 242, 0.92);
  border-radius: 10px;
}

.dict-section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 8px;
}

.dict-section-title {
  font-size: 12px;
  font-weight: 600;
  color: #475569;
}

.dict-link {
  border: 0;
  background: transparent;
  color: #0f766e;
  font-size: 12px;
  cursor: pointer;
}

.dict-link:hover {
  text-decoration: underline;
}

.dict-loading {
  font-size: 12px;
  color: #94a3b8;
  padding: 6px 0;
}

.dict-pill-list {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.dict-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 6px 12px;
  border-radius: 999px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  background: rgba(248, 250, 252, 0.9);
  color: #334155;
  font-size: 12px;
  cursor: pointer;
  transition: all 0.18s ease;
}

.dict-pill:hover {
  border-color: rgba(15, 118, 110, 0.4);
}

.dict-pill.active {
  background: rgba(15, 118, 110, 0.12);
  border-color: rgba(15, 118, 110, 0.6);
  color: #0f766e;
}

.dict-pill-tag {
  font-size: 11px;
  color: #94a3b8;
}

.dict-pill.active .dict-pill-tag {
  color: rgba(15, 118, 110, 0.7);
}

.dict-create {
  display: flex;
  flex-direction: column;
  gap: 8px;
  padding: 10px;
  border-radius: 12px;
  background: rgba(241, 245, 249, 0.6);
  border: 1px dashed rgba(148, 163, 184, 0.36);
  margin-top: 8px;
}

.dict-field {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 12px;
  color: #475569;
}

.dict-field input,
.dict-field textarea {
  border-radius: 8px;
  border: 1px solid rgba(148, 163, 184, 0.32);
  background: #ffffff;
  padding: 7px 10px;
  font-size: 12px;
  color: #0f172a;
  outline: none;
  resize: none;
  transition: border-color 0.18s ease;
}

.dict-field input:focus,
.dict-field textarea:focus {
  border-color: #0f766e;
}

.dict-foot {
  display: flex;
  justify-content: flex-end;
  gap: 8px;
  padding: 12px 18px;
  border-top: 1px solid rgba(226, 232, 240, 0.7);
}

.dict-btn {
  border: 0;
  border-radius: 999px;
  padding: 7px 18px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  transition: background 0.18s ease;
}

.dict-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.dict-btn.ghost {
  background: rgba(148, 163, 184, 0.16);
  color: #334155;
}

.dict-btn.ghost:hover:not(:disabled) {
  background: rgba(148, 163, 184, 0.26);
}

.dict-btn.primary {
  background: #0f766e;
  color: #ffffff;
}

.dict-btn.primary:hover:not(:disabled) {
  background: #0d5f59;
}

.dict-fade-enter-active,
.dict-fade-leave-active {
  transition: opacity 0.18s ease;
}

.dict-fade-enter-from,
.dict-fade-leave-to {
  opacity: 0;
}
</style>

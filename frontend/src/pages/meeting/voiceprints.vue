<script setup lang="ts">
import type { AxiosError } from 'axios'
import type { VoiceprintItem } from '@/api/voiceprint'

import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onBeforeUnmount, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { createVoiceprint, deleteVoiceprint, getVoiceprints } from '@/api/voiceprint'
import { useAudioRecorder } from '@/composables/useAudioRecorder'
import { useDeleteConfirmDialog } from '@/composables/useDeleteConfirmDialog'

const router = useRouter()
const message = useMessage()
const confirmDelete = useDeleteConfirmDialog()

const loading = ref(false)
const saving = ref(false)
const deletingID = ref<string | null>(null)
const serviceURL = ref('')
const voiceprints = ref<VoiceprintItem[]>([])
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFile = ref<File | null>(null)
const previewAudioURL = ref('')
const audioSourceMode = ref<'upload' | 'recorded' | ''>('')
const audioInputMode = ref<'upload' | 'record'>('upload')
const selectedScriptID = ref('difficult-case-reading')
const recordingElapsedSeconds = ref(0)
const speakerName = ref('')
const department = ref('')
const notes = ref('')

const { isRecording, start: startRecording, stop: stopRecording } = useAudioRecorder()

const recordingScriptOptions = [
  {
    id: 'difficult-case-reading',
    title: '疑难病例读片场景',
    tip: '适合住院总、影像科或多学科会诊时围绕影像表现进行连续分析。',
    content: '这位患者胸部 CT 显示双肺多发斑片及结节样高密度影，以右下肺外带更为明显，部分病灶边界欠清，可见支气管充气征。纵隔内可见数枚稍大淋巴结，右侧少量胸腔积液。结合患者持续发热、炎症指标升高及近期免疫治疗史，需要重点鉴别感染性病变、免疫相关性肺炎以及肿瘤进展，建议尽快结合增强扫描、病原学检查和临床治疗反应综合判断。',
  },
  {
    id: 'handover',
    title: '医学交接班场景',
    tip: '适合护士交接班、值班交接等连续陈述场景。',
    content: '各位老师早上好，下面进行今日交接班。昨晚新收入院两位患者，其中三床因发热伴咳嗽收入呼吸内科，已完善血常规、胸部 CT 和痰培养。八床夜间血压波动较大，已遵医嘱调整降压方案，目前生命体征平稳。请白班继续关注尿量、体温变化以及复查结果。',
  },
  {
    id: 'ward-round',
    title: '查房讨论场景',
    tip: '适合主任查房、病例讨论或 MDT 多学科会诊中的个人发言录入。',
    content: '关于十二床患者，目前主要问题是感染指标下降不明显，但氧合较前有所改善。昨天复查 C 反应蛋白和降钙素原仍偏高，建议继续现有抗感染方案，同时尽快完善痰培养药敏结果。若今晚体温仍反复，明早查房时需要重新评估是否存在新的感染灶，并讨论是否调整抗菌药物覆盖范围。',
  },
] as const

const selectedScript = computed(() => recordingScriptOptions.find(item => item.id === selectedScriptID.value) || recordingScriptOptions[0])
const selectedAudioLabel = computed(() => {
  if (!selectedFile.value)
    return ''
  return audioSourceMode.value === 'recorded' ? '当前样本来自录音' : '当前样本来自本地上传'
})
const audioInputModeMeta = computed(() => {
  if (audioInputMode.value === 'record') {
    return {
      title: '直接录音',
      description: isRecording.value
        ? '录音进行中，建议连续朗读 15 到 30 秒后停止并回放确认。'
        : '适合现场直接采集单人近讲语音，录完可立即回放并作为注册样本。',
      actionLabel: isRecording.value ? '停止录音' : '开始录音',
    }
  }
  return {
    title: '选择注册音频',
    description: '上传已有干净语音样本，适合已经准备好单人录音文件的场景。',
    actionLabel: '选择注册音频',
  }
})

const voiceprintCount = computed(() => voiceprints.value.length)

const TARGET_SAMPLE_RATE = 16000
let recordingChunks: ArrayBuffer[] = []
let previewObjectURL: string | null = null
let recordingTimer: number | null = null

function formatDateTime(value?: string) {
  if (!value)
    return '-'

  const date = new Date(value)
  if (Number.isNaN(date.getTime()))
    return value

  return date.toLocaleString('zh-CN', { hour12: false })
}

function formatDuration(value?: number) {
  if (!value || value <= 0)
    return '-'

  const totalSeconds = Math.max(0, Math.round(value))
  const minutes = Math.floor(totalSeconds / 60)
  const seconds = totalSeconds % 60

  if (minutes > 0)
    return `${minutes}分${seconds}秒`
  return `${seconds}秒`
}

function extractErrorMessage(error: unknown, fallback: string) {
  const responseMessage = (error as AxiosError<{ message?: string }>)?.response?.data?.message
  if (typeof responseMessage === 'string' && responseMessage.trim())
    return responseMessage
  return fallback
}

function writeASCII(view: DataView, offset: number, value: string) {
  for (let i = 0; i < value.length; i++)
    view.setUint8(offset + i, value.charCodeAt(i))
}

function createWavFileFromChunks(chunks: ArrayBuffer[], fileName: string) {
  const totalBytes = chunks.reduce((sum, chunk) => sum + chunk.byteLength, 0)
  const wavBuffer = new ArrayBuffer(44 + totalBytes)
  const view = new DataView(wavBuffer)
  const pcmPayload = new Uint8Array(wavBuffer, 44)

  writeASCII(view, 0, 'RIFF')
  view.setUint32(4, 36 + totalBytes, true)
  writeASCII(view, 8, 'WAVE')
  writeASCII(view, 12, 'fmt ')
  view.setUint32(16, 16, true)
  view.setUint16(20, 1, true)
  view.setUint16(22, 1, true)
  view.setUint32(24, TARGET_SAMPLE_RATE, true)
  view.setUint32(28, TARGET_SAMPLE_RATE * 2, true)
  view.setUint16(32, 2, true)
  view.setUint16(34, 16, true)
  writeASCII(view, 36, 'data')
  view.setUint32(40, totalBytes, true)

  let offset = 0
  for (const chunk of chunks) {
    pcmPayload.set(new Uint8Array(chunk), offset)
    offset += chunk.byteLength
  }

  return new File([wavBuffer], fileName, { type: 'audio/wav' })
}

function revokePreviewAudioURL() {
  if (!previewObjectURL)
    return
  URL.revokeObjectURL(previewObjectURL)
  previewObjectURL = null
}

function syncPreviewAudio(file: File | null) {
  revokePreviewAudioURL()
  if (!file) {
    previewAudioURL.value = ''
    return
  }
  previewObjectURL = URL.createObjectURL(file)
  previewAudioURL.value = previewObjectURL
}

function applySelectedAudio(file: File | null, mode: 'upload' | 'recorded' | '') {
  selectedFile.value = file
  audioSourceMode.value = file ? mode : ''
  syncPreviewAudio(file)
}

function stopRecordingClock() {
  if (recordingTimer != null) {
    window.clearInterval(recordingTimer)
    recordingTimer = null
  }
}

function startRecordingClock() {
  stopRecordingClock()
  recordingElapsedSeconds.value = 0
  recordingTimer = window.setInterval(() => {
    recordingElapsedSeconds.value += 1
  }, 1000)
}

function formatRecordingElapsed(value: number) {
  const minutes = Math.floor(value / 60)
  const seconds = value % 60
  return `${minutes.toString().padStart(2, '0')}:${seconds.toString().padStart(2, '0')}`
}

function selectAudioInputMode(mode: 'upload' | 'record') {
  audioInputMode.value = mode
}

function openFilePicker() {
  if (isRecording.value) {
    message.warning('录音进行中，请先停止录音')
    return
  }
  fileInput.value?.click()
}

function clearForm() {
  speakerName.value = ''
  department.value = ''
  notes.value = ''
  if (isRecording.value)
    stopRecording()
  stopRecordingClock()
  recordingChunks = []
  recordingElapsedSeconds.value = 0
  applySelectedAudio(null, '')
  if (fileInput.value)
    fileInput.value.value = ''
}

function handleFileSelected(event: Event) {
  const target = event.target as HTMLInputElement | null
  const file = target?.files?.[0] || null
  applySelectedAudio(file, file ? 'upload' : '')
  if (target)
    target.value = ''
}

async function handleStartRecording() {
  if (isRecording.value)
    return

  recordingChunks = []
  recordingElapsedSeconds.value = 0
  applySelectedAudio(null, '')

  try {
    await startRecording((chunk) => {
      recordingChunks.push(chunk.slice(0))
    })
    startRecordingClock()
  }
  catch (error) {
    message.error(error instanceof Error ? error.message : '开始录音失败')
  }
}

function handleStopRecording() {
  if (!isRecording.value)
    return

  stopRecording()
  stopRecordingClock()

  if (recordingChunks.length === 0) {
    message.warning('未采集到有效录音，请重试')
    return
  }

  const recordedFile = createWavFileFromChunks(recordingChunks, `voiceprint-${Date.now()}.wav`)
  applySelectedAudio(recordedFile, 'recorded')
  message.success('录音已生成，可先回放确认后再提交注册')
}

function handleClearAudio() {
  if (isRecording.value)
    stopRecording()
  stopRecordingClock()
  recordingChunks = []
  recordingElapsedSeconds.value = 0
  applySelectedAudio(null, '')
  if (fileInput.value)
    fileInput.value.value = ''
}

async function handleCopyScript() {
  try {
    await navigator.clipboard.writeText(selectedScript.value.content)
    message.success('示例文案已复制')
  }
  catch {
    message.warning('当前浏览器无法自动复制，请手动选择文案')
  }
}

async function loadVoiceprints(options?: { silent?: boolean }) {
  loading.value = true
  try {
    const result = await getVoiceprints()
    voiceprints.value = result.data.items
    serviceURL.value = typeof result.data.service_url === 'string' ? result.data.service_url : ''
  }
  catch (error) {
    if (!options?.silent)
      message.error(extractErrorMessage(error, '声纹库加载失败'))
  }
  finally {
    loading.value = false
  }
}

async function handleSubmit() {
  if (!speakerName.value.trim()) {
    message.warning('请填写说话人姓名')
    return
  }
  if (!selectedFile.value) {
    message.warning('请上传注册音频')
    return
  }

  const payload = new FormData()
  payload.append('file', selectedFile.value)
  payload.append('speaker_name', speakerName.value.trim())
  if (department.value.trim())
    payload.append('department', department.value.trim())
  if (notes.value.trim())
    payload.append('notes', notes.value.trim())

  saving.value = true
  try {
    const result = await createVoiceprint(payload)
    serviceURL.value = typeof result.data.service_url === 'string' ? result.data.service_url : serviceURL.value
    message.success('声纹注册成功')
    clearForm()
    await loadVoiceprints({ silent: true })
  }
  catch (error) {
    message.error(extractErrorMessage(error, '声纹注册失败'))
  }
  finally {
    saving.value = false
  }
}

async function handleDelete(row: VoiceprintItem) {
  if (deletingID.value)
    return

  const confirmed = await confirmDelete({
    entityType: '声纹记录',
    entityName: row.speaker_name,
    description: '删除后，说话人分离节点将无法再把该说话人自动映射为实名。',
  })
  if (!confirmed)
    return

  deletingID.value = row.id
  try {
    await deleteVoiceprint(row.id)
    message.success('声纹记录已删除')
    voiceprints.value = voiceprints.value.filter(item => item.id !== row.id)
  }
  catch (error) {
    message.error(extractErrorMessage(error, '声纹记录删除失败'))
  }
  finally {
    deletingID.value = null
  }
}

const columns = [
  {
    title: '说话人',
    key: 'speaker_name',
    minWidth: 160,
    render: (row: VoiceprintItem) => h('div', { class: 'text-sm font-600 text-ink' }, row.speaker_name || '-'),
  },
  {
    title: '部门',
    key: 'department',
    width: 120,
    render: (row: VoiceprintItem) => row.department
      ? h(NTag, { bordered: false, round: true, type: 'success', size: 'small' }, { default: () => row.department })
      : '-',
  },
  {
    title: '样本时长',
    key: 'audio_duration',
    width: 110,
    render: (row: VoiceprintItem) => formatDuration(row.audio_duration),
  },
  {
    title: '备注',
    key: 'notes',
    minWidth: 200,
    render: (row: VoiceprintItem) => h('div', {
      class: 'max-w-72 truncate text-xs leading-6 text-slate',
      title: row.notes || undefined,
    }, row.notes || '-'),
  },
  {
    title: '更新时间',
    key: 'updated_at',
    width: 170,
    render: (row: VoiceprintItem) => formatDateTime(row.updated_at || row.created_at),
  },
  {
    title: '操作',
    key: 'actions',
    width: 100,
    render: (row: VoiceprintItem) => h(NButton, {
      text: true,
      type: 'error',
      size: 'small',
      loading: deletingID.value === row.id,
      onClick: () => handleDelete(row),
    }, { default: () => '删除' }),
  },
]

onMounted(() => {
  void loadVoiceprints()
})

onBeforeUnmount(() => {
  if (isRecording.value)
    stopRecording()
  stopRecordingClock()
  revokePreviewAudioURL()
})
</script>

<template>
  <div class="flex h-full min-h-0 min-w-0 flex-col gap-5 overflow-x-hidden overflow-y-auto pr-1">
    <NCard class="card-main">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <div class="text-sm font-700 text-ink">
            声纹库
          </div>
          <div class="mt-1 text-sm leading-6 text-slate">
            这里维护会议纪要中用于实名匹配的说话人声纹样本。注册完成后，在工作流里打开“启用声纹匹配”即可把匿名说话人标签替换成已登记姓名。
          </div>
        </div>
        <div class="grid grid-cols-2 gap-3 sm:flex sm:items-center">
          <div class="subtle-panel m-0 min-w-28">
            <div class="text-xs text-slate/70">
              已注册声纹
            </div>
            <div class="mt-1.5 text-lg font-700 text-ink">
              {{ voiceprintCount }}
            </div>
          </div>
          <NButton quaternary @click="router.push('/workflows')">
            管理工作流
          </NButton>
        </div>
      </div>

      <div class="mt-4 rounded-2 border border-emerald-200 bg-emerald-50/80 px-4 py-3 text-xs leading-6 text-emerald-800">
        <div class="text-sm font-700 text-emerald-900">
          当前声纹分析服务
        </div>
        <div class="mt-1 break-all">
          {{ serviceURL || '首次成功连通后会显示当前服务地址。若一直为空，请检查后端 services.speaker_analysis_url 配置。' }}
        </div>
      </div>
    </NCard>

    <div class="grid items-start gap-5 xl:grid-cols-[1.6fr,1.1fr]">
      <NCard class="card-main">
        <template #header>
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm font-600">已注册说话人</span>
            <span class="text-xs text-slate">说话人分离节点开启声纹匹配后会直接复用这里的声纹库</span>
          </div>
        </template>

        <div v-if="voiceprints.length > 0 || loading">
          <NDataTable
            :columns="columns"
            :data="voiceprints"
            :loading="loading"
            :pagination="{ pageSize: 10 }"
            :scroll-x="820"
            size="small"
          />
        </div>
        <div v-else class="flex flex-1 items-center justify-center py-10">
          <NEmpty description="声纹库还是空的，先注册一位常驻发言人即可开始测试实名匹配。" class="empty-shell" />
        </div>
      </NCard>

      <NCard class="card-main">
        <div class="text-sm font-700 text-ink">
          注册新声纹
        </div>
        <div class="mt-1 text-xs leading-6 text-slate">
          建议上传 15 到 30 秒、单人、近讲、背景较干净的语音样本。多人串音或会议远场录音会明显降低匹配稳定性。
        </div>

        <div class="mt-4 grid gap-3">
          <NInput v-model:value="speakerName" placeholder="说话人姓名，例如 张三" />
          <NInput v-model:value="department" placeholder="所属部门，可选" />
          <NInput
            v-model:value="notes"
            type="textarea"
            placeholder="备注信息，可选，例如 主讲人 / 常驻发言人"
            :autosize="{ minRows: 3, maxRows: 5 }"
          />

          <input
            ref="fileInput"
            type="file"
            accept=".wav,.mp3,.m4a,.aac,.flac,.ogg,.opus,.webm"
            class="hidden"
            @change="handleFileSelected"
          >

          <div class="rounded-2 border border-dashed border-slate-200 bg-white/70 px-4 py-3">
            <div class="audio-mode-switcher" role="tablist" aria-label="选择声纹采集方式">
              <button
                type="button"
                class="audio-mode-switcher__item"
                :class="{ 'is-active': audioInputMode === 'upload' }"
                :aria-selected="audioInputMode === 'upload'"
                @click="selectAudioInputMode('upload')"
              >
                <span class="audio-mode-switcher__title">选择注册音频</span>
                <span class="audio-mode-switcher__hint">上传已有样本</span>
              </button>
              <button
                type="button"
                class="audio-mode-switcher__item"
                :class="{ 'is-active': audioInputMode === 'record' }"
                :aria-selected="audioInputMode === 'record'"
                @click="selectAudioInputMode('record')"
              >
                <span class="audio-mode-switcher__title">直接录音</span>
                <span class="audio-mode-switcher__hint">现场采集并回放</span>
              </button>
            </div>

            <div class="mt-3 rounded-2 bg-[#fbfdff] p-3">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <div class="text-sm font-600 text-ink">
                    {{ audioInputModeMeta.title }}
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate/75">
                    {{ audioInputModeMeta.description }}
                  </div>
                </div>
                <div class="flex flex-wrap items-center gap-2">
                  <NButton
                    v-if="audioInputMode === 'upload'"
                    secondary
                    size="small"
                    :disabled="isRecording"
                    @click="openFilePicker"
                  >
                    {{ audioInputModeMeta.actionLabel }}
                  </NButton>
                  <NButton
                    v-else-if="!isRecording"
                    type="primary"
                    color="#0f766e"
                    size="small"
                    @click="handleStartRecording"
                  >
                    {{ audioInputModeMeta.actionLabel }}
                  </NButton>
                  <NButton
                    v-else
                    type="error"
                    secondary
                    size="small"
                    @click="handleStopRecording"
                  >
                    {{ audioInputModeMeta.actionLabel }}
                  </NButton>
                  <NButton v-if="selectedFile || isRecording" text size="small" @click="handleClearAudio">
                    清空样本
                  </NButton>
                </div>
              </div>

              <div class="mt-3 flex flex-wrap items-center gap-2">
                <NTag round :bordered="false" size="small" :type="audioInputMode === 'upload' ? 'info' : 'success'">
                  当前方式：{{ audioInputMode === 'upload' ? '上传音频' : '直接录音' }}
                </NTag>
                <NTag v-if="isRecording" round :bordered="false" type="error" size="small">
                  录音中 {{ formatRecordingElapsed(recordingElapsedSeconds) }}
                </NTag>
              </div>
            </div>

            <div v-if="audioInputMode === 'record'" class="mt-3 rounded-2 border border-slate-200 bg-white/70 p-4">
              <div class="flex flex-wrap items-center justify-between gap-3">
                <div>
                  <div class="text-sm font-600 text-ink">
                    示例朗读文案
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate/75">
                    选择一段文案并自然朗读，更容易录到稳定、干净的个人声纹样本。
                  </div>
                </div>
                <NButton text size="small" @click="handleCopyScript">
                  复制当前文案
                </NButton>
              </div>

              <div class="mt-3 grid gap-2 sm:grid-cols-3">
                <button
                  v-for="item in recordingScriptOptions"
                  :key="item.id"
                  type="button"
                  class="rounded-2 border px-3 py-3 text-left transition-all duration-150"
                  :class="selectedScriptID === item.id ? 'border-teal bg-teal/[0.08] shadow-sm' : 'border-gray-200 bg-white hover:border-gray-300'"
                  @click="selectedScriptID = item.id"
                >
                  <div class="text-sm font-600 text-ink">
                    {{ item.title }}
                  </div>
                  <div class="mt-1 text-xs leading-5 text-slate/75">
                    {{ item.tip }}
                  </div>
                </button>
              </div>

              <div class="mt-3 rounded-2 bg-[#fbfdff] p-4">
                <div class="text-xs font-600 text-slate/80">
                  当前文案
                </div>
                <div class="mt-2 whitespace-pre-wrap text-sm leading-7 text-ink">
                  {{ selectedScript.content }}
                </div>
              </div>
            </div>

            <div class="mt-3 flex flex-wrap items-center gap-2">
              <NButton v-if="selectedFile || isRecording" text size="small" @click="handleClearAudio">
                清空样本
              </NButton>
            </div>
            <div class="mt-2 text-xs leading-6 text-slate">
              {{ selectedFile ? `${selectedFile.name} · ${Math.max(1, Math.round(selectedFile.size / 1024))} KB` : '支持 wav / mp3 / m4a / flac / ogg / opus / webm，也支持直接录音生成 wav。' }}
            </div>
            <div v-if="isRecording" class="mt-2 text-xs leading-6 text-amber-700">
              请保持单人近讲，按上方示例文案自然朗读 15 到 30 秒后再停止录音。
            </div>
            <div v-if="previewAudioURL" class="mt-3 rounded-2 bg-[#fbfdff] p-3">
              <div class="text-xs text-slate/70">
                {{ selectedAudioLabel }}
              </div>
              <audio class="mt-2 w-full" :src="previewAudioURL" controls preload="metadata" />
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <NButton type="primary" color="#0f766e" :loading="saving" :disabled="isRecording" @click="handleSubmit">
              注册声纹
            </NButton>
            <NButton quaternary :disabled="saving" @click="clearForm">
              重置表单
            </NButton>
            <NButton quaternary :loading="loading" @click="loadVoiceprints()">
              刷新列表
            </NButton>
          </div>
        </div>
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.audio-mode-switcher {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 10px;
}

.audio-mode-switcher__item {
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 4px;
  border: 1px solid rgba(148, 163, 184, 0.22);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.9);
  padding: 14px 16px;
  text-align: left;
  cursor: pointer;
  transition: border-color 0.18s ease, background-color 0.18s ease, box-shadow 0.18s ease, transform 0.18s ease;
}

.audio-mode-switcher__item:hover {
  border-color: rgba(15, 118, 110, 0.32);
  box-shadow: 0 10px 24px rgba(15, 23, 42, 0.06);
  transform: translateY(-1px);
}

.audio-mode-switcher__item.is-active {
  border-color: rgba(15, 118, 110, 0.46);
  background: rgba(240, 253, 250, 0.92);
  box-shadow: 0 12px 26px rgba(15, 118, 110, 0.08);
}

.audio-mode-switcher__title {
  color: #16202c;
  font-size: 14px;
  font-weight: 700;
  line-height: 1.4;
}

.audio-mode-switcher__hint {
  color: #64748b;
  font-size: 12px;
  line-height: 1.5;
}

@media (max-width: 640px) {
  .audio-mode-switcher {
    grid-template-columns: 1fr;
  }
}
</style>

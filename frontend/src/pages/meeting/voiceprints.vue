<script setup lang="ts">
import type { AxiosError } from 'axios'

import { NButton, NTag, useMessage } from 'naive-ui'
import { computed, h, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import type { VoiceprintItem } from '@/api/voiceprint'

import { createVoiceprint, deleteVoiceprint, getVoiceprints } from '@/api/voiceprint'
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
const speakerName = ref('')
const department = ref('')
const notes = ref('')

const voiceprintCount = computed(() => voiceprints.value.length)

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

function openFilePicker() {
  fileInput.value?.click()
}

function clearForm() {
  speakerName.value = ''
  department.value = ''
  notes.value = ''
  selectedFile.value = null
  if (fileInput.value)
    fileInput.value.value = ''
}

function handleFileSelected(event: Event) {
  const target = event.target as HTMLInputElement | null
  selectedFile.value = target?.files?.[0] || null
  if (target)
    target.value = ''
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
    description: '删除后，speaker_diarize 节点将无法再把该说话人自动映射成实名。',
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
</script>

<template>
  <div class="grid gap-5">
    <NCard class="card-main">
      <div class="flex flex-col gap-4 lg:flex-row lg:items-center lg:justify-between">
        <div>
          <div class="text-sm font-700 text-ink">
            声纹库
          </div>
          <div class="mt-1 text-sm leading-6 text-slate">
            这里维护会议纪要里 speaker_diarize 节点用于实名匹配的说话人样本。注册完成后，在工作流里打开“启用声纹匹配”即可把匿名 speaker 标签替换成已登记姓名。
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
          <NButton type="primary" color="#0f766e" @click="router.push('/meetings')">
            返回会议
          </NButton>
        </div>
      </div>

      <div class="mt-4 rounded-2 border border-emerald-200 bg-emerald-50/80 px-4 py-3 text-xs leading-6 text-emerald-800">
        <div class="text-sm font-700 text-emerald-900">
          当前 Speaker Analysis Service
        </div>
        <div class="mt-1 break-all">
          {{ serviceURL || '首次成功连通后会显示当前服务地址。若一直为空，请检查后端 services.speaker_analysis_url 配置。' }}
        </div>
      </div>
    </NCard>

    <div class="grid gap-5 xl:grid-cols-[1.1fr,1.6fr]">
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
            <div class="flex flex-wrap items-center gap-2">
              <NButton secondary size="small" @click="openFilePicker">
                选择注册音频
              </NButton>
              <NButton v-if="selectedFile" text size="small" @click="selectedFile = null">
                清空
              </NButton>
            </div>
            <div class="mt-2 text-xs leading-6 text-slate">
              {{ selectedFile ? `${selectedFile.name} · ${Math.max(1, Math.round(selectedFile.size / 1024))} KB` : '支持 wav / mp3 / m4a / flac / ogg / opus / webm。' }}
            </div>
          </div>

          <div class="flex flex-wrap items-center gap-2">
            <NButton type="primary" color="#0f766e" :loading="saving" @click="handleSubmit">
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

      <NCard class="card-main" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
        <template #header>
          <div class="flex items-center justify-between gap-3">
            <span class="text-sm font-600">已注册说话人</span>
            <span class="text-xs text-slate">speaker_diarize 开启声纹匹配后会直接复用这里的库</span>
          </div>
        </template>

        <div v-if="voiceprints.length > 0 || loading" class="flex flex-1 min-h-0 flex-col overflow-hidden">
          <NDataTable
            flex-height
            class="flex-1 min-h-0"
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
    </div>
  </div>
</template>
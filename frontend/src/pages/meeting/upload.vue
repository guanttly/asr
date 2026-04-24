<script setup lang="ts">
import { useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'
import { useRouter } from 'vue-router'

import { uploadMeetingFile } from '@/api/meeting'
import WorkflowSelectionPreview from '@/components/WorkflowSelectionPreview.vue'
import { useWorkflowBindingStatus } from '@/composables/useWorkflowBindingStatus'
import { useWorkflowCatalog } from '@/composables/useWorkflowCatalog'
import { WORKFLOW_BINDING_KEYS, WORKFLOW_TYPES } from '@/types/workflow'

const router = useRouter()
const message = useMessage()
const meetingWorkflowCatalog = useWorkflowCatalog(WORKFLOW_TYPES.MEETING)
const {
  configuredWorkflowId,
  configuredWorkflow: selectedWorkflow,
  configuredWorkflowMissing,
  configuredWorkflowNotice: workflowConfigNotice,
} = useWorkflowBindingStatus(WORKFLOW_BINDING_KEYS.MEETING, meetingWorkflowCatalog, {
  emptyLabel: '未配置默认工作流',
  unsetMessage: '当前未配置会议默认工作流，上传后会先完成转写，后续可在会议详情页继续生成摘要。',
  missingMessage: workflowId => `应用配置中的会议工作流 #${workflowId} 当前不可用，请前往应用配置页重新选择。`,
  readyMessage: () => '上传的会议录音会直接进入会议纪要链路，并自动复用应用配置中的会议工作流。',
})
const loading = ref(false)
const fileInput = ref<HTMLInputElement | null>(null)
const selectedFiles = ref<File[]>([])

const totalSelectedSizeText = computed(() => {
  const totalBytes = selectedFiles.value.reduce((sum, file) => sum + file.size, 0)
  if (!totalBytes)
    return '未选择文件'
  return `已选 ${selectedFiles.value.length} 个文件，共 ${(totalBytes / 1024 / 1024).toFixed(2)} MB`
})

async function loadWorkflows() {
  try {
    await meetingWorkflowCatalog.loadWorkflows()
  }
  catch {
    message.error('工作流列表加载失败')
  }
}

async function handleSubmit() {
  if (selectedFiles.value.length === 0) {
    message.warning('请先选择至少一个音频文件')
    return
  }

  loading.value = true
  try {
    const createdMeetings: Array<{ id: number }> = []
    for (const file of selectedFiles.value) {
      const formData = new FormData()
      formData.append('file', file)
      if (configuredWorkflowId.value)
        formData.append('workflow_id', String(configuredWorkflowId.value))
      const result = await uploadMeetingFile(formData)
      if (result.data?.meeting?.id)
        createdMeetings.push(result.data.meeting)
    }
    const count = createdMeetings.length
    message.success(count > 1 ? `已创建 ${count} 条会议任务，系统会在会议列表内持续更新状态` : '会议任务已创建，系统会在会议详情页持续更新状态')
    selectedFiles.value = []
    if (fileInput.value)
      fileInput.value.value = ''
    if (count === 1)
      router.push({ name: 'meeting-detail', params: { id: String(createdMeetings[0].id) } })
    else
      router.push('/meetings')
  }
  catch {
    message.error('会议任务创建失败')
  }
  finally {
    loading.value = false
  }
}

function handleChooseFile() {
  fileInput.value?.click()
}

function handleFileSelected(event: Event) {
  const target = event.target as HTMLInputElement | null
  const files = Array.from(target?.files || [])
  if (files.length === 0)
    return

  const merged = [...selectedFiles.value]
  for (const file of files) {
    const exists = merged.some(item => item.name === file.name && item.size === file.size && item.lastModified === file.lastModified)
    if (!exists)
      merged.push(file)
  }
  selectedFiles.value = merged
  if (target)
    target.value = ''
}

function removeSelectedFile(index: number) {
  selectedFiles.value.splice(index, 1)
}

function clearSelectedFiles() {
  selectedFiles.value = []
  if (fileInput.value)
    fileInput.value.value = ''
}

onMounted(loadWorkflows)
</script>

<template>
  <div class="grid gap-5">
    <NCard class="card-main">
      <template #header>
        <span class="text-sm font-600">会议录音导入</span>
      </template>
      <NForm label-placement="top">
        <NFormItem label="会议音频文件">
          <div class="grid w-full gap-3">
            <input ref="fileInput" type="file" accept=".wav,.mp3,.m4a,.aac,.flac,.ogg,.opus,.webm" multiple class="hidden" @change="handleFileSelected">
            <div class="flex flex-wrap items-center gap-2">
              <NButton type="primary" color="#0f766e" secondary @click="handleChooseFile">
                选择音频文件
              </NButton>
              <NButton v-if="selectedFiles.length" quaternary @click="clearSelectedFiles">
                清空已选
              </NButton>
            </div>
            <div class="rounded-2 border border-dashed border-gray-300 bg-[#fbfdff] px-4 py-3 text-sm text-slate">
              上传后会直接创建会议记录，并由会议应用独立提交转写与摘要处理。支持单个大文件，也支持一次选择多个小文件批量提交。
            </div>
            <div class="text-xs text-slate/80">
              {{ totalSelectedSizeText }}
            </div>
            <div v-if="selectedFiles.length" class="grid gap-2">
              <div v-for="(file, index) in selectedFiles" :key="`${file.name}-${file.size}-${file.lastModified}`" class="flex items-center justify-between rounded-2 bg-mist/60 px-3 py-2 text-sm text-ink">
                <div class="min-w-0 flex-1 pr-3">
                  <div class="truncate font-600">
                    {{ file.name }}
                  </div>
                  <div class="mt-1 text-xs text-slate">
                    {{ (file.size / 1024 / 1024).toFixed(2) }} MB
                  </div>
                </div>
                <NButton text size="small" type="error" @click="removeSelectedFile(index)">
                  移除
                </NButton>
              </div>
            </div>
          </div>
        </NFormItem>
        <NFormItem label="会议工作流">
          <div class="grid w-full gap-3">
            <div class="rounded-2 border px-3 py-2 text-xs leading-6" :class="configuredWorkflowMissing ? 'border-amber-200 bg-amber-50 text-amber-700' : 'border-transparent bg-mist/70 text-slate'">
              {{ workflowConfigNotice }}
            </div>
            <div class="flex justify-end">
              <NButton text size="small" @click="router.push('/workflows/application-settings')">
                前往应用配置
              </NButton>
            </div>
            <WorkflowSelectionPreview
              :workflow="selectedWorkflow"
              :loading="meetingWorkflowCatalog.loading.value"
              empty-title="未配置会议默认工作流"
              empty-description="前往应用配置页设置后，这里会展示会议录音导入时复用的会议处理链路。"
            />
          </div>
        </NFormItem>

        <div class="flex justify-end gap-3">
          <NButton @click="router.push('/meetings')">
            返回列表
          </NButton>
          <NButton type="primary" color="#0f766e" :loading="loading" @click="handleSubmit">
            上传并开始转写
          </NButton>
        </div>
      </NForm>
    </NCard>
  </div>
</template>

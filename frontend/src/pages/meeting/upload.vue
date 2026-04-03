<script setup lang="ts">
import { reactive, ref } from 'vue'
import { useMessage } from 'naive-ui'
import { useRouter } from 'vue-router'

import { createMeeting } from '@/api/meeting'

const router = useRouter()
const message = useMessage()
const loading = ref(false)
const form = reactive({
  title: '',
  audio_url: '',
  duration: 0,
})

async function handleSubmit() {
  if (!form.title || !form.audio_url) {
    message.warning('请填写会议标题和音频地址')
    return
  }

  loading.value = true
  try {
    const result = await createMeeting(form)
    message.success('会议创建成功')
    router.push(`/meetings/${result.data.id}`)
  }
  catch {
    message.error('会议创建失败')
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="grid gap-5">

    <NCard class="card-main">
      <template #header>
        <span class="text-sm font-600">会议信息</span>
      </template>
      <NForm :model="form" label-placement="top">
        <NFormItem label="会议标题">
          <NInput v-model:value="form.title" placeholder="请输入会议标题" />
        </NFormItem>
        <NFormItem label="音频地址">
          <NInput v-model:value="form.audio_url" placeholder="请输入可访问的音频 URL" />
        </NFormItem>
        <NFormItem label="时长（秒）">
          <NInputNumber v-model:value="form.duration" :min="0" class="w-full sm:w-60" />
        </NFormItem>

        <div class="flex justify-end gap-3">
          <NButton @click="router.push('/meetings')">返回列表</NButton>
          <NButton type="primary" color="#0f766e" :loading="loading" @click="handleSubmit">创建会议</NButton>
        </div>
      </NForm>
    </NCard>
  </div>
</template>
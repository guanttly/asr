<script setup lang="ts">
import type { AxiosError } from 'axios'

import { useMessage } from 'naive-ui'
import { reactive, ref } from 'vue'
import { useRouter } from 'vue-router'

import { login } from '@/api/user'
import { useAppStore } from '@/stores/app'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const message = useMessage()
const appStore = useAppStore()
const userStore = useUserStore()

const form = reactive({
  username: 'admin',
  password: '123456',
})

const loading = ref(false)

async function handleLogin() {
  loading.value = true
  try {
    const result = await login(form)
    userStore.setToken(result.data.token)
    await userStore.bootstrap()
    await appStore.bootstrapWorkflowBindings()
    message.success('登录成功')
    router.push('/dashboard')
  }
  catch (error) {
    const responseMessage = (error as AxiosError<{ message?: string }>)?.response?.data?.message
    message.warning(responseMessage || '登录失败，请确认 gateway 与 admin-api 已启动，且默认管理员已初始化')
  }
  finally {
    loading.value = false
  }
}
</script>

<template>
  <div class="page-shell p-4 sm:p-6">
    <div class="scroll-shell mx-auto grid h-full w-full max-w-6xl content-center gap-5 grid-cols-1 lg:grid-cols-[1.1fr_0.9fr] lg:items-center">
      <section class="card-main px-5 py-6 sm:px-7 sm:py-8">
        <div class="inline-flex rounded-full bg-tide/12 px-4 py-1 text-sm font-600 text-tide">
          语音转写与会议摘要平台
        </div>
        <h1 class="mt-5 font-display text-3xl leading-tight font-700 text-ink sm:text-4xl lg:text-5xl">
          为局域网私有部署准备的现代化转写工作台
        </h1>
        <p class="mt-4 max-w-2xl text-sm leading-7 text-slate sm:text-base">
          统一连接流式 ASR、术语纠错、会议逐字稿和纪要生成。前端骨架已经就位，后续只需要接入真实接口即可进入联调。
        </p>

        <div class="mt-8 grid grid-cols-1 gap-3 md:grid-cols-3">
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              实时转写
            </div>
            <div class="mt-1.5 text-xl font-700 text-ink">
              P95 ≤ 1.5s
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              术语纠错
            </div>
            <div class="mt-1.5 text-xl font-700 text-ink">
              三层管道
            </div>
          </div>
          <div class="subtle-panel">
            <div class="text-xs text-slate/70">
              会议摘要
            </div>
            <div class="mt-1.5 text-xl font-700 text-ink">
              结构化输出
            </div>
          </div>
        </div>
      </section>

      <section class="card-main p-5 sm:p-6">
        <div class="mb-6">
          <div class="font-display text-2xl font-700 text-ink">
            登录
          </div>
          <div class="mt-1 text-sm text-slate/70">
            管理后台、转写工作台和会议模块共用统一认证。
          </div>
        </div>

        <NForm :model="form" label-placement="top">
          <NFormItem label="用户名">
            <NInput v-model:value="form.username" placeholder="请输入用户名" />
          </NFormItem>
          <NFormItem label="密码">
            <NInput v-model:value="form.password" type="password" show-password-on="click" placeholder="请输入密码" />
          </NFormItem>
          <div class="mb-4 rounded-2.5 bg-mist/60 px-4 py-3 text-sm text-slate">
            默认管理员会在 admin-api 启动时自动创建：{{ form.username }} / {{ form.password }}
          </div>
          <NButton block type="primary" color="#0f766e" size="large" :loading="loading" @click="handleLogin">
            进入系统
          </NButton>
        </NForm>
      </section>
    </div>
  </div>
</template>

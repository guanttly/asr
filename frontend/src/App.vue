<script setup lang="ts">
import { dateZhCN, zhCN } from 'naive-ui'
import { watch } from 'vue'

import { useBusinessSocket } from '@/composables/useBusinessSocket'
import { useUserStore } from '@/stores/user'

const userStore = useUserStore()
const { connect, disconnect } = useBusinessSocket()

watch(() => userStore.token, (token) => {
  if (!token) {
    disconnect()
    return
  }
  void connect(token).catch(() => undefined)
}, { immediate: true })
</script>

<template>
  <NConfigProvider :locale="zhCN" :date-locale="dateZhCN">
    <NLoadingBarProvider>
      <NDialogProvider>
        <NNotificationProvider>
          <NMessageProvider>
            <RouterView />
          </NMessageProvider>
        </NNotificationProvider>
      </NDialogProvider>
    </NLoadingBarProvider>
  </NConfigProvider>
</template>

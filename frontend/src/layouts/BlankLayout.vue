<script setup lang="ts">
const buildLabel = `版本 ${__APP_VERSION__}-${__APP_BUILD_CODE__}`
const buildTitle = `构建日期 ${__APP_BUILD_DATE__}`
</script>

<template>
  <RouterView />
  <div class="build-badge" :title="buildTitle">
    {{ buildLabel }}
  </div>
</template>

<style scoped>
.build-badge {
  position: fixed;
  right: 16px;
  bottom: 14px;
  z-index: 50;
  max-width: calc(100vw - 24px);
  padding: 6px 12px;
  border: 1px solid rgba(148, 163, 184, 0.28);
  border-radius: 999px;
  background: rgba(15, 23, 42, 0.78);
  color: #f8fafc;
  font-size: 12px;
  font-weight: 600;
  line-height: 1.2;
  box-shadow: 0 12px 24px rgba(15, 23, 42, 0.16);
  backdrop-filter: blur(10px);
}

@media (max-width: 640px) {
  .build-badge {
    right: 50%;
    bottom: 10px;
    transform: translateX(50%);
    padding: 6px 10px;
    font-size: 11px;
  }
}
</style>

<script setup lang="ts">
import { computed, h, onMounted, reactive, ref } from 'vue'
import { NTag, useMessage } from 'naive-ui'

import { createUser, getUsers } from '@/api/user'
import { useUserStore } from '@/stores/user'

type UserItem = {
  id: number
  username: string
  display_name?: string
  displayName?: string
  role: string
}

const message = useMessage()
const userStore = useUserStore()
const loading = ref(false)
const creating = ref(false)
const showCreateModal = ref(false)
const keyword = ref('')
const users = ref<UserItem[]>([])
const createForm = reactive({
  username: '',
  password: '',
  display_name: '',
  role: 'user' as 'admin' | 'user',
})

const columns = [
  { title: 'ID', key: 'id', width: 64 },
  { title: '用户名', key: 'username', width: 140 },
  { title: '显示名', key: 'displayName', render: (row: UserItem) => row.displayName || row.display_name || '-' },
  {
    title: '角色',
    key: 'role',
    width: 88,
    render: (row: UserItem) => h(NTag, {
      size: 'small',
      round: true,
      bordered: false,
      type: row.role === 'admin' ? 'warning' : 'default',
    }, { default: () => row.role === 'admin' ? '管理员' : '普通用户' }),
  },
]

const filteredUsers = computed(() => {
  const value = keyword.value.trim().toLowerCase()
  if (!value)
    return users.value
  return users.value.filter(item =>
    item.username.toLowerCase().includes(value)
    || (item.displayName || item.display_name || '').toLowerCase().includes(value)
    || item.role.toLowerCase().includes(value),
  )
})

async function loadUsers() {
  loading.value = true
  try {
    const result = await getUsers({ offset: 0, limit: 100 })
    users.value = result.data.items
  }
  catch {
    message.error('用户列表加载失败')
  }
  finally {
    loading.value = false
  }
}

async function handleCreateUser() {
  if (!createForm.username || !createForm.password) {
    message.warning('请填写用户名和密码')
    return
  }

  creating.value = true
  try {
    await createUser(createForm)
    message.success('用户创建成功')
    showCreateModal.value = false
    createForm.username = ''
    createForm.password = ''
    createForm.display_name = ''
    createForm.role = 'user'
    await loadUsers()
  }
  catch {
    message.error('用户创建失败，请检查是否重名或权限不足')
  }
  finally {
    creating.value = false
  }
}

onMounted(loadUsers)
</script>

<template>
  <div class="flex-1 flex flex-col min-h-0 gap-5">

    <NCard class="card-main flex flex-col min-h-0 flex-1" content-style="display: flex; flex-direction: column; min-height: 0; padding: 0 20px 20px;">
      <template #header>
        <div class="flex flex-wrap items-center justify-between gap-3">
          <span class="text-sm font-600">用户列表</span>
          <div class="flex flex-wrap items-center gap-2">
            <NInput v-model:value="keyword" clearable placeholder="搜索用户名 / 显示名 / 角色" size="small" class="w-full sm:!w-56" />
            <NButton quaternary size="small" @click="loadUsers">刷新</NButton>
            <NButton v-if="userStore.profile?.role === 'admin'" type="primary" size="small" color="#0f766e" @click="showCreateModal = true">新增用户</NButton>
          </div>
        </div>
      </template>
      <NDataTable flex-height class="flex-1 min-h-0" :columns="columns" :data="filteredUsers" :loading="loading" :pagination="{ pageSize: 10 }" size="small" />
    </NCard>

    <NModal v-model:show="showCreateModal" preset="card" title="新增用户" class="modal-card max-w-140">
      <NForm :model="createForm" label-placement="top">
        <NFormItem label="用户名">
          <NInput v-model:value="createForm.username" placeholder="请输入用户名" />
        </NFormItem>
        <NFormItem label="密码">
          <NInput v-model:value="createForm.password" type="password" show-password-on="click" placeholder="请输入密码" />
        </NFormItem>
        <NFormItem label="显示名">
          <NInput v-model:value="createForm.display_name" placeholder="请输入显示名" />
        </NFormItem>
        <NFormItem label="角色">
          <NSelect
            v-model:value="createForm.role"
            :options="[
              { label: '普通用户', value: 'user' },
              { label: '管理员', value: 'admin' },
            ]"
          />
        </NFormItem>

        <div class="flex justify-end gap-3">
          <NButton @click="showCreateModal = false">取消</NButton>
          <NButton type="primary" color="#0f766e" :loading="creating" @click="handleCreateUser">创建</NButton>
        </div>
      </NForm>
    </NModal>
  </div>
</template>
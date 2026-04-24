<script setup lang="ts">
import type { AxiosError } from 'axios'

import type { DownloadArtifact } from '@/api/downloads'
import { useMessage } from 'naive-ui'
import { computed, onMounted, ref } from 'vue'

import { useRouter } from 'vue-router'

import { getDownloadArtifacts } from '@/api/downloads'
import { useUserStore } from '@/stores/user'

const router = useRouter()
const message = useMessage()
const userStore = useUserStore()

const CERT_DOWNLOAD_PATH = '/downloads/certs/tls.crt'

const loading = ref(false)
const artifacts = ref<DownloadArtifact[]>([])

const latestArtifact = computed(() => artifacts.value[0] ?? null)
const hasLogin = computed(() => Boolean(userStore.token))
const certDownloadPath = computed(() => CERT_DOWNLOAD_PATH)
const certDownloadUrl = computed(() => {
  if (typeof window === 'undefined')
    return CERT_DOWNLOAD_PATH
  return new URL(CERT_DOWNLOAD_PATH, window.location.origin).toString()
})

function formatSize(sizeBytes: number) {
  if (!Number.isFinite(sizeBytes) || sizeBytes <= 0)
    return '未知大小'

  const units = ['B', 'KB', 'MB', 'GB']
  let value = sizeBytes
  let unitIndex = 0
  while (value >= 1024 && unitIndex < units.length - 1) {
    value /= 1024
    unitIndex += 1
  }
  return `${value >= 100 || unitIndex === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[unitIndex]}`
}

function formatTime(value: string) {
  if (!value)
    return '未知时间'

  const parsed = new Date(value)
  if (Number.isNaN(parsed.getTime()))
    return '未知时间'

  return parsed.toLocaleString('zh-CN', { hour12: false })
}

async function loadArtifacts() {
  loading.value = true
  try {
    const result = await getDownloadArtifacts()
    artifacts.value = result.data?.items ?? []
  }
  catch (error) {
    const responseMessage = (error as AxiosError<{ message?: string }>)?.response?.data?.message
    message.warning(responseMessage || '下载包列表读取失败，请确认 admin-api 与挂载目录已就绪')
  }
  finally {
    loading.value = false
  }
}

async function handleCopyLink(url: string) {
  try {
    await navigator.clipboard.writeText(new URL(url, window.location.origin).toString())
    message.success('下载链接已复制')
  }
  catch {
    message.warning('当前浏览器不支持复制，请直接点击下载')
  }
}

onMounted(() => {
  loadArtifacts()
})

function handlePortalEntry() {
  router.push(hasLogin.value ? '/dashboard' : '/login')
}
</script>

<template>
  <div class="downloads-page">
    <div class="downloads-page__inner mx-auto flex min-h-full w-full max-w-6xl flex-col gap-5">
      <NCard class="card-main overflow-hidden">
        <div class="download-hero grid gap-4 lg:grid-cols-[1.5fr_1fr]">
          <div>
            <div class="download-pill">
              Public Download Portal
            </div>
            <h2 class="mt-4 text-2xl font-700 text-ink sm:text-3xl">
              终端安装包下载
            </h2>
            <p class="mt-3 max-w-3xl text-sm leading-7 text-slate/78 sm:text-base">
              这是一个免登录公共入口页，直接读取容器挂载出来的下载目录，适合集中分发 Windows 安装包。运维只需要把最新构建产物放进下载挂载目录，页面会自动刷新可见列表。
            </p>

            <div class="mt-4 rounded-3 border border-amber-200 bg-amber-50 px-4 py-3 text-sm leading-7 text-amber-800">
              浏览器端实时语音采集依赖安全上下文。远程网页访问时，实时录音通常需要 HTTPS 或 localhost；如果只是 HTTP 页面，建议优先下载终端客户端，或在服务器前面接入 HTTPS 反向代理。
            </div>

            <div class="mt-5 flex flex-wrap gap-3 text-sm text-slate/78">
              <div class="download-stat">
                <span class="download-stat-label">可下载版本</span>
                <span class="download-stat-value">{{ artifacts.length }}</span>
              </div>
              <div class="download-stat">
                <span class="download-stat-label">挂载地址</span>
                <span class="download-stat-value">/var/lib/asr/downloads</span>
              </div>
              <div class="download-stat">
                <span class="download-stat-label">证书下载</span>
                <span class="download-stat-value break-all">{{ certDownloadPath }}</span>
              </div>
            </div>

            <div class="mt-5 flex flex-wrap gap-2">
              <NButton type="primary" color="#0f766e" @click="handlePortalEntry">
                {{ hasLogin ? '进入后台' : '管理员登录' }}
              </NButton>
              <NButton tertiary :loading="loading" @click="loadArtifacts">
                刷新下载列表
              </NButton>
            </div>
          </div>

          <div class="download-highlight">
            <div class="text-xs font-700 uppercase tracking-[0.18em] text-teal-700/78">
              最新推荐
            </div>
            <template v-if="latestArtifact">
              <div class="mt-3 text-lg font-700 text-ink break-all">
                {{ latestArtifact.name }}
              </div>
              <div class="mt-3 flex flex-wrap gap-2 text-xs text-slate/72">
                <NTag size="small" round :bordered="false" type="success">
                  {{ formatSize(latestArtifact.size_bytes) }}
                </NTag>
                <NTag size="small" round :bordered="false" type="info">
                  {{ formatTime(latestArtifact.modified_at) }}
                </NTag>
              </div>
              <div class="mt-5 flex flex-wrap gap-2">
                <a :href="latestArtifact.download_url" class="inline-flex">
                  <NButton type="primary" color="#0f766e">
                    立即下载
                  </NButton>
                </a>
                <NButton secondary @click="handleCopyLink(latestArtifact.download_url)">
                  复制链接
                </NButton>
              </div>
            </template>
            <NEmpty v-else description="下载目录还没有安装包" class="empty-shell mt-4" />
          </div>
        </div>
      </NCard>

      <NCard class="card-main overflow-hidden">
        <div class="certificate-panel grid gap-4 xl:grid-cols-[1.1fr_1.3fr]">
          <section class="certificate-hero">
            <div class="certificate-badge">
              TLS Trust Kit
            </div>
            <h3 class="mt-4 text-xl font-700 text-ink sm:text-2xl">
              服务器证书下载
            </h3>
            <p class="mt-3 text-sm leading-7 text-slate/78 sm:text-base">
              如果客户希望浏览器、桌面端长期信任当前 HTTPS 站点，可以直接从这里下载服务器证书并导入到系统或浏览器信任库。实施人员不需要再逐台机器手工拷证书文件。
            </p>

            <div class="certificate-meta mt-4">
              <div class="certificate-meta-label">
                证书链接
              </div>
              <div class="certificate-meta-value">
                {{ certDownloadUrl }}
              </div>
            </div>

            <div class="mt-5 flex flex-wrap gap-2">
              <a :href="certDownloadPath" class="inline-flex">
                <NButton type="primary" color="#0f766e">
                  下载证书
                </NButton>
              </a>
              <NButton tertiary @click="handleCopyLink(certDownloadPath)">
                复制证书链接
              </NButton>
            </div>

            <div class="mt-4 rounded-3 border border-sky-200 bg-sky-50 px-4 py-3 text-sm leading-7 text-sky-800">
              如果点击下载返回 404，通常表示服务器刚部署完成但还没有生成自签名证书。此时先完成一次 all-in-one 安装，再刷新当前页面即可。
            </div>
          </section>

          <section class="certificate-guide">
            <div class="guide-block">
              <div class="guide-title">
                Windows Chrome / Edge
              </div>
              <ol class="guide-list">
                <li>点击上面的“下载证书”，保存 `asr-server.crt`。</li>
                <li>双击证书文件，选择“安装证书”。</li>
                <li>选择“本地计算机”，继续下一步。</li>
                <li>选择“将所有的证书都放入下列存储”。</li>
                <li>目标存储选择“受信任的根证书颁发机构”。</li>
                <li>完成后重启 Chrome 或 Edge，再访问 HTTPS 地址。</li>
              </ol>
            </div>

            <div class="guide-block">
              <div class="guide-title">
                Firefox
              </div>
              <ol class="guide-list">
                <li>下载证书文件 `asr-server.crt`。</li>
                <li>打开“设置 -> 隐私与安全 -> 证书 -> 查看证书”。</li>
                <li>进入“证书机构”页签并点击“导入”。</li>
                <li>选择刚下载的证书文件。</li>
                <li>勾选“信任此 CA 以标识网站”。</li>
                <li>导入后重启 Firefox，再访问 HTTPS 页面。</li>
              </ol>
            </div>

            <div class="guide-block guide-note">
              <div class="guide-title">
                使用提示
              </div>
              <ul class="guide-list unordered">
                <li>证书导入后，浏览器访问登录页、下载页和实时录音页都不会再反复弹出自签名告警。</li>
                <li>如果服务器重新生成了一套新证书，需要让客户重新下载并覆盖导入。</li>
                <li>桌面客户端现在也支持忽略自签名证书错误，但浏览器端麦克风权限仍然更适合通过受信任的 HTTPS 使用。</li>
              </ul>
            </div>
          </section>
        </div>
      </NCard>

      <NCard class="card-main" content-style="padding: 0 20px 20px;">
        <template #header>
          <div class="flex flex-wrap items-center justify-between gap-3">
            <div>
              <div class="text-sm font-700 text-ink">
                安装包列表
              </div>
              <div class="mt-1 text-xs text-slate/70">
                页面数据来自 /api/admin/public/downloads，文件实体由 Nginx 直接分发。
              </div>
            </div>
            <NButton quaternary size="small" :loading="loading" @click="loadArtifacts">
              刷新列表
            </NButton>
          </div>
        </template>

        <div v-if="artifacts.length" class="grid gap-4 md:grid-cols-2 xl:grid-cols-3">
          <article v-for="artifact in artifacts" :key="artifact.name" class="download-card">
            <div class="flex items-start justify-between gap-3">
              <div class="min-w-0">
                <div class="download-card-title">
                  {{ artifact.name }}
                </div>
                <div class="mt-2 text-xs text-slate/68">
                  更新时间 {{ formatTime(artifact.modified_at) }}
                </div>
              </div>
              <NTag size="small" round :bordered="false" type="success">
                {{ formatSize(artifact.size_bytes) }}
              </NTag>
            </div>

            <div class="mt-5 flex flex-wrap gap-2">
              <a :href="artifact.download_url" class="inline-flex">
                <NButton type="primary" color="#0f766e" secondary>
                  下载
                </NButton>
              </a>
              <NButton tertiary @click="handleCopyLink(artifact.download_url)">
                复制链接
              </NButton>
            </div>
          </article>
        </div>

        <div v-else class="flex items-center justify-center py-10">
          <NEmpty description="当前没有可下载的终端安装包，请先将构建产物放入挂载目录。" class="empty-shell py-10" />
        </div>
      </NCard>
    </div>
  </div>
</template>

<style scoped>
.download-hero {
  position: relative;
}

.downloads-page {
  height: 100dvh;
  max-height: 100dvh;
  overflow-x: hidden;
  overflow-y: auto;
  overscroll-behavior: contain;
  -webkit-overflow-scrolling: touch;
  padding: 1rem;
}

.downloads-page__inner {
  min-height: calc(100dvh - 2rem);
  padding-bottom: 1rem;
}

@media (min-width: 640px) {
  .downloads-page {
    padding: 1.5rem;
  }

  .downloads-page__inner {
    min-height: calc(100dvh - 3rem);
  }
}

.download-pill {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 0.45rem 0.85rem;
  background: rgba(15, 118, 110, 0.12);
  color: rgb(15 118 110);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.download-stat {
  display: inline-flex;
  flex-direction: column;
  min-width: 132px;
  gap: 0.35rem;
  padding: 0.85rem 1rem;
  border: 1px solid rgba(15, 118, 110, 0.08);
  border-radius: 16px;
  background: rgba(255, 255, 255, 0.82);
}

.download-stat-label {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: rgba(71, 98, 122, 0.78);
}

.download-stat-value {
  font-size: 0.95rem;
  font-weight: 700;
  color: #16202c;
  word-break: break-all;
}

.download-highlight {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(15, 118, 110, 0.12);
  border-radius: 24px;
  padding: 1.4rem;
  background:
    radial-gradient(circle at top right, rgba(15, 118, 110, 0.16), transparent 44%),
    linear-gradient(180deg, rgba(247, 252, 251, 0.98), rgba(236, 245, 244, 0.92));
}

.certificate-panel {
  align-items: stretch;
}

.certificate-hero {
  position: relative;
  overflow: hidden;
  border: 1px solid rgba(14, 116, 144, 0.12);
  border-radius: 24px;
  padding: 1.4rem;
  background:
    radial-gradient(circle at top right, rgba(14, 116, 144, 0.16), transparent 42%),
    linear-gradient(180deg, rgba(245, 251, 255, 0.98), rgba(236, 246, 250, 0.94));
}

.certificate-badge {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 0.45rem 0.85rem;
  background: rgba(14, 116, 144, 0.12);
  color: rgb(14 116 144);
  font-size: 0.75rem;
  font-weight: 700;
  letter-spacing: 0.14em;
  text-transform: uppercase;
}

.certificate-meta {
  border: 1px solid rgba(14, 116, 144, 0.12);
  border-radius: 16px;
  padding: 0.95rem 1rem;
  background: rgba(255, 255, 255, 0.82);
}

.certificate-meta-label {
  font-size: 0.72rem;
  text-transform: uppercase;
  letter-spacing: 0.12em;
  color: rgba(71, 98, 122, 0.78);
}

.certificate-meta-value {
  margin-top: 0.4rem;
  font-size: 0.92rem;
  font-weight: 700;
  line-height: 1.7;
  color: #16202c;
  word-break: break-all;
}

.certificate-guide {
  display: grid;
  gap: 1rem;
}

.guide-block {
  border: 1px solid rgba(15, 118, 110, 0.08);
  border-radius: 20px;
  padding: 1rem 1.05rem;
  background: linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(244, 248, 250, 0.92));
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.04);
}

.guide-note {
  border-color: rgba(245, 158, 11, 0.18);
  background: linear-gradient(180deg, rgba(255, 251, 235, 0.98), rgba(255, 247, 237, 0.94));
}

.guide-title {
  font-size: 0.98rem;
  font-weight: 700;
  color: #16202c;
}

.guide-list {
  margin: 0.8rem 0 0;
  padding-left: 1.15rem;
  color: rgba(51, 65, 85, 0.86);
  font-size: 0.92rem;
  line-height: 1.9;
}

.guide-list.unordered {
  list-style: disc;
}

.download-card {
  border: 1px solid rgba(15, 118, 110, 0.08);
  border-radius: 20px;
  padding: 1rem;
  background:
    linear-gradient(180deg, rgba(255, 255, 255, 0.98), rgba(244, 248, 250, 0.92));
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.04);
}

.download-card-title {
  font-size: 0.98rem;
  font-weight: 700;
  color: #16202c;
  line-height: 1.6;
  word-break: break-all;
}
</style>

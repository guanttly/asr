<script setup lang="ts">
import { onMounted, ref, watch } from 'vue'
import { invoke } from '@tauri-apps/api/core'
import { useSettings } from '@/composables/useSettings'
import { useAudioRecorder } from '@/composables/useAudioRecorder'
import { useVoiceControl } from '@/composables/useVoiceControl'
import { useAppStore, type SceneMode } from '@/stores/app'
import { ensureAnonymousLogin, ensureRealtimeWorkflowBinding, getCurrentUser, getMachineIdentity, pingServer, updateProfile, type MachineIdentity } from '@/utils/auth'
import { appendRuntimeLog, debugLog, getRuntimeLogPath, readRuntimeLogTail } from '@/utils/debug'
import { DEFAULT_SERVER_URL, normalizeServerUrl } from '@/utils/server'

const appStore = useAppStore()
const { settings, reset } = useSettings()
const recorder = useAudioRecorder()
const voiceControl = useVoiceControl()
const machineIdentity = ref<MachineIdentity | null>(null)
const runtimeLogPath = ref('')
const runtimeLogTail = ref('')

const authLoading = ref(false)
const authMessage = ref('')
const authMessageType = ref<'success' | 'error' | 'info'>('info')

function setSceneMode(mode: SceneMode) {
  if (appStore.sceneMode === mode) return
  appStore.sceneMode = mode
  void debugLog('settings.scene', 'scene mode changed', { mode })
}

async function refreshVoiceControl() {
  await voiceControl.ensureLoaded(true)
}

function setAuthMessage(type: 'success' | 'error' | 'info', message: string) {
  authMessageType.value = type
  authMessage.value = message
}

async function syncIdentityAndLogin(forceLogin = false) {
  authLoading.value = true
  try {
    await debugLog('settings.auth', 'starting identity sync', { forceLogin, serverUrl: appStore.serverUrl })
    appStore.serverUrl = normalizeServerUrl(appStore.serverUrl)
    machineIdentity.value = await getMachineIdentity()
    if (forceLogin || !appStore.token.trim())
      await ensureAnonymousLogin(forceLogin)
    else
      await getCurrentUser().catch(async () => await ensureAnonymousLogin(true))
    await ensureRealtimeWorkflowBinding(true)
    setAuthMessage('success', '服务连接正常，已完成匿名登录')
    await debugLog('settings.auth', 'identity sync completed', { username: appStore.username, machineCode: appStore.machineCode, realtimeWorkflowId: appStore.realtimeWorkflowId })
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '匿名登录失败')
    void debugLog('settings.error', 'identity sync failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function verifyServer() {
  authLoading.value = true
  try {
    await pingServer()
    setAuthMessage('success', `服务可达，当前地址 ${normalizeServerUrl(appStore.serverUrl)}`)
    await debugLog('settings.server', 'server health check passed', { serverUrl: appStore.serverUrl })
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '服务校验失败')
    void debugLog('settings.error', 'server health check failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function saveAlias() {
  authLoading.value = true
  try {
    const alias = appStore.deviceAlias.trim()
    if (!alias) {
      setAuthMessage('error', '请先输入设备别名')
      return
    }
    await updateProfile(alias)
    setAuthMessage('success', '设备别名已保存')
    await debugLog('settings.profile', 'device alias updated', { alias })
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '保存别名失败')
    void debugLog('settings.error', 'device alias update failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function requestMicrophonePermission() {
  authLoading.value = true
  try {
    await recorder.requestPermission()
    setAuthMessage('success', '麦克风授权已完成，之后点击开始将不再弹出首次授权提示')
    await debugLog('settings.audio', 'microphone permission initialized')
  }
  catch (error) {
    setAuthMessage('error', error instanceof Error ? error.message : '麦克风授权失败')
    void debugLog('settings.error', 'microphone permission init failed', error instanceof Error ? { message: error.message, stack: error.stack } : error)
  }
  finally {
    authLoading.value = false
  }
}

async function refreshRuntimeLogs() {
  runtimeLogPath.value = await getRuntimeLogPath().catch(() => '')
  runtimeLogTail.value = await readRuntimeLogTail(120).catch(() => '')
}

async function copyRuntimeLogs() {
  const content = runtimeLogTail.value.trim()
  if (!content) {
    setAuthMessage('info', '当前还没有可复制的调试日志')
    return
  }

  await navigator.clipboard.writeText(content)
  setAuthMessage('success', '最近调试日志已复制到剪贴板')
}

watch(() => appStore.debugLoggingEnabled, async (enabled) => {
  if (enabled) {
    await appendRuntimeLog('settings.debug', 'debug logging enabled from settings')
    await refreshRuntimeLogs()
  }
  else {
    await appendRuntimeLog('settings.debug', 'debug logging disabled from settings')
  }
})

onMounted(() => {
  void syncIdentityAndLogin(false)
  void refreshRuntimeLogs()
  void voiceControl.ensureLoaded()
})
</script>

<template>
  <div class="settings-panel">
    <section class="settings-section">
      <h4 class="section-title">连接与身份</h4>
      <p class="section-hint">打包时可通过环境变量 VITE_DEFAULT_SERVER_URL 覆盖默认服务器地址。当前默认地址：{{ DEFAULT_SERVER_URL }}</p>
      <div class="field">
        <label>服务器地址</label>
        <input
          v-model="appStore.serverUrl"
          type="text"
          :placeholder="DEFAULT_SERVER_URL"
          spellcheck="false"
        >
      </div>
      <div class="field">
        <label>设备别名</label>
        <input
          v-model="appStore.deviceAlias"
          type="text"
          placeholder="例如：张医生诊室电脑"
          spellcheck="false"
        >
      </div>
      <div class="field action-row">
        <button class="action-btn primary" :disabled="authLoading" @click="syncIdentityAndLogin(true)">连接并登录</button>
        <button class="action-btn" :disabled="authLoading" @click="verifyServer">校验服务</button>
      </div>
      <div class="field action-row">
        <button class="action-btn" :disabled="authLoading" @click="saveAlias">保存别名</button>
      </div>
      <div class="identity-grid">
        <div class="identity-card">
          <span class="identity-label">当前账号</span>
          <strong>{{ appStore.displayName || appStore.username || '未登录' }}</strong>
        </div>
        <div class="identity-card">
          <span class="identity-label">机器码</span>
          <strong class="machine-code">{{ appStore.machineCode || '读取中...' }}</strong>
        </div>
      </div>
      <div v-if="machineIdentity" class="identity-meta">
        <span>{{ machineIdentity.hostname || '未知主机' }}</span>
        <span>{{ machineIdentity.platform }}</span>
        <span>{{ machineIdentity.ip_addresses.join(' / ') || '无可用 IP' }}</span>
      </div>
      <p v-if="authMessage" class="auth-message" :class="authMessageType">
        {{ authMessage }}
      </p>
    </section>

    <section class="settings-section">
      <h4 class="section-title">录入行为</h4>
      <p class="section-hint">推荐使用 Ctrl+Shift+Space 开始和停止录音，这样更容易保持外部应用的输入焦点，自动注入也更稳定。</p>
      <div class="field row permission-row">
        <div>
          <label class="status-label">麦克风权限</label>
          <p class="permission-hint">{{ appStore.microphonePermissionGranted ? '已可直接开始录音' : '可选：预先检测并初始化麦克风权限' }}</p>
        </div>
        <button class="action-btn primary compact" :disabled="authLoading" @click="requestMicrophonePermission">
          {{ appStore.microphonePermissionGranted ? '重新检测' : '检测麦克风' }}
        </button>
      </div>
      <div class="field row">
        <label>自动注入到光标</label>
        <label class="toggle">
          <input v-model="appStore.autoInject" type="checkbox">
          <span class="toggle-slider" />
        </label>
      </div>
      <div class="field row">
        <label>保留标点符号</label>
        <label class="toggle">
          <input v-model="settings.keepPunctuation" type="checkbox">
          <span class="toggle-slider" />
        </label>
      </div>
    </section>

    <section class="settings-section">
      <h4 class="section-title">使用场景</h4>
      <p class="section-hint">报告模式：录音结束仅保存为实时转写历史；会议模式：录音结束自动创建会议纪要任务。终端语音控制可在录音中切换两种场景。</p>
      <div class="scene-segmented" role="tablist">
        <button
          type="button"
          class="scene-btn report"
          :class="{ active: appStore.sceneMode === 'report' }"
          @click="setSceneMode('report')"
        >
          <span class="scene-dot" /> 报告模式
          <span class="scene-tag">实时</span>
        </button>
        <button
          type="button"
          class="scene-btn meeting"
          :class="{ active: appStore.sceneMode === 'meeting' }"
          @click="setSceneMode('meeting')"
        >
          <span class="scene-dot" /> 会议模式
          <span class="scene-tag">纪要</span>
        </button>
      </div>
    </section>

    <section class="settings-section">
      <h4 class="section-title">终端语音控制</h4>
      <p class="section-hint">桌面端会把每段转写文本发送给当前绑定的语音控制工作流，由后台统一执行 voice_wake 与 voice_intent 节点；命中后，小球会进入“等待指令”状态（蓝色）。</p>
      <div class="voice-card">
        <div class="voice-row">
          <span class="voice-label">等待时长</span>
          <span>{{ Math.round(appStore.voiceControl.commandTimeoutMs / 1000) }} 秒</span>
        </div>
        <div class="voice-row">
          <span class="voice-label">语音控制</span>
          <span :class="appStore.voiceControl.enabled ? 'voice-on' : 'voice-off'">{{ appStore.voiceControl.enabled ? '已启用' : '已关闭' }}</span>
        </div>
      </div>
      <div class="field action-row">
        <button class="action-btn" :disabled="authLoading" @click="refreshVoiceControl">刷新语音控制配置</button>
      </div>
      <p class="section-hint">实际唤醒词、同音词归一化和命令命中结果，均以后台执行当前绑定工作流返回的结果为准。这里仅展示全局运行状态和等待时长。</p>
    </section>

    <section class="settings-section">
      <h4 class="section-title">调试</h4>
      <p class="section-hint">打开后会把前端关键事件、录音、上传和窗口启动信息追加到本地日志文件，便于排查 Windows 打包运行问题。</p>
      <div class="field row">
        <label>启用详细调试日志</label>
        <label class="toggle">
          <input v-model="appStore.debugLoggingEnabled" type="checkbox">
          <span class="toggle-slider" />
        </label>
      </div>
      <div class="field action-row">
        <button class="action-btn" @click="refreshRuntimeLogs">刷新日志</button>
        <button class="action-btn" @click="copyRuntimeLogs">复制日志</button>
        <button class="action-btn" @click="invoke('open_devtools').catch(() => undefined)">打开控制台</button>
      </div>
      <p class="section-hint">日志文件：{{ runtimeLogPath || '读取中...' }}</p>
      <textarea class="debug-log" readonly :value="runtimeLogTail" placeholder="最近日志会显示在这里" />
    </section>

    <!-- VAD Parameters -->
    <section class="settings-section">
      <h4 class="section-title">语音检测参数</h4>
      <div class="field">
        <label>最小语音阈值 ({{ settings.minSpeechThreshold.toFixed(3) }})</label>
        <input
          v-model.number="settings.minSpeechThreshold"
          type="range"
          min="0.005"
          max="0.08"
          step="0.001"
        >
      </div>
      <div class="field">
        <label>噪声门开倍数 ({{ settings.noiseGateMultiplier.toFixed(1) }})</label>
        <input
          v-model.number="settings.noiseGateMultiplier"
          type="range"
          min="1.2"
          max="6"
          step="0.1"
        >
      </div>
      <div class="field">
        <label>句末静音块数 ({{ settings.endSilenceChunks }})</label>
        <input
          v-model.number="settings.endSilenceChunks"
          type="range"
          min="2"
          max="10"
          step="1"
        >
      </div>
      <div class="field">
        <label>最小有效语音块数 ({{ settings.minEffectiveSpeechChunks }})</label>
        <input
          v-model.number="settings.minEffectiveSpeechChunks"
          type="range"
          min="1"
          max="6"
          step="1"
        >
      </div>
      <div class="field">
        <label>单块峰值倍数 ({{ settings.singleChunkPeakMultiplier.toFixed(2) }})</label>
        <input
          v-model.number="settings.singleChunkPeakMultiplier"
          type="range"
          min="1"
          max="3"
          step="0.05"
        >
      </div>
      <button class="reset-btn" @click="reset">恢复默认参数</button>
    </section>
  </div>
</template>

<style scoped>
.settings-panel {
  padding: 8px;
}

.settings-section {
  margin-bottom: 16px;
}

.section-title {
  font-size: 12px;
  font-weight: 600;
  color: #435266;
  margin-bottom: 8px;
  padding: 0 4px;
}

.section-hint {
  padding: 0 4px;
  margin-bottom: 8px;
  font-size: 11px;
  line-height: 1.5;
  color: #64748b;
}

.field {
  margin-bottom: 8px;
  padding: 0 4px;
}

.identity-grid {
  display: grid;
  gap: 8px;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  padding: 0 4px;
}

.identity-card {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 10px;
  border-radius: 10px;
  background: rgba(15, 118, 110, 0.05);
  border: 1px solid rgba(15, 118, 110, 0.08);
}

.identity-label {
  font-size: 11px;
  color: #64748b;
}

.machine-code {
  font-size: 11px;
  line-height: 1.5;
  word-break: break-all;
}

.identity-meta {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 8px 4px 0;
  font-size: 11px;
  line-height: 1.5;
  color: #64748b;
}

.field label {
  display: block;
  font-size: 11px;
  color: #64748b;
  margin-bottom: 4px;
}

.field.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
}

.field.row label:first-child {
  margin-bottom: 0;
}

.permission-row {
  align-items: center;
  gap: 12px;
}

.status-label {
  margin-bottom: 2px;
}

.permission-hint {
  font-size: 11px;
  line-height: 1.4;
  color: #64748b;
}

.field input[type="text"],
.field input[type="password"] {
  width: 100%;
  padding: 6px 8px;
  font-size: 12px;
  border: 1px solid rgba(0, 0, 0, 0.1);
  border-radius: 6px;
  background: rgba(0, 0, 0, 0.02);
  outline: none;
  color: #16202c;
  transition: border-color 0.15s;
}

.field input[type="text"]:focus,
.field input[type="password"]:focus {
  border-color: #0f766e;
}

.field input[type="range"] {
  width: 100%;
  accent-color: #0f766e;
}

.action-row {
  display: flex;
  gap: 8px;
}

.action-btn {
  flex: 1;
  padding: 7px 8px;
  font-size: 12px;
  color: #435266;
  background: rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.08);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
}

.action-btn:hover:not(:disabled) {
  background: rgba(0, 0, 0, 0.06);
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.action-btn.primary {
  color: white;
  background: #0f766e;
  border-color: #0f766e;
}

.action-btn.primary:hover:not(:disabled) {
  background: #0d5f59;
}

.action-btn.compact {
  flex: 0 0 auto;
  min-width: 92px;
}

.auth-message {
  padding: 0 4px;
  margin-top: 8px;
  font-size: 11px;
  line-height: 1.5;
}

.auth-message.success {
  color: #0f766e;
}

.auth-message.error {
  color: #dc2626;
}

.auth-message.info {
  color: #64748b;
}

/* Toggle switch */
.toggle {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 20px;
  flex-shrink: 0;
}

.toggle input {
  opacity: 0;
  width: 0;
  height: 0;
}

.toggle-slider {
  position: absolute;
  cursor: pointer;
  top: 0; left: 0; right: 0; bottom: 0;
  background: #cbd5e1;
  border-radius: 10px;
  transition: 0.2s;
}

.toggle-slider::before {
  content: '';
  position: absolute;
  width: 16px;
  height: 16px;
  left: 2px;
  bottom: 2px;
  background: white;
  border-radius: 50%;
  transition: 0.2s;
}

.toggle input:checked + .toggle-slider {
  background: #0f766e;
}

.toggle input:checked + .toggle-slider::before {
  transform: translateX(16px);
}

.reset-btn {
  width: 100%;
  padding: 6px;
  font-size: 12px;
  color: #64748b;
  background: rgba(0, 0, 0, 0.03);
  border: 1px solid rgba(0, 0, 0, 0.06);
  border-radius: 6px;
  cursor: pointer;
  transition: all 0.15s;
  margin-top: 4px;
}

.reset-btn:hover {
  background: rgba(0, 0, 0, 0.06);
}

.debug-log {
  width: 100%;
  min-height: 180px;
  padding: 10px;
  font-size: 11px;
  line-height: 1.5;
  color: #16202c;
  background: rgba(15, 23, 42, 0.04);
  border: 1px solid rgba(148, 163, 184, 0.2);
  border-radius: 10px;
  resize: vertical;
}

/* Scene mode segmented control */
.scene-segmented {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 8px;
  padding: 0 4px;
}

.scene-btn {
  display: flex;
  align-items: center;
  gap: 8px;
  padding: 12px 14px;
  border-radius: 12px;
  border: 1.5px solid rgba(148, 163, 184, 0.35);
  background: rgba(255, 255, 255, 0.6);
  color: #475569;
  font-size: 13px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.18s ease;
  text-align: left;
}

.scene-btn:hover { background: rgba(255, 255, 255, 0.9); }

.scene-btn .scene-dot {
  width: 10px;
  height: 10px;
  border-radius: 50%;
  background: currentColor;
  opacity: 0.6;
}

.scene-btn .scene-tag {
  margin-left: auto;
  font-size: 11px;
  opacity: 0.7;
}

.scene-btn.report.active {
  color: #0f766e;
  border-color: #0f766e;
  background: rgba(15, 118, 110, 0.08);
  box-shadow: 0 4px 12px rgba(15, 118, 110, 0.15);
}

.scene-btn.meeting.active {
  color: #b45309;
  border-color: #d97706;
  background: rgba(217, 119, 6, 0.08);
  box-shadow: 0 4px 12px rgba(217, 119, 6, 0.18);
}

/* Voice control read-only card */
.voice-card {
  margin: 0 4px;
  padding: 12px 14px;
  border-radius: 12px;
  background: rgba(37, 99, 235, 0.06);
  border: 1px solid rgba(37, 99, 235, 0.18);
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.voice-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  font-size: 12px;
  color: #1f2937;
}

.voice-label {
  color: #64748b;
}

.voice-wake {
  font-size: 14px;
  letter-spacing: 0.5px;
  color: #1d4ed8;
}

.voice-on { color: #0f766e; }
.voice-off { color: #94a3b8; }
</style>

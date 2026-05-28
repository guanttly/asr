export type UseCaseCoverageKind = 'unit' | 'source' | 'manual' | 'issue' | 'unclassified'

export interface DesktopUseCaseRow {
  用例编号: string
  用例标题: string
  所属模块?: string
}

export interface DesktopUseCaseAssessment {
  id: string
  title: string
  kind: UseCaseCoverageKind
  area: string
  conclusion: string
  marker: 'OK' | 'MANUAL' | 'ISSUE' | 'TODO'
  sourceRefs: string[]
}

interface CoverageRule {
  area: string
  kind: UseCaseCoverageKind
  marker: DesktopUseCaseAssessment['marker']
  patterns: RegExp[]
  conclusion: string
  sourceRefs: string[]
}

const ISSUE_OVERRIDES: Record<string, Omit<DesktopUseCaseAssessment, 'id' | 'title'>> = {
  121310: {
    kind: 'unit',
    area: '麦克风热插拔',
    marker: 'OK',
    conclusion: '录音器已监听 MediaStreamTrack ended 和 mediaDevices devicechange；录音中断时悬浮球提示“麦克风已断开，等待重新接入”，并进入 MIC 未检测到状态；设备恢复后自动重建采集流继续录音，错误映射有单测。真实热插拔仍需桌面手测。',
    sourceRefs: ['desktop/src/composables/useAudioRecorder.ts', 'desktop/src/components/MicButton.vue', 'desktop/src/stores/app.ts', 'desktop/src/composables/useAudioRecorder.test.ts'],
  },
  121311: {
    kind: 'unit',
    area: '麦克风热插拔',
    marker: 'OK',
    conclusion: '录音中麦克风断开后保留录音会话和 chunk 回调，devicechange 检测到音频输入恢复时自动重新 getUserMedia 并恢复采集；悬浮球 MIC 未检测到状态已接入。真实设备热插拔仍需桌面手测。',
    sourceRefs: ['desktop/src/composables/useAudioRecorder.ts', 'desktop/src/components/MicButton.vue', 'desktop/src/stores/app.ts', 'desktop/src/composables/useAudioRecorder.test.ts'],
  },
  121326: {
    kind: 'unit',
    area: '配置同步失败',
    marker: 'OK',
    conclusion: '当前客户端配置同步点为本地持久化和原生热键注册；原生同步失败时明确提示“已保留本地热键配置，请点击重新同步重试”，不会清空本地当前值，提示文案和 payload 行为有单测。',
    sourceRefs: ['desktop/src/stores/app.ts', 'desktop/src/components/SettingsPanel.vue', 'desktop/src/utils/hotkeys.ts', 'desktop/src/utils/hotkeys.test.ts'],
  },
  121327: {
    kind: 'unit',
    area: '配置同步失败',
    marker: 'OK',
    conclusion: '当前客户端配置同步点为本地持久化和原生热键注册；原生同步失败时设置页提示保留本地配置并引导点击“重新同步”重试，提示文案有单测覆盖。',
    sourceRefs: ['desktop/src/components/SettingsPanel.vue', 'desktop/src/composables/useDesktopHotkeys.ts', 'desktop/src/utils/hotkeys.ts', 'desktop/src/utils/hotkeys.test.ts'],
  },
}

const RULES: CoverageRule[] = [
  {
    area: '长期运行/性能/系统环境',
    kind: 'manual',
    marker: 'MANUAL',
    patterns: [/大量历史|连续运行|时间调|调后|系统语言|滚动流畅|不卡顿|24小时|72小时/],
    conclusion: '源码仅能核对分页、排序展示和 UTF-8 文本渲染入口；性能、长稳、系统时间和系统语言需要真实桌面环境手测。',
    sourceRefs: ['desktop/src/components/HistoryList.vue', 'desktop/src/components/MeetingsList.vue', 'desktop/src/components/SettingsWindow.vue'],
  },
  {
    area: '悬浮球/窗口交互',
    kind: 'manual',
    marker: 'MANUAL',
    patterns: [/悬浮球|浮层|右键|拖动|齿轮入口|控制台|日志内容|调试日志|查看日志|刷新-查看日志|日志文本/],
    conclusion: '源码已核对窗口、拖拽、右键设置和调试日志调用点；窗口位置、渲染和原生控制台行为需要桌面手测。',
    sourceRefs: ['desktop/src/components/RecorderWindow.vue', 'desktop/src/components/SettingsPanel.vue', 'desktop/src/utils/debug.ts'],
  },
  {
    area: '连接/匿名登录/JWT',
    kind: 'unit',
    marker: 'OK',
    patterns: [/服务地址|地址配置|地址|非法URL|HTTPS|HTTP|http:\/\/|192\.168|健康检查|匿名登录|登录成功|登录接口超时|401|JWT|重登录|服务可达|服务不可达/],
    conclusion: '地址归一化、协议候选、健康检查提示、匿名登录、Authorization 请求头和 401 后清旧令牌重登重试均有单测；真实网络超时/端口不可达仍需环境手测。',
    sourceRefs: ['desktop/src/utils/settingsValidation.ts', 'desktop/src/utils/server.ts', 'desktop/src/utils/auth.ts', 'desktop/src/components/SettingsPanel.vue'],
  },
  {
    area: '设备别名',
    kind: 'unit',
    marker: 'OK',
    patterns: [/别名/],
    conclusion: '别名空值、非法字符、128/129 字符边界、保存成功和失败回退已接入校验；服务端资料持久化需联调手测。',
    sourceRefs: ['desktop/src/utils/settingsValidation.ts', 'desktop/src/components/SettingsPanel.vue', 'desktop/src/utils/auth.ts'],
  },
  {
    area: '全局热键',
    kind: 'unit',
    marker: 'OK',
    patterns: [/热键|Alt\+Shift|Ctrl\+Shift|清空单个热键|恢复默认|系统冲突|注册失败|手动同步/],
    conclusion: '默认组合、清空、冲突检测和原生注册 payload 有单测；系统级注册成败与快捷键响应需桌面手测。',
    sourceRefs: ['desktop/src/utils/hotkeys.ts', 'desktop/src/composables/useDesktopHotkeys.ts', 'desktop/src/composables/useHotkeyActions.ts'],
  },
  {
    area: '麦克风/录音入口',
    kind: 'unit',
    marker: 'OK',
    patterns: [/麦克风|授权|录音文件达200MB|录音中再次点击|左键点击悬浮球|系统拒绝授权|音频提交完成/],
    conclusion: '权限拒绝、无设备、设备占用错误映射有单测；真实权限弹窗、音频采集、200MB 连续录音与提交链路需桌面手测。',
    sourceRefs: ['desktop/src/composables/useAudioRecorder.ts', 'desktop/src/components/MicButton.vue', 'desktop/src/composables/useTranscribe.ts'],
  },
  {
    area: '实时转写/工作流/注入',
    kind: 'source',
    marker: 'MANUAL',
    patterns: [/报告模式录音|短句识别|工作流执行失败|工作流绑定为空|自动注入|注入完成|录音[0-9.]+秒|两次结果不串扰|多个热键|创建实时历史任务|不创建实时历史任务|不维持流式会话/],
    conclusion: '源码已核对 VAD 分段、短句 HTTP 请求、工作流失败回退、自动注入失败保留历史、1s/5s 与空文本守卫；音频和焦点注入需手测。',
    sourceRefs: ['desktop/src/composables/useTranscribe.ts', 'desktop/src/composables/useInjector.ts'],
  },
  {
    area: 'VAD 参数',
    kind: 'unit',
    marker: 'OK',
    patterns: [/VAD|阈值|底噪|静音块数|有效语音块数|极大值10000|负数阈值|零底噪倍数/],
    conclusion: 'VAD 参数归一化和上下限兜底有单测；对后续真实录音效果生效需要录音手测。',
    sourceRefs: ['desktop/src/utils/settingsValidation.ts', 'desktop/src/composables/useSettings.ts', 'desktop/src/composables/useTranscribe.ts'],
  },
  {
    area: '语音控制/场景切换',
    kind: 'source',
    marker: 'MANUAL',
    patterns: [/语音控制|唤醒词|命令模式|指令|未命中唤醒词|分类失败|分类超时|未绑定工作流|等待指令|连续3次失败|第2次失败|计数器/],
    conclusion: '源码已核对语音控制能力开关、命令模式吞段、三次失败退出、超时恢复和场景切换；ASR/工作流分类结果需端到端手测。',
    sourceRefs: ['desktop/src/composables/useVoiceControl.ts', 'desktop/src/composables/useHotkeyActions.ts', 'desktop/src/utils/voiceControl.ts'],
  },
  {
    area: '转写历史/词库收录',
    kind: 'unit',
    marker: 'OK',
    patterns: [/转写记录|历史|清空|收录术语|收录接口失败|词库|复制|删除接口失败-确认|取消删除|确认删除|点击删除|注入失败|选中历史文本|列表加载完成|滚动到列表底部|历史加载失败|历史仅1条/],
    conclusion: '历史分页、final_text 优先、删除、清空和 realtime task API 契约有单测；复制、注入、词库弹窗和失败提示需界面手测。',
    sourceRefs: ['desktop/src/components/HistoryList.vue', 'desktop/src/utils/transcription.ts', 'desktop/src/components/DictPickerDialog.vue'],
  },
  {
    area: '会议纪要/会议列表/PDF',
    kind: 'unit',
    marker: 'OK',
    patterns: [/会议|摘要|逐字稿|PDF|导出|搜索关键字|分页|标题|重新生成|会议能力|会议模式|报告切到会议|会议切到报告|列表加载失败|编辑完成-保存|修改设置并保存|工作流绑定|新场景/],
    conclusion: '会议分页、详情更新、删除失败、摘要重生成 API 契约有单测；会议能力 gating、PDF 动态库/保存对话框、编辑器和轮询需桌面手测。',
    sourceRefs: ['desktop/src/utils/meetings.ts', 'desktop/src/components/MeetingsList.vue', 'desktop/src/components/MeetingDetail.vue', 'desktop/src/components/SettingsWindow.vue'],
  },
]

export function assessDesktopUseCase(row: DesktopUseCaseRow): DesktopUseCaseAssessment {
  const id = row.用例编号
  const title = row.用例标题
  const override = ISSUE_OVERRIDES[id]
  if (override)
    return { id, title, ...override }

  const rule = RULES.find(item => item.patterns.some(pattern => pattern.test(title)))
  if (rule) {
    return {
      id,
      title,
      kind: rule.kind,
      area: rule.area,
      conclusion: rule.conclusion,
      marker: rule.marker,
      sourceRefs: rule.sourceRefs,
    }
  }

  return {
    id,
    title,
    kind: 'unclassified',
    area: '未分类',
    marker: 'TODO',
    conclusion: '未匹配到源码检查规则，需要补充测试或人工判断。',
    sourceRefs: [],
  }
}
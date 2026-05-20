import type { Page, Route } from '@playwright/test'

const now = '2026-05-20T10:00:00+08:00'

const workflows = [
  {
    id: 1,
    name: '实时转写默认流程',
    description: 'E2E realtime workflow',
    workflow_type: 'realtime',
    source_kind: 'realtime_asr',
    target_kind: 'transcript',
    is_legacy: false,
    owner_type: 'system',
    owner_id: 1,
    is_published: true,
    nodes: [
      { id: 101, node_type: 'realtime_asr', label: '实时 ASR', position: 1, enabled: true, is_fixed: true, config: {} },
      { id: 102, node_type: 'term_correction', label: '术语纠错', position: 2, enabled: true, is_fixed: false, config: { dict_id: 1 } },
    ],
    created_at: now,
    updated_at: now,
  },
  {
    id: 2,
    name: '批量转写默认流程',
    description: 'E2E batch workflow',
    workflow_type: 'batch',
    source_kind: 'batch_asr',
    target_kind: 'transcript',
    is_legacy: false,
    owner_type: 'user',
    owner_id: 1,
    is_published: true,
    nodes: [
      { id: 201, node_type: 'batch_asr', label: '批量 ASR', position: 1, enabled: true, is_fixed: true, config: {} },
      { id: 202, node_type: 'llm_correction', label: 'LLM 校对', position: 2, enabled: true, is_fixed: false, config: { model: 'qwen' } },
    ],
    created_at: now,
    updated_at: now,
  },
  {
    id: 3,
    name: '会议纪要默认流程',
    description: 'E2E meeting workflow',
    workflow_type: 'meeting',
    source_kind: 'batch_asr',
    target_kind: 'meeting_summary',
    is_legacy: false,
    owner_type: 'system',
    owner_id: 1,
    is_published: true,
    nodes: [
      { id: 301, node_type: 'speaker_diarize', label: '说话人识别', position: 1, enabled: true, is_fixed: false, config: {} },
      { id: 302, node_type: 'meeting_summary', label: '会议纪要', position: 2, enabled: true, is_fixed: true, config: {} },
    ],
    created_at: now,
    updated_at: now,
  },
  {
    id: 4,
    name: '语音控制默认流程',
    description: 'E2E voice control workflow',
    workflow_type: 'voice_control',
    source_kind: 'voice_wake',
    target_kind: 'voice_command',
    is_legacy: false,
    owner_type: 'system',
    owner_id: 1,
    is_published: true,
    nodes: [
      { id: 401, node_type: 'voice_wake', label: '唤醒词', position: 1, enabled: true, is_fixed: true, config: {} },
      { id: 402, node_type: 'voice_intent', label: '指令识别', position: 2, enabled: true, is_fixed: true, config: {} },
    ],
    created_at: now,
    updated_at: now,
  },
]

const nodeTypes = [
  { type: 'term_correction', label: '术语纠错', role: 'transform', description: '按术语库纠错', default_config: { dict_id: 1 } },
  { type: 'filler_filter', label: '语气词过滤', role: 'transform', description: '过滤语气词', default_config: { dict_id: 1 } },
  { type: 'sensitive_filter', label: '敏感词过滤', role: 'transform', description: '替换敏感词', default_config: { dict_id: 1, replacement: '[已过滤]' } },
  { type: 'llm_correction', label: 'LLM 校对', role: 'transform', description: '大模型文本校对', default_config: { model: 'qwen', temperature: 0.1 } },
  { type: 'speaker_diarize', label: '说话人识别', role: 'transform', description: '会议说话人分离', default_config: {} },
  { type: 'meeting_summary', label: '会议纪要', role: 'sink', description: '生成会议纪要', default_config: {} },
  { type: 'voice_wake', label: '唤醒词', role: 'source', description: '识别唤醒词', default_config: {} },
  { type: 'voice_intent', label: '控制指令识别', role: 'sink', description: '识别控制指令', default_config: { include_base: true } },
]

const transcriptionTask = {
  id: 8801,
  type: 'batch',
  status: 'completed',
  external_task_id: 'mock-task-8801',
  progress_percent: 100,
  progress_stage: 'completed',
  segment_total: 1,
  segment_completed: 1,
  audio_url: 'https://example.com/audio/report.wav',
  post_process_status: 'completed',
  result_text: '患者肺部小结节，建议随访。',
  duration: 36,
  workflow_id: 2,
  created_at: now,
  updated_at: now,
}

const execution = {
  id: 9901,
  workflow_id: 2,
  trigger_type: 'batch_asr_completed',
  final_text: '患者肺部小结节，建议随访。',
  status: 'completed',
  created_at: now,
  node_results: [
    { id: 1, node_type: 'term_correction', label: '术语纠错', position: 1, input_text: '患者肺部小结节', output_text: '患者肺部小结节', status: 'completed', duration_ms: 18 },
  ],
}

function response(data: unknown, message = 'ok') {
  return { code: 0, message, data }
}

async function fulfill(route: Route, data: unknown, status = 200) {
  await route.fulfill({
    status,
    contentType: 'application/json; charset=utf-8',
    body: JSON.stringify(response(data)),
  })
}

function workflowList(searchParams: URLSearchParams) {
  const scope = searchParams.get('scope')
  const workflowType = searchParams.get('workflow_type')
  let items = workflows
  if (scope)
    items = items.filter(item => item.owner_type === scope)
  if (workflowType)
    items = items.filter(item => item.workflow_type === workflowType)
  return { items, total: items.length }
}

function catalogTree() {
  return {
    items: [
      {
        name: 'radiology',
        path: 'radiology',
        title: '影像科',
        is_dir: true,
        children: [
          { name: 'README.md', path: 'radiology/README.md', title: '总览', is_dir: false, total_terms: 2, total_rules: 2 },
        ],
      },
    ],
    source: 'mock',
  }
}

async function handleAPI(route: Route) {
  const request = route.request()
  const url = new URL(request.url())
  const path = url.pathname
  const method = request.method()

  if (method === 'POST' && path === '/api/admin/auth/login')
    return fulfill(route, { token: 'mock-token' })
  if (method === 'GET' && path === '/api/admin/me')
    return fulfill(route, { id: 1, username: 'admin', displayName: '系统管理员', role: 'admin' })
  if (method === 'GET' && path === '/api/admin/app-settings/product-features') {
    return fulfill(route, {
      edition: 'advanced',
      capabilities: { realtime: true, batch: true, meeting: true, voiceprint: true, voice_control: true },
      supported_languages: [{ code: 'auto', label: '自动识别（中英混合）' }],
      hardware_tier: 'advanced',
      hardware_requirements: {
        standard: { tier: 'standard', minimum: { cpu: '8 核', memory: '16 GB', storage: '200 GB', acceleration: 'RTX 3090' }, recommended: { cpu: '16 核', memory: '32 GB', storage: '500 GB', acceleration: 'A10' } },
        advanced: { tier: 'advanced', minimum: { cpu: '16 核', memory: '32 GB', storage: '500 GB', acceleration: 'A10' }, recommended: { cpu: '32 核', memory: '64 GB', storage: '1 TB', acceleration: 'A100' } },
      },
    })
  }
  if (path === '/api/admin/me/workflow-bindings')
    return fulfill(route, { realtime: 1, batch: 2, meeting: 3, voice_control: 4 })
  if (path === '/api/admin/app-settings/voice-control')
    return fulfill(route, { enabled: true, command_timeout_ms: 3000 })

  if (path === '/api/admin/dashboard/overview') {
    return fulfill(route, {
      pending_count: 1,
      processing_count: 2,
      completed_count: 3,
      failed_count: 0,
      post_process_pending_count: 0,
      post_process_processing_count: 1,
      post_process_completed_count: 3,
      post_process_failed_count: 0,
      repeated_failure_count: 0,
      latest_sync_at: now,
      retry_history: [],
      alerts: [],
    })
  }
  if (path.startsWith('/api/admin/dashboard/'))
    return fulfill(route, { updated: 0, failed: 0, items: [] })

  if (path === '/api/asr/tasks' && method === 'GET')
    return fulfill(route, { items: [transcriptionTask], total: 1 })
  if (path === '/api/asr/tasks' && method === 'POST')
    return fulfill(route, { ...transcriptionTask, id: 8802, audio_url: 'https://example.com/audio/new.wav' })
  if (path === '/api/asr/tasks/upload')
    return fulfill(route, { ...transcriptionTask, id: 8803 })
  if (/^\/api\/asr\/tasks\/\d+$/.test(path))
    return fulfill(route, transcriptionTask)
  if (/^\/api\/asr\/tasks\/\d+\/executions$/.test(path))
    return fulfill(route, [execution])
  if (/^\/api\/asr\/tasks\/\d+\/(?:sync|resume-post-process)$/.test(path))
    return fulfill(route, transcriptionTask)
  if (path === '/api/asr/realtime-segments')
    return fulfill(route, { text: '实时识别文本' })
  if (path === '/api/asr/realtime-tasks/upload')
    return fulfill(route, transcriptionTask)

  if (path === '/api/admin/workflows' && method === 'GET')
    return fulfill(route, workflowList(url.searchParams))
  if (path === '/api/admin/workflows' && method === 'POST')
    return fulfill(route, { ...workflows[1], id: 99, name: 'E2E 新建工作流' })
  if (path === '/api/admin/workflows/node-types')
    return fulfill(route, nodeTypes)
  if (/^\/api\/admin\/workflows\/\d+$/.test(path)) {
    const id = Number(path.split('/').pop())
    return fulfill(route, workflows.find(item => item.id === id) || workflows[1])
  }
  if (/^\/api\/admin\/workflows\/\d+\/clone$/.test(path))
    return fulfill(route, { ...workflows[1], id: 100, name: '克隆工作流' })
  if (/^\/api\/admin\/workflows\/\d+\/nodes$/.test(path))
    return fulfill(route, { ...workflows[1], nodes: workflows[1].nodes })
  if (/^\/api\/admin\/workflows\/\d+\/execute$/.test(path) || path === '/api/admin/workflows/test-node')
    return fulfill(route, { status: 'completed', final_text: '工作流执行结果', node_results: execution.node_results })
  if (/^\/api\/admin\/workflows\/node-defaults\//.test(path))
    return fulfill(route, {})
  if (/^\/api\/admin\/workflow-executions\/\d+$/.test(path))
    return fulfill(route, execution)

  if (path === '/api/admin/term-dicts')
    return fulfill(route, { items: [{ id: 1, name: '影像术语', domain: 'radiology' }], total: 1 })
  if (/^\/api\/admin\/term-dicts\/\d+\/entries$/.test(path))
    return fulfill(route, [{ id: 1, correct_term: '肺结节', wrong_variants: ['费结节'] }])
  if (/^\/api\/admin\/term-dicts\/\d+\/rules$/.test(path))
    return fulfill(route, [{ id: 1, match_type: 'regex', pattern: '血压(\\d+)/(\\d+)', replacement: '血压$1-$2', enabled: true, sort_order: 100 }])
  if (path.startsWith('/api/admin/term-dicts/'))
    return fulfill(route, { id: 1, imported: 1, skipped: 0, deleted: 1 })
  if (path === '/api/admin/term-dicts/import-template')
    return fulfill(route, {})

  if (path === '/api/admin/sensitive-dicts')
    return fulfill(route, { items: [{ id: 1, name: '基础敏感词', scene: 'base', description: '默认库', is_base: true }], total: 1 })
  if (/^\/api\/admin\/sensitive-dicts\/\d+\/entries$/.test(path))
    return fulfill(route, [{ id: 1, word: '测试敏感词', enabled: true }])
  if (path.startsWith('/api/admin/sensitive-dicts/'))
    return fulfill(route, { id: 1 })

  if (path === '/api/admin/filler-dicts')
    return fulfill(route, { items: [{ id: 1, name: '基础语气词', scene: 'base', description: '默认库', is_base: true }], total: 1 })
  if (/^\/api\/admin\/filler-dicts\/\d+\/entries$/.test(path))
    return fulfill(route, [{ id: 1, word: '嗯', enabled: true }])
  if (path.startsWith('/api/admin/filler-dicts/'))
    return fulfill(route, { id: 1 })

  if (path === '/api/admin/voice-command-dicts')
    return fulfill(route, { items: [{ id: 1, name: '基础控制指令', group_key: 'base', description: '默认库', is_base: true }], total: 1 })
  if (/^\/api\/admin\/voice-command-dicts\/\d+\/entries$/.test(path))
    return fulfill(route, [{ id: 1, intent: 'save_report', label: '保存报告', utterances: ['保存报告'], enabled: true, sort_order: 100 }])
  if (path.startsWith('/api/admin/voice-command-dicts/'))
    return fulfill(route, { id: 1 })

  if (path === '/api/meetings')
    return fulfill(route, { items: [{ id: 1, title: '科室晨会', status: 'completed', duration: 120, audio_url: 'https://example.com/meeting.wav', summary: '会议摘要', created_at: now, updated_at: now }], total: 1 })
  if (path === '/api/meetings/upload')
    return fulfill(route, { meeting: { id: 2, title: '上传会议', status: 'processing' } })
  if (/^\/api\/meetings\/\d+$/.test(path))
    return fulfill(route, { id: 1, title: '科室晨会', status: 'completed', duration: 120, audio_url: 'https://example.com/meeting.wav', transcript: '张三：讨论报告模板。', segments: [{ speaker: '张三', text: '讨论报告模板。', start: 0, end: 10 }], summary: '会议摘要', workflow_id: 3, created_at: now, updated_at: now })
  if (/^\/api\/meetings\/\d+\/summary$/.test(path))
    return fulfill(route, { summary: '会议摘要已更新' })
  if (path === '/api/meetings/voiceprints')
    return fulfill(route, { service_url: 'http://speaker:9853', items: [{ id: 'vp-1', speaker_name: '张三', department: '影像科', notes: '主任', audio_duration: 8, created_at: now }] })
  if (path.startsWith('/api/meetings/voiceprints/'))
    return fulfill(route, {})

  if (path === '/api/admin/term-catalog/tree' || path === '/api/admin/rules-catalog/tree')
    return fulfill(route, catalogTree())
  if (path === '/api/admin/term-catalog/file')
    return fulfill(route, { path: 'radiology/README.md', name: 'README.md', title: '影像术语总览', markdown_body: '# 影像术语总览', terms: [{ key: 'lung', standard_term: '肺结节', english_or_abbr: 'nodule', pinyin: 'fei jie jie', mixed_score: 1, rare_score: 1, glyph_score: 1, level: 'L1', common_misrecs: ['费结节'], notes: '', subsection_title: '胸部', source_path: 'radiology/README.md' }] })
  if (path === '/api/admin/rules-catalog/file')
    return fulfill(route, { path: 'radiology/README.md', name: 'README.md', title: '影像规则总览', markdown_body: '# 影像规则总览', rules: [{ key: 'bp', category: '格式', pattern: '血压(\\d+)/(\\d+)', replacement: '血压$1-$2', match_type: 'regex', priority: 100, conflict_group: 'bp', enabled: true, example: '血压120/80', notes: '', subsection_title: '生命体征', source_path: 'radiology/README.md' }] })
  if (path.startsWith('/api/admin/term-catalog/') || path.startsWith('/api/admin/rules-catalog/'))
    return fulfill(route, { imported: 1, dict_id: 1 })

  if (path === '/api/admin/users')
    return fulfill(route, { items: [{ id: 1, username: 'admin', displayName: '系统管理员', role: 'admin', created_at: now }], total: 1 })
  if (path === '/api/admin/public/downloads')
    return fulfill(route, { items: [{ name: 'asr-desktop-win10.exe', size_bytes: 1024, modified_at: now, download_url: '/downloads/asr-desktop-win10.exe', platform: 'win10+' }] })
  if (path === '/api/admin/openplatform/capabilities')
    return fulfill(route, { items: [{ id: 'transcription.create', display_name: '提交转写', description: '创建转写任务' }] })
  if (path === '/api/admin/openapi/docs')
    return fulfill(route, { format: 'markdown', content: '# OpenAPI\n\nMock docs', capabilities: [{ id: 'transcription.create', display_name: '提交转写', description: '创建转写任务' }] })
  if (path === '/api/admin/openplatform/apps')
    return fulfill(route, { items: [{ id: 1, app_id: 'app_mock', name: 'HIS 对接', description: '测试应用', secret_hint: 'sec***', secret_version: 1, status: 'active', rate_limit_per_sec: 10, allowed_caps: ['transcription.create'], default_workflows: { batch: 2 }, callback_whitelist: [], created_at: now, updated_at: now }], total: 1 })
  if (/^\/api\/admin\/openplatform\/apps\/\d+\/calls$/.test(path))
    return fulfill(route, { items: [{ id: 1, request_id: 'req-1', capability: 'transcription.create', route: '/openapi/v1/transcriptions', http_status: 200, latency_ms: 30, created_at: now }], total: 1 })
  if (path.startsWith('/api/admin/openplatform/apps/'))
    return fulfill(route, { id: 1, app_id: 'app_mock', app_secret: 'secret_once', name: 'HIS 对接', status: 'active', allowed_caps: ['transcription.create'], rate_limit_per_sec: 10, secret_version: 2 })

  return fulfill(route, {})
}

export async function mockFrontendAPI(page: Page) {
  await page.route('**/*', async (route) => {
    const path = new URL(route.request().url()).pathname
    if (path.startsWith('/api/')) {
      await handleAPI(route)
      return
    }
    await route.fallback()
  })
}

export async function loginByStorage(page: Page) {
  await page.addInitScript(() => {
    window.localStorage.setItem('asr_token', 'mock-token')
  })
}

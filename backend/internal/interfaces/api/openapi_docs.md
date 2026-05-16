# OpenAPI 对接指南

本文档随后端二进制一同发布，与 `/openapi/v1/*` 实际路由保持一致。所有接口以 `OpenEnvelope` 包装返回，错误时 `code` 字段为字符串错误码。

## 鉴权

### 换取 access_token

```
POST /openapi/v1/auth/token
Content-Type: application/json

{
  "app_id": "YOUR_APP_ID",
  "app_secret": "YOUR_APP_SECRET"
}
```

成功响应：

```json
{
  "code": 0,
  "message": "ok",
  "data": {
    "access_token": "<JWT>",
    "expires_in": 7200,
    "token_type": "Bearer",
    "allowed_caps": ["asr.recognize", "nlp.correct"]
  }
}
```

### 携带 token

业务接口在以下任一方式中携带 token 即可，token 会绑定 `app_id`、能力、`secret_version` 与过期时间。

- `Authorization: Bearer <access_token>`（推荐）
- Query 参数 `access_token=<token>`（仅 WebSocket / 浏览器直连场景使用）

token 失效原因可通过响应体的错误码区分：`ERR_OPEN_AUTH_EXPIRED`、`ERR_OPEN_AUTH_REVOKED`、`ERR_OPEN_AUTH_INVALID`。

## 响应格式

成功：

```json
{ "code": 0, "message": "ok", "data": { ... } }
```

失败：

```json
{ "code": "ERR_OPEN_AUTH_INVALID", "message": "invalid access token" }
```

每条业务响应里都会带上 `data.request_id`，用于追踪审计日志。

## Callback 与签名

- 异步类接口（`recognize`、`recognize/vad`、`tasks`、`audio-summary`）在请求体携带 `callback_url` 时，后端会在任务完成后向该 URL `POST` 回调
- Skill 命中也会向 Skill 的 `callback_url` 发起回调
- 回调请求附带的 Header：
  - `Content-Type: application/json`
  - `X-OpenAPI-Signature: hmac-sha256=<hex>`：签名值 = `HMAC_SHA256(app_secret, raw_body)` 的十六进制
  - `X-OpenAPI-Request-Id`：与原始请求一致
- 历史应用如果创建于本次能力之前，需要先执行一次 `rotate-secret`，否则没有签名材料
- 回调目标必须命中应用的 callback 白名单前缀；启用 `skill.invoke` 能力时必须配置白名单

## 能力：语音转文字（asr.recognize）

适用于短音频同步识别、VAD 切分识别、长音频异步任务。所有接口均为 `multipart/form-data`。

公共表单参数：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `file` | file | 待识别的音频，最大尺寸由部署侧 `upload.max_audio_size_mb` 控制 |
| `language` | string | 可选，`auto` / `zh` / `en` 等，缺省 `auto` |
| `use_itn` | bool | 可选，是否启用反向文本归一化（数字、日期等） |
| `hotwords` | string | 可选，逗号或换行分隔的热词列表 |
| `workflow_id` | string | 可选，显式指定批量转写工作流；缺省时使用应用的默认绑定 |
| `callback_url` | string | 可选，异步任务完成后回调；同步路径若同时提供，会先返回 `task_id` 再回调结果 |

### POST /openapi/v1/asr/recognize

**同步识别**。音频时长必须 ≤ 60 秒；否则返回 `ERR_AUDIO_TOO_LONG`。返回完整识别文本。

```json
{
  "request_id": "...",
  "task_id": "task_123",
  "duration_ms": 4520,
  "language": "zh",
  "text": "今天上午十点开会。",
  "segments": [
    {"start_ms": 0, "end_ms": 4520, "text": "今天上午十点开会。"}
  ],
  "workflow_id": 17,
  "workflow_origin": "app_default"
}
```

若请求体中带了 `callback_url`，本接口会改为返回任务态 `{ task_id, status, callback_url, estimated_duration_sec }`，识别完成后再向 `callback_url` 发起签名回调。

### POST /openapi/v1/asr/recognize/vad

**同步 + VAD 切分**。约束与字段同 `recognize`，但响应的 `segments` 会按上游返回的句读切分多段。段时间是按文本长度近似分配，详见“当前实现边界”。

### POST /openapi/v1/asr/tasks

**异步识别**。无 60 秒时长限制，立即返回 `task_id`。建议配合 `callback_url` 或轮询 `GET /asr/tasks/{task_id}`。

```json
{
  "request_id": "...",
  "task_id": "task_456",
  "status": "pending",
  "callback_url": "",
  "estimated_duration_sec": 312
}
```

`status` 取值：`pending` / `running` / `succeeded` / `failed`。

### GET /openapi/v1/asr/tasks/{task_id}

查询任务结果。响应结构与 `recognize` 同步成功一致；若任务尚未完成，`text` 与 `segments` 会缺省。

## 能力：实时流识别（asr.stream）

通过 PCM 分片推送的方式做流式识别，链路由「创建会话 → 推送 chunk → 事件流 → 结束」组成。

### POST /openapi/v1/asr/stream-sessions

创建会话，立即返回 chunk/commit/events/ws URL。

```json
{
  "request_id": "...",
  "session_id": "stream_xxx",
  "commit_url": "https://host/openapi/v1/asr/stream-sessions/stream_xxx/commit?access_token=...",
  "events_url": "https://host/openapi/v1/asr/stream-sessions/stream_xxx/events?access_token=...",
  "ws_url": "wss://host/openapi/v1/asr/stream-sessions/stream_xxx/events?access_token=...",
  "expires_at": "2026-01-01T00:15:00Z"
}
```

`events_url` 与 `ws_url` 自动带上当前 `access_token`，便于浏览器直接握手。`commit_url` 同样自动注入 token。

### POST /openapi/v1/asr/stream-sessions/{id}/chunks

请求体为 16-bit PCM 单声道原始字节；单次大小受部署侧上限控制。返回当前累计文本与本次 delta：

```json
{
  "request_id": "...",
  "session_id": "stream_xxx",
  "text": "今天上午开会。",
  "text_delta": "开会。",
  "is_final": false,
  "language": "zh"
}
```

### POST /openapi/v1/asr/stream-sessions/{id}/commit

强制提交当前句段，返回 `text_delta`（即本次新增的句段文本）。

### GET /openapi/v1/asr/stream-sessions/{id}/events

WebSocket 升级接口，服务端推送：

- `session.ready`
- `transcript.partial`
- `transcript.segment`
- `session.finished`
- `session.error`（含 `code` / `message`，常见 `ERR_SESSION_EXPIRED`）

### POST /openapi/v1/asr/stream-sessions/{id}/finish

显式结束会话，释放资源。返回最终累计文本。

## 能力：会议纪要（meeting.summary）

### POST /openapi/v1/meetings/audio-summary

`multipart/form-data`，字段：

| 字段 | 类型 | 说明 |
| --- | --- | --- |
| `audio_file` | file | 待识别的会议录音 |
| `title` | string | 可选，纪要标题；缺省取文件名 |
| `workflow_id` | string | 可选，会议纪要类工作流 ID |
| `callback_url` | string | 可选，异步回调 URL |

未提供 `callback_url` 时同步等待最长 5 分钟。完整响应：

```json
{
  "request_id": "...",
  "meeting_id": 88,
  "asr": {"text": "...", "duration_sec": 600.0, "language": "auto"},
  "summary": {"title": "...", "abstract": "...", "raw_text": "..."}
}
```

带 `callback_url` 时立即返回 `{ meeting_id, task_id: "mtask_<id>", status, callback_url }`，纪要完成后向回调地址发送同结构 payload（`status` 为 `completed` 时附 `data` 字段）。

### POST /openapi/v1/meetings/text-summary

`application/json`，请求 `{ "text": "..." }`，同步返回 `summary { title, abstract, raw_text }`。

### GET /openapi/v1/meetings/templates

返回当前可用纪要模板列表，目前包含 `default`。

### GET /openapi/v1/meetings/{id}

按会议 ID 查询纪要详情，结构同 `audio-summary` 完成态。

### POST /openapi/v1/meetings/{id}/regenerate-summary

请求体或 query 支持 `workflow_id`，再次执行纪要生成并返回新结果。

## 能力：文本纠错（nlp.correct）

### POST /openapi/v1/nlp/correct

```
Content-Type: application/json

{ "text": "今天天起不错" }
```

响应：

```json
{
  "request_id": "...",
  "original_text": "今天天起不错",
  "corrected_text": "今天天气不错",
  "corrections": [
    {"start": 3, "end": 5, "original": "天起", "corrected": "天气"}
  ]
}
```

## 能力：Skill 管理（skill.register）

`application/json`。

### POST /openapi/v1/skills · GET /openapi/v1/skills

创建与列举 Skill。请求体：

| 字段 | 类型 | 必填 | 说明 |
| --- | --- | --- | --- |
| `name` | string | 是 | 内部名称 |
| `display_name` | string | 是 | 展示名 |
| `description` | string | 否 | 说明 |
| `intent_patterns` | string[] | 是 | 至少一条意图正则或模板 |
| `parameters` | string | 否 | JSON Schema 字符串 |
| `callback_url` | string | 是 | 命中后回调地址，必须命中应用 callback 白名单 |
| `callback_timeout_ms` | uint32 | 否 | 缺省 5000 ms |
| `enabled` | bool | 否 | 缺省 true |

返回 `SkillResponse { skill_id, ..., consecutive_failures, last_failure_at }`。

### GET / PUT / DELETE /openapi/v1/skills/{id}

按 `skill_id` 查询、更新、删除。

### POST /openapi/v1/skills/{id}/dry-run

不发起真实回调，校验意图匹配：

```json
{ "utterance": "把灯调暗一点" }
```

响应：

```json
{
  "matched": true,
  "matched_pattern": "把(?P<target>.+)调(?P<action>.+)",
  "extracted_parameters": {"target": "灯", "action": "暗一点"},
  "would_callback": "https://partner.example.com/openapi/skill"
}
```

## 能力：Skill 回调（skill.invoke）

命中语音指令后由后端主动 `POST` 调用方的 `callback_url`，签名规则同 [Callback 与签名](#callback-与签名)。回调 body 形如：

```json
{
  "request_id": "...",
  "skill_id": "skill_xxx",
  "matched_pattern": "...",
  "parameters": {"target": "灯"},
  "utterance": "把灯调暗一点",
  "audio_meta": {"duration_ms": 1820}
}
```

- 连续 5 次失败后该 Skill 会被后端自动禁用（`enabled=false`），需要管理员或合作方调用 Skill 更新接口重新启用
- 启用本能力时**必须**在应用配置中填入 callback 白名单前缀

## 错误码

| code | HTTP | 含义 |
| --- | --- | --- |
| `ERR_VALIDATION` | 400 | 参数缺失或格式错误 |
| `ERR_OPEN_AUTH_MISSING` | 401 | 未携带 token |
| `ERR_OPEN_AUTH_INVALID` | 401 | token 非法或与当前 secret 不匹配 |
| `ERR_OPEN_AUTH_EXPIRED` | 401 | token 过期 |
| `ERR_OPEN_AUTH_REVOKED` | 401 | token 被撤销 |
| `ERR_OPEN_APP_DISABLED` | 403 | 应用被停用 |
| `ERR_OPEN_CAP_DENIED` | 403 | 应用未授权该能力 |
| `ERR_OPEN_RATE_LIMITED` | 429 | 命中应用级限流，响应附 `Retry-After` |
| `ERR_WORKFLOW_NOT_FOUND` | 400 | `workflow_id` 不存在 |
| `ERR_WORKFLOW_INVALID` | 400 | 工作流类型与能力不匹配 |
| `ERR_EDITION_LIMITED` | 403 | 当前版本未启用该能力（如会议纪要） |
| `ERR_AUDIO_TOO_LARGE` | 413 | 音频体积超限 |
| `ERR_AUDIO_TOO_LONG` | 422 | 同步识别音频超过 60 秒 |
| `ERR_UNSUPPORTED_FORMAT` | 422 | 音频格式无法解析 |
| `ERR_SESSION_EXPIRED` | 410 | 流式会话已失效 |
| `ERR_SKILL_NAME_DUPLICATED` | 409 | Skill `name` 重复 |
| `ERR_SKILL_NOT_FOUND` | 404 | Skill 不存在或不属于当前应用 |
| `ERR_SKILL_CALLBACK_UNREACHABLE` | 502 | dry-run 时回调连通性校验失败 |
| `ERR_SKILL_CALLBACK_NOT_WHITELISTED` | 400 | `callback_url` 不在应用白名单内 |
| `ERR_SKILL_DISABLED_BY_FAILURE` | 409 | Skill 因连续失败被自动停用 |
| `ERR_TEMPLATE_NOT_FOUND` | 404 | 纪要模板不存在 |
| `ERR_OPEN_INTERNAL` | 5xx | 其他后端异常 |

## Legacy 兼容接口

当部署侧 `legacy.enabled=true` 时，gateway 会开放以下旧路径，并将访问写入 `runtime/legacy-access.log`。同一组路径也提供 `/api/legacy/*` 别名。

- `POST /api/upload`
- `POST /api/recognize`
- `POST /api/recognize/vad`
- `POST /api/audio/to_summary`
- `GET /api/task/{task_id}`
- `POST /api/meeting/summary`
- `POST /api/text/correct`
- `GET /api/templates`

Legacy 响应格式（注意 `code` 是整数 `0`）：

```json
{ "success": true, "message": "", "data": {} }
```

`legacy.enabled=false` 时上述旧路径返回 `410 Gone`，gateway 仍会保留 `GET /api/health` 与 `GET /api/legacy/health` 的健康检查响应。

## 当前实现边界

- `audio-summary` 在应用未绑定会议纪要工作流时，会回退为「先转写、再即时调用 NLP 摘要」
- `recognize/vad` 的分段时间按文本长度近似分配，上游尚未暴露逐帧时间戳
- Legacy `audio/to_summary` 不附带 HMAC 签名
- OpenAPI 审计仅记录通过鉴权并进入业务处理的请求

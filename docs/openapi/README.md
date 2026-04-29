# OpenAPI 与 Legacy 兼容接口

当前版本已经通过 gateway 暴露以下对外入口。

## OpenAPI

- `POST /openapi/v1/auth/token`
  - 使用 `app_id` 和 `app_secret` 换取 `access_token`
- `POST /openapi/v1/asr/recognize`
- `POST /openapi/v1/asr/recognize/vad`
- `GET /openapi/v1/asr/tasks/:task_id`
- `POST /openapi/v1/asr/stream-sessions`
- `POST /openapi/v1/asr/stream-sessions/:id/chunks`
- `POST /openapi/v1/asr/stream-sessions/:id/commit`
- `POST /openapi/v1/asr/stream-sessions/:id/finish`
- `GET /openapi/v1/asr/stream-sessions/:id/events`
- `POST /openapi/v1/meetings/audio-summary`
- `POST /openapi/v1/meetings/text-summary`
- `GET /openapi/v1/meetings/templates`
- `GET /openapi/v1/meetings/:id`
- `POST /openapi/v1/meetings/:id/regenerate-summary`
- `POST /openapi/v1/nlp/correct`
- `POST /openapi/v1/skills`
- `GET /openapi/v1/skills`
- `GET /openapi/v1/skills/:id`
- `PUT /openapi/v1/skills/:id`
- `DELETE /openapi/v1/skills/:id`
- `POST /openapi/v1/skills/:id/dry-run`

### 鉴权

- `Authorization: Bearer <access_token>`
- 也支持 query 参数 `access_token`
- token 绑定 app、capability、secret_version 和过期时间

### 响应格式

- OpenAPI 使用 `OpenEnvelope`
- 成功时 `code = 0`
- 失败时 `code` 为字符串错误码，例如 `ERR_OPEN_AUTH_INVALID`

### Callback

- `POST /openapi/v1/asr/recognize`、`POST /openapi/v1/asr/recognize/vad`、`POST /openapi/v1/meetings/audio-summary` 在提供 `callback_url` 时，会在异步任务完成后真实发起 `POST` 回调
- 回调头会附带 `X-OpenAPI-Signature: hmac-sha256=<hex>`、`X-OpenAPI-Request-Id`
- 签名使用应用当前的 `app_secret` 计算，因此历史旧应用如果创建于本次能力之前，需要先执行一次 rotate-secret 才会补齐签名材料
- `skill.invoke` 已接入 `voice_intent` 运行时链路；命中 skill 后会立即发起签名 callback，并在连续 5 次失败后自动禁用该 skill

### Streaming

- `POST /openapi/v1/asr/stream-sessions` 现在会同时返回 `commit_url`、`events_url`、`ws_url`
- `ws_url` 与 `events_url` 会自动带上当前 `access_token` query，便于浏览器直接建立 websocket 连接
- `POST /openapi/v1/asr/stream-sessions/:id/chunks` 返回累计文本与 `text_delta`
- `POST /openapi/v1/asr/stream-sessions/:id/commit` 会提交当前句段，并返回这一次的 `text_delta`
- `GET /openapi/v1/asr/stream-sessions/:id/events` 会通过 websocket 推送 `session.ready`、`transcript.partial`、`transcript.segment`、`session.finished`

## Legacy 兼容接口

当 `legacy.enabled=true` 时，gateway 会开放以下旧路径，并将访问写入 `runtime/legacy-access.log`。

- `GET /api/health`
- `POST /api/upload`
- `POST /api/recognize`
- `POST /api/recognize/vad`
- `POST /api/audio/to_summary`
- `GET /api/task/:task_id`
- `POST /api/meeting/summary`
- `POST /api/text/correct`
- `GET /api/templates`

同一组路径也提供 `/api/legacy/*` 别名。

### 响应格式

Legacy 路由保持旧格式：

```json
{
  "success": true,
  "message": "",
  "data": {}
}
```

当 `legacy.enabled=false` 时，以上旧路径会返回 `410 Gone`。

## 当前实现边界

- OpenAPI `audio-summary` 在没有绑定工作流时，会回退为“先转写，再即时调用 NLP 生成摘要”
- OpenAPI `recognize/vad` 现在会按转写结果中的句读切分多段，并按文本长度近似分配时间；当前上游还没有暴露逐帧时间戳，因此段落时间仍是近似值
- Legacy `audio/to_summary` 支持无签名 callback，当前不会附带 HMAC 签名
- OpenAPI audit 仅记录通过鉴权并进入业务处理的请求
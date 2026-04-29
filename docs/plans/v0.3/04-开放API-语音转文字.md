# 04 — 开放 API：语音转文字

> 路径前缀：`/openapi/v1/asr`
> 鉴权：Bearer access_token（OpenAuth）
> 必须的能力：`asr.recognize`（同步 / 异步），`asr.stream`（流式）

本能力对应平台 `asr-api` 的 `recognize_audio` / `recognize_audio_with_vad_segmentation` / 流式接口；对接方可在三种调用模式之间选择，并可选附加工作流。

## 1. 能力总览

| 模式 | 路径 | 适用场景 |
|---|---|---|
| 1.1 同步识别 | `POST /openapi/v1/asr/recognize` | 短音频（≤ 60 s），等响应即拿结果 |
| 1.2 VAD 分段同步识别 | `POST /openapi/v1/asr/recognize/vad` | 较长音频（≤ 5 min），希望按句返回 |
| 1.3 异步任务 | `POST /openapi/v1/asr/tasks` + `GET /openapi/v1/asr/tasks/:id` | 长音频（≤ 1 GB），后台处理 |
| 1.4 实时流 | `POST /openapi/v1/asr/stream-sessions` 等 | 实时麦克风、设备直推 |

所有模式都支持 `workflow_id`（可选），见 [07-工作流附加机制.md](07-工作流附加机制.md)。

## 2. 同步识别

### 2.1 接口

`POST /openapi/v1/asr/recognize`
Content-Type: `multipart/form-data`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `file` | file | ✅ | - | wav/mp3/m4a/flac/ogg/webm，≤ 60 s |
| `language` | string | ❌ | `auto` | `auto` / `zh` / `en` |
| `use_itn` | bool | ❌ | `true` | 是否做反向数字归一 |
| `hotwords` | string | ❌ | - | 逗号 / 分号 / 换行分隔 |
| `workflow_id` | uint64 | ❌ | 应用默认 | 不传走应用配置的默认工作流 |
| `callback_url` | string | ❌ | - | 传则改为异步：先返回 task_id，处理完回调 |

### 2.2 同步响应（无 callback）

```json
{
  "code": 0,
  "data": {
    "request_id": "req_2k3jas...",
    "duration_ms": 1820,
    "language": "zh",
    "text": "今天天气不错，我们一起出去走走。",
    "segments": [
      { "start_ms": 0, "end_ms": 1240, "text": "今天天气不错" },
      { "start_ms": 1240, "end_ms": 1820, "text": "我们一起出去走走" }
    ],
    "workflow_execution_id": 9012,
    "post_processed_text": "今天天气不错，我们一起出去走走。"
  }
}
```

`post_processed_text` 在接入工作流后才有；未启用工作流时省略此字段。

### 2.3 异步响应（带 callback）

```json
{
  "code": 0,
  "data": {
    "task_id": "task_7Hq...",
    "status": "pending",
    "callback_url": "https://your-system/asr/cb"
  }
}
```

回调请求示例（平台 → 三方）：

```http
POST https://your-system/asr/cb
Content-Type: application/json
X-OpenAPI-Signature: hmac-sha256=...

{
  "request_id": "req_...",
  "task_id": "task_7Hq...",
  "status": "succeeded",
  "data": { /* 与同步响应 data 字段相同 */ }
}
```

回调签名规则：`HMAC-SHA256(app_secret, raw_body)`，三方校验通过才接受。失败时平台按指数退避（1 / 5 / 30 分钟）重试 3 次。

## 3. VAD 分段同步识别

`POST /openapi/v1/asr/recognize/vad`

在 2.1 基础上额外支持：

| 字段 | 类型 | 默认 | 说明 |
|---|---|---|---|
| `min_segment_duration` | float | 1.0 | 秒 |
| `max_segment_duration` | float | 30.0 | 秒 |

响应字段同 2.2，`segments` 长度反映 VAD 的切分结果。

## 4. 异步任务

适合 1 GB 以内长音频。

### 4.1 创建任务

`POST /openapi/v1/asr/tasks`（multipart）

| 字段 | 必填 | 说明 |
|---|---|---|
| `file` | ✅ | 音频文件 |
| `language` `use_itn` `hotwords` `workflow_id` | ❌ | 同 2.1 |
| `callback_url` | ❌ | 完成后回调 |

响应：

```json
{
  "code": 0,
  "data": {
    "task_id": "task_7Hq...",
    "status": "pending",
    "estimated_duration_sec": 320
  }
}
```

### 4.2 查询任务

`GET /openapi/v1/asr/tasks/:task_id`

```json
{
  "code": 0,
  "data": {
    "task_id": "task_7Hq...",
    "status": "running" | "succeeded" | "failed" | "cancelled",
    "progress": 0.42,
    "data": { /* 完成后才有，结构同 2.2 data */ },
    "error": { "code": "ERR_ASR_DECODE", "message": "..." } | null
  }
}
```

### 4.3 取消任务

`DELETE /openapi/v1/asr/tasks/:task_id`

仅 `pending` / `running` 可取消。

## 5. 实时流

### 5.1 创建会话

`POST /openapi/v1/asr/stream-sessions`

```json
{ "language": "zh", "use_itn": true, "workflow_id": null }
```

响应：

```json
{
  "code": 0,
  "data": {
    "session_id": "sess_...",
    "ws_url": "wss://<host>/openapi/v1/asr/stream-sessions/sess_.../events",
    "expires_at": "2026-04-28T09:30:00Z"
  }
}
```

### 5.2 推送音频帧

`POST /openapi/v1/asr/stream-sessions/:session_id/chunks`
Content-Type: `application/octet-stream`
Body：16 kHz / 16 bit / mono PCM，单帧 ≤ 64 KB，建议 100~500 ms 一帧。

### 5.3 结束

`POST /openapi/v1/asr/stream-sessions/:session_id/finish`

返回最终聚合文本。

### 5.4 WebSocket 事件流

订阅 `ws_url`，帧格式：

```json
{
  "type": "asr.partial" | "asr.final" | "session.closed",
  "session_id": "sess_...",
  "payload": { "text": "...", "is_final": false },
  "ts": "2026-04-28T09:25:13.221Z"
}
```

WebSocket 鉴权：`?access_token=<token>` 或 `Authorization` 头（gateway 同时支持）。

## 6. 错误码

| code | HTTP | 含义 |
|---|---|---|
| `ERR_VALIDATION` | 400 | 字段缺失 / 格式错 |
| `ERR_UNSUPPORTED_FORMAT` | 415 | 文件类型不支持 |
| `ERR_AUDIO_TOO_LARGE` | 413 | 超过 1 GB |
| `ERR_AUDIO_TOO_LONG` | 422 | 同步接口收到 > 60 s 音频 |
| `ERR_ASR_ENGINE_UNAVAILABLE` | 503 | 后端 ASR 引擎不可达 |
| `ERR_WORKFLOW_NOT_FOUND` | 404 | 指定 workflow_id 不存在 / 不属于该应用 |
| `ERR_SESSION_EXPIRED` | 410 | 流式会话过期 |

## 7. SDK 片段

### 7.1 cURL

```bash
curl -X POST https://<host>/openapi/v1/asr/recognize \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -F "file=@example.wav" \
  -F "language=zh" \
  -F "workflow_id=12"
```

### 7.2 Python（aiohttp）

```python
async with aiohttp.ClientSession() as s:
    form = aiohttp.FormData()
    form.add_field("file", open("example.wav","rb"), filename="example.wav")
    form.add_field("language", "zh")
    form.add_field("workflow_id", "12")
    async with s.post(f"{HOST}/openapi/v1/asr/recognize",
                      data=form,
                      headers={"Authorization": f"Bearer {token}"}) as r:
        result = await r.json()
        print(result["data"]["text"])
```

### 7.3 Go（标准库）

```go
req, _ := http.NewRequest("POST", host+"/openapi/v1/asr/recognize", body)
req.Header.Set("Authorization", "Bearer "+token)
req.Header.Set("Content-Type", writer.FormDataContentType())
resp, err := http.DefaultClient.Do(req)
```

## 8. 性能与配额（v0.3 默认）

| 项目 | 默认值 |
|---|---|
| 同步接口超时 | 60 s |
| VAD 同步接口超时 | 5 min |
| 异步任务最大体积 | 1 GB |
| 单应用并发流式会话 | 5 |
| 单应用 QPS | 30 req/s（可后台调） |

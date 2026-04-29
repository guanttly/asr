# 05 — 开放 API：会议纪要

> 路径前缀：`/openapi/v1/meetings`
> 鉴权：Bearer access_token（OpenAuth）
> 必须的能力：`meeting.summary`
> 高级版限定：当目标部署为标准版时，所有路径返回 `403 ERR_EDITION_LIMITED`。

会议纪要能力封装：上传音频 → 自动语音识别 → LLM 摘要 → 可选说话人分离。底层复用 asr-api 中的 `meetingService` 与工作流引擎。

## 1. 能力总览

| 路径 | 用途 |
|---|---|
| `POST /openapi/v1/meetings/audio-summary` | 一步完成：上传音频 → 拿纪要 |
| `POST /openapi/v1/meetings/text-summary` | 已有转写文本 → 直接生成纪要 |
| `GET /openapi/v1/meetings/templates` | 列出可用纪要模板 |
| `GET /openapi/v1/meetings/:id` | 查询会议详情（含纪要、转写、说话人） |
| `POST /openapi/v1/meetings/:id/regenerate-summary` | 重生成纪要 |

## 2. 一站式：音频 → 纪要

`POST /openapi/v1/meetings/audio-summary`
Content-Type: `multipart/form-data`

| 字段 | 类型 | 必填 | 默认 | 说明 |
|---|---|---|---|---|
| `audio_file` | file | ✅ | - | 同语音转文字支持的格式 |
| `template` | string | ❌ | `default` | 模板名（见 `/templates`） |
| `enable_correction` | bool | ❌ | `true` | LLM 文本纠错 |
| `enable_speaker` | bool | ❌ | `false` | 说话人分离（高级版） |
| `language` | string | ❌ | `auto` | |
| `use_itn` | bool | ❌ | `true` | |
| `variables` | json | ❌ | `{}` | 模板自定义变量 |
| `workflow_id` | uint64 | ❌ | 应用默认 | 不传走默认；工作流必须包含 `meeting_summary` 节点 |
| `callback_url` | string | ❌ | - | 异步模式 |

### 2.1 同步响应

```json
{
  "code": 0,
  "data": {
    "request_id": "req_...",
    "meeting_id": 1024,
    "asr": {
      "text": "...",
      "duration_sec": 1820,
      "language": "zh"
    },
    "summary": {
      "title": "Q2 复盘会",
      "abstract": "...",
      "agenda": [...],
      "todos": [
        { "owner": "张三", "task": "本周内提交 v0.3 评审稿", "due": "2026-05-04" }
      ],
      "decisions": ["..."],
      "raw_text": "<原始 markdown 纪要>"
    },
    "speakers": [
      { "label": "Speaker_1", "name": "张三", "duration_sec": 612.3 }
    ],
    "processing_time": {
      "asr_sec": 78.2,
      "llm_sec": 132.1,
      "total_sec": 210.3
    }
  }
}
```

### 2.2 异步响应（带 callback）

```json
{
  "code": 0,
  "data": {
    "task_id": "mtask_...",
    "meeting_id": 1024,
    "status": "pending",
    "callback_url": "https://your-system/cb"
  }
}
```

回调载荷与同步响应 `data` 等价，外加 `task_id`、`status` 字段；签名规则同 [04-...](04-开放API-语音转文字.md#23-异步响应带-callback)。

## 3. 文本 → 纪要

`POST /openapi/v1/meetings/text-summary`
Content-Type: `application/json`

```json
{
  "text": "<已有的会议转写文字>",
  "template": "default",
  "variables": { "meeting_title": "Q2 复盘" },
  "workflow_id": null
}
```

响应字段是 2.1 中 `summary` 段的等价输出（无 `asr` / `speakers`）。

## 4. 模板列表

`GET /openapi/v1/meetings/templates`

```json
{
  "code": 0,
  "data": {
    "default_template": "default",
    "templates": [
      {
        "name": "default",
        "display_name": "通用纪要",
        "variables": ["meeting_title"]
      },
      {
        "name": "medical_handover",
        "display_name": "医疗交接班",
        "variables": ["ward", "shift"]
      }
    ]
  }
}
```

## 5. 会议详情与重生成

| 路径 | 用途 |
|---|---|
| `GET /openapi/v1/meetings/:id` | 取已有纪要、转写、说话人；返回字段同 2.1 |
| `POST /openapi/v1/meetings/:id/regenerate-summary` | 仅重跑摘要，不改原始转写 |

仅当 `meeting_id` 由该应用创建时可访问；越权返回 404。

## 6. 错误码

| code | HTTP | 含义 |
|---|---|---|
| `ERR_VALIDATION` | 400 | 字段错 |
| `ERR_AUDIO_TOO_LARGE` | 413 | > 1 GB |
| `ERR_TEMPLATE_NOT_FOUND` | 404 | 模板名错 |
| `ERR_WORKFLOW_INVALID` | 422 | 工作流不含 `meeting_summary` 节点 |
| `ERR_EDITION_LIMITED` | 403 | 标准版部署调用了高级版能力 |
| `ERR_ASR_PARTIAL` | 422 | ASR 成功但 LLM 失败，data 含 `asr` 与 `error` |
| `ERR_LLM_ENGINE_UNAVAILABLE` | 503 | nlp-api 不可达 |

`ERR_ASR_PARTIAL` 对应 `old http.py` 中的 `partial_success`，保留语义以便联调。

## 7. SDK 片段

```bash
curl -X POST https://<host>/openapi/v1/meetings/audio-summary \
  -H "Authorization: Bearer $TOKEN" \
  -F "audio_file=@meeting.mp3" \
  -F "template=default" \
  -F "enable_speaker=true" \
  -F "callback_url=https://your-system/cb"
```

```python
async with aiohttp.ClientSession() as s:
    form = aiohttp.FormData()
    form.add_field("audio_file", open("meeting.mp3", "rb"), filename="meeting.mp3")
    form.add_field("template", "default")
    form.add_field("enable_speaker", "true")
    async with s.post(f"{HOST}/openapi/v1/meetings/audio-summary",
                      data=form,
                      headers={"Authorization": f"Bearer {token}"}) as r:
        body = await r.json()
        print(body["data"]["summary"]["abstract"])
```

## 8. 性能与配额

| 项目 | 默认值 |
|---|---|
| 同步接口超时 | 30 min（30 min 录音端到端） |
| 异步任务最大音频 | 1 GB / 4 小时 |
| 模板自定义变量上限 | 32 键 |
| 重生成接口冷却 | 同一会议 1 次 / 分钟 |

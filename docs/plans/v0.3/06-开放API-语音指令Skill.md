# 06 — 开放 API：语音指令 Skill 与回调

> 路径前缀：`/openapi/v1/skills`
> 鉴权：Bearer access_token
> 必须的能力：`skill.register`（管理 skill）+ `skill.invoke`（接收回调）
> 高级版限定：依赖 `voice_intent` 节点，标准版部署返回 `ERR_EDITION_LIMITED`。

## 1. 模型

平台引入"Skill"概念：三方系统把自己的一项能力包装成可触发单元注册到平台，并提供回调地址。当平台收到的语音指令意图命中某个 skill，就以 webhook 的形式调用三方完成实际动作。

```
[ 三方系统 ]                        [ 巨鲨平台 ]                    [ 终端用户 ]
    │  注册 skill                       │                                 │
    │  ─────────POST /skills──────────▶│                                 │
    │                                   │  voice_intent 节点中追加该 skill│
    │                                   │                                 │
    │                                   │   ◀──── 用户说：「打开会议模式」│
    │                                   │ ASR + voice_intent 命中 skill   │
    │                                   │                                 │
    │  ◀────POST <callback_url>────────│                                 │
    │  根据载荷执行业务逻辑              │                                 │
    │  ─────────回调响应──────────────▶│                                 │
    │                                   │ 把响应作为指令结果给前端 / 桌面  │
```

## 2. Skill 数据结构

| 字段 | 类型 | 必填 | 说明 |
|---|---|---|---|
| `name` | string | ✅ | 应用内唯一 |
| `display_name` | string | ✅ | 调试 UI 展示用 |
| `description` | string | ❌ | |
| `intent_patterns` | string[] | ✅ | 触发该 skill 的关键词 / 短语 / 正则；与 voice_intent 节点匹配 |
| `parameters` | object schema | ❌ | JSON Schema，描述 skill 接受的结构化参数 |
| `callback_url` | string | ✅ | HTTPS（私有化场景允许 HTTP，但需在应用 `callback_whitelist` 内） |
| `callback_timeout_ms` | int | ❌ | 默认 3000，最大 10000 |
| `enabled` | bool | ✅ | 默认 true |

## 3. 管理接口

| 方法 | 路径 | 用途 |
|---|---|---|
| `POST` | `/openapi/v1/skills` | 注册 skill |
| `GET` | `/openapi/v1/skills` | 列出当前应用的 skill |
| `GET` | `/openapi/v1/skills/:id` | 详情 |
| `PUT` | `/openapi/v1/skills/:id` | 修改 |
| `DELETE` | `/openapi/v1/skills/:id` | 注销 |
| `POST` | `/openapi/v1/skills/:id/dry-run` | 用一段示例文本试触发，便于联调 |

### 3.1 注册示例

```bash
curl -X POST https://<host>/openapi/v1/skills \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "open_meeting_mode",
    "display_name": "开启会议模式",
    "intent_patterns": ["打开会议模式", "切换到会议", "进入会议模式"],
    "parameters": {
      "type": "object",
      "properties": { "room": { "type": "string" } }
    },
    "callback_url": "https://your-system/skills/open-meeting",
    "callback_timeout_ms": 2000
  }'
```

响应：

```json
{
  "code": 0,
  "data": {
    "skill_id": "skl_8KLm...",
    "name": "open_meeting_mode",
    "intent_patterns": [...],
    "callback_url": "https://your-system/skills/open-meeting",
    "enabled": true
  }
}
```

注册时平台会发起一次 GET `<callback_url>` 探活（5 s 超时，2xx 通过；失败拒绝注册并返回 `ERR_SKILL_CALLBACK_UNREACHABLE`）。

### 3.2 dry-run

```bash
curl -X POST https://<host>/openapi/v1/skills/skl_8KLm.../dry-run \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d '{ "utterance": "请打开会议模式" }'
```

响应：

```json
{
  "code": 0,
  "data": {
    "matched": true,
    "matched_pattern": "打开会议模式",
    "extracted_parameters": {},
    "would_callback": "https://your-system/skills/open-meeting"
  }
}
```

dry-run 不真正调用三方。

## 4. 回调协议（平台 → 三方）

```http
POST <callback_url>
Content-Type: application/json
X-OpenAPI-Signature: hmac-sha256=<sig>
X-OpenAPI-Request-Id: req_xxx
X-OpenAPI-Skill-Id: skl_xxx

{
  "skill_id": "skl_8KLm...",
  "skill_name": "open_meeting_mode",
  "request_id": "req_xxx",
  "matched_pattern": "打开会议模式",
  "utterance": "请打开会议模式",
  "parameters": {},
  "context": {
    "session_id": "sess_...",
    "user_label": "Speaker_1"
  },
  "ts": "2026-04-28T09:25:00.123Z"
}
```

签名 = `HMAC-SHA256(app_secret, raw_body)`，三方需校验后才信任请求。

### 4.1 期望的响应

```json
{
  "code": 0,
  "message": "ok",
  "data": { "spoken_reply": "已为您切换到会议模式。" }
}
```

`spoken_reply` 可选；平台会把它播报给前端 / 桌面端。其余字段平台不解读。

### 4.2 失败与重试

- HTTP 非 2xx 或超时 → 失败。
- 平台**不重试**意图触发的 skill 回调（避免重复执行业务动作）。
- 失败计入调用日志；连续 5 次失败时把 skill 自动置为 `enabled=false` 并发邮件通知管理员。

## 5. 与 voice_intent 工作流节点的关系

- 平台为每个应用维护一份"语音指令字典"，由该应用注册的 skill 自动产生 / 同步条目。
- voice_intent 节点在执行时读取该字典并尝试命中。
- 已有的"语音指令字典"（FEAT-DICT-04）保留给桌面端本地命令；外部 skill 字典与之并行存在，用 `app_id` 区分。

## 6. 错误码

| code | HTTP | 含义 |
|---|---|---|
| `ERR_VALIDATION` | 400 | 字段错 |
| `ERR_SKILL_NAME_DUPLICATED` | 409 | 同应用下重名 |
| `ERR_SKILL_CALLBACK_UNREACHABLE` | 422 | 注册时探活失败 |
| `ERR_SKILL_CALLBACK_NOT_WHITELISTED` | 422 | URL 不在 callback_whitelist 内 |
| `ERR_SKILL_NOT_FOUND` | 404 | id 错或非本应用 |
| `ERR_SKILL_DISABLED_BY_FAILURE` | 423 | 被自动禁用 |

## 7. 安全注意事项

- `callback_url` 默认要求 HTTPS；HTTP 仅当应用的 `callback_whitelist` 显式包含目标地址才允许。
- 回调签名密钥 = `app_secret`，rotate-secret 后旧签名无效。
- 平台不向三方泄漏完整对话上下文，仅传 `utterance` 与命中信息。

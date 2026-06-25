# 第二轮提测 BUG 修复代码变更记录

> 适用产品：巨鲨语音助手（语音转写系统 Private LAN Edition）
> 提测基线版本：**Fama_V1.0.0_20260612**（第一轮修复后版本，见 [第一轮提测BUG修复变更记录.md](第一轮提测BUG修复变更记录.md)）
> 修复周期：**2026-06-22 ~ 2026-06-23**
> 修复版本：v0.9.2（构建 26623）
> BUG 清单来源：[bugs/第二轮测试BUG.csv](bugs/第二轮测试BUG.csv)（共 17 条）

---

## 一、概述

本轮提测共提报 **17 个 BUG**（类型均为“代码错误”，严重程度以“一般”为主，含 2 条“建议”优化项），集中在**会议纪要、实时识别与语音控制（桌面端）、工作流与节点管理、术语 / 敏感词 / 控制指令库、OpenAPI 对接**等模块。

其中 **11 条为第二轮新发现缺陷**，**6 条为第一轮“待复测 / 复活”缺陷**（14852 / 14765 / 14726 / 14691 / 14690 / 14688，详见第五节）。

修复工作由 **2 个提交**完成，时间跨度 2026-06-22 至 2026-06-23。除围绕缺陷的直接修复外，本轮重点夯实了三条主链路的健壮性：

- **会议纪要链路**：重新生成摘要改为**脱离请求连接的后台异步执行**，修正“卡在生成中”会议的失败计数与可删除判定；
- **桌面端实时 / 语音控制链路**：补齐单断句 ASR 超时兜底，新增**场景切换时旧场景会话后台落库**，使语音控制切换前后的转写记录与会议不再丢失；
- **资源删除引用校验**：把“被工作流节点引用时禁止删除并提示先解除引用”的能力从语气词库**推广到术语库 / 敏感词库 / 控制指令库**，并在前端透出后端的具体提示文案；
- **OpenAPI 流式接口**：`asr.stream` 在未配置流式上游时返回**明确的 503 与可操作文案**，不再以“配置缺失”的内部错误暴露。

本轮同步补充了后端（会议服务、ASR 服务、流式可用性判定）与桌面端（`useTranscribe`）的单元测试。

### BUG 状态分布

| 状态 | 数量 | 说明 |
| --- | --- | --- |
| 已修复（待复测） | 17 | 对应修复已在 `5789f15` / `166cc9d` 提交，CSV 导出时状态仍为“激活”，待回归复测后关闭 |

---

## 二、修复提交时间线

| 序号 | 提交 | 日期 | 提交说明 | 变更规模 |
| --- | --- | --- | --- | --- |
| 1 | `166cc9d` | 2026-06-22 | fix batch task execution summary loading | 5 文件，+175 / −115 |
| 2 | `5789f15` | 2026-06-23 | fix：修复二轮BUG | 42 文件，+932 / −238 |

---

## 三、按模块分类的 BUG 修复明细

### 1. 会议纪要

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14929 | 会议模式长时间录音有概率图标一直转圈，停止后没有会议生成 | 为单断句 ASR 请求增加超时兜底，避免有序消费协程卡死导致停止流程不返回；会议落库改以“是否有音频”为准（实时预览文本为空也生成会议） | `5789f15` |
| 14916 | 卡在“生成中”的会议纪要删除不掉、重试也无法成功 | 放开删除限制：当会议虽为 processing 但已记录同步/摘要失败时允许删除（前后端一致） | `5789f15` |
| 14912 | 总结没有失败状态、一直停在“生成中” | 修正失败计数：上游查询成功不再每轮清零累计失败次数，使收尾失败能累加到上限后落入 failed 终态 | `5789f15` |
| 14883 | 极长会议（数小时）点击“重新生成摘要”失败（疑似同步接口问题） | 重新生成摘要改为**后台异步**：同步完成校验并置为“处理中”后立即返回，真正生成在脱离请求连接、带 2h 上限的后台 goroutine 执行，客户端断连不再影响生成 | `5789f15` |
| 14690 | 摘要内容修改后会强行插入一个表头（破坏排版） | 首次插入会议信息表头时按 Markdown 段落补空行分隔，避免与正文/标题粘连；移除保存/导出前的强制元字段回写 | `5789f15` |
| 14691 | 摘要文本风格需适配大模型 Markdown 格式 | 导出 PDF 时为标题/引用块/列表/表格/加粗等逐一注入行内样式，并补全预览态表格表头/斑马纹样式，使导出与预览一致 | `5789f15` |

**关键变更文件**：`backend/internal/application/meeting/service.go`（+141）、`backend/internal/interfaces/api/meeting_handler.go`、[frontend/src/pages/meeting/index.vue](../frontend/src/pages/meeting/index.vue)、[frontend/src/pages/meeting/detail.vue](../frontend/src/pages/meeting/detail.vue)、`desktop/src/components/MeetingDetail.vue`、`desktop/src/composables/useTranscribe.ts`

### 2. 实时识别与语音控制（桌面端）

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14924 | 会议/报告模式下用语音控制切换场景，不手动停止时中间切换的转写记录与会议纪要不保存 | 新增**场景切换时旧场景会话后台落库**：切换前把累计会话快照按旧场景落库（会议→自动建会议，报告→实时任务），停止时等待全部落库完成，确保中间历史不丢 | `5789f15` |
| 14894 | 会议模式下用语音控制切换到报告模式后，报告模式的转写记录不再更新 | 同上场景切换落库机制；并明确报告模式必跑逐段实时 ASR、会议模式仅在启用语音控制时逐段识别，切换后新场景从零累计 | `5789f15` |
| 14688 | 命令模式识别文本在切换模式后被写入历史 | 残留抑制改以断句“录入时间”判断（对识别/提交延迟更鲁棒），切换前后录入的命令尾音不分类、不写历史、不注入；命令模式分类失败的断句也直接吞掉 | `5789f15` |

**关键变更文件**：`desktop/src/composables/useTranscribe.ts`（+301）、`desktop/src/composables/useVoiceControl.ts`

### 3. 工作流管理

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14899 | 预置系统工作流可被删除，删除后无法再创建工作流来绑定 | 后端禁止删除预置系统模板（owner=system 且 owner_id=0），返回 `ErrPresetWorkflowProtected`；前端对预置项的“删除”按钮置灰并加 Tooltip 说明 | `5789f15` |
| 14889 | 新增工作流时“创建归属”两个选项因组件与字体颜色问题看不见文字 | 未选中态按钮改用 `quaternary` 样式，保证未选中选项文字可见 | `5789f15` |

**关键变更文件**：`backend/internal/domain/workflow/entity.go`、`backend/internal/application/workflow/service.go`、`backend/internal/interfaces/api/workflow_handler.go`、[frontend/src/pages/workflow/index.vue](../frontend/src/pages/workflow/index.vue)

### 4. 节点管理

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14895 | 工作流唤醒词识别同样无法添加第四个同音易错词、无法换行 | 默认唤醒词 / 同音易错词的录入控件由 `NDynamicTags` 改为多行自增 `textarea`，按行录入、数量不再受限 | `5789f15` |
| 14852 | 普通版进入节点管理报错（语音控制节点未开放仍触发加载） | 工作流编辑页仅在具备“语音控制”能力时加载控制指令库选项，避免普通版因 403 弹出无关的加载失败 | `5789f15` |

**关键变更文件**：[frontend/src/pages/workflow/nodes.vue](../frontend/src/pages/workflow/nodes.vue)、[frontend/src/pages/workflow/editor.vue](../frontend/src/pages/workflow/editor.vue)

### 5. 术语库与纠错规则

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14765 | 纠错规则新增/编辑时，错误的正则表达式提示语不明显 | 弹窗内实时校验正则可编译性，输入框置错误态并显示醒目红色提示文案 | `5789f15` |
| 14726 | 词库（场景库）被节点引用时点击删除未提示“先解除引用” | 后端为术语库 / 敏感词库 / 控制指令库补齐“被工作流节点引用则禁止删除并提示先解除引用”的校验；前端统一透出后端的具体提示文案（不再吞成通用“删除失败”） | `5789f15` |

**关键变更文件**：`backend/internal/application/terminology/service.go`、`backend/internal/infrastructure/persistence/workflow_repo.go`（新增 `CountConfigDictListReferences`）、`backend/cmd/admin-api/main.go`、[frontend/src/pages/terminology/index.vue](../frontend/src/pages/terminology/index.vue)

### 6. 敏感词库 / 控制指令库

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14896 | 普通版控制指令库界面“默认附加”和“当前分组”标识叠在一起 | 类型列与操作列改为 `flex-wrap` 自适应换行，标识不再重叠 | `5789f15` |

**关键变更文件**：`backend/internal/application/sensitive/service.go`、`backend/internal/application/voicecommand/service.go`、`backend/internal/interfaces/api/voice_command_handler.go`、[frontend/src/pages/terminology/voice-commands.vue](../frontend/src/pages/terminology/voice-commands.vue)、[frontend/src/pages/terminology/sensitive.vue](../frontend/src/pages/terminology/sensitive.vue)

> 说明：术语库 / 敏感词库 / 控制指令库三者的“被引用禁止删除 + 前端透出提示”属于同一处增强（对应 14726），分别落在各自的应用服务、`admin-api` 装配与前端删除处理中。

### 7. OpenAPI 对接管理

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14907 | OpenAPI 的 `asr.stream` 接口不可用，却提示“配置缺失”的内部错误 | 新增 `ErrStreamEngineUnavailable` 与 `StreamingAvailable()` 可用性判定：未配置流式上游时返回 **503** 并给出可操作文案（改用录音文件识别 `asr.recognize`，或在服务端配置 `services.asr_stream` 后重试） | `5789f15` |

**关键变更文件**：`backend/internal/application/asr/service.go`、`backend/internal/infrastructure/asrengine/client.go`、`backend/internal/interfaces/api/asr_handler.go`、`backend/internal/interfaces/api/openapi_handler.go`、`backend/cmd/asr-api/main.go`、`backend/pkg/errcode/codes.go`（新增 `ERR_OPEN_STREAM_UNAVAILABLE`）

### 8. 批量转写（配套修复）

`166cc9d` 修复批量转写历史列表的执行状态加载：后端在任务 DTO 中直接携带 `execution_summary`，前端 `history.vue` 改为从列表数据应用执行摘要，取代逐任务的 N+1 次执行记录请求，避免任务较多时执行状态加载缓慢/缺失。

**关键变更文件**：`backend/internal/application/asr/dto.go`、`backend/internal/domain/asr/entity.go`、`backend/internal/infrastructure/persistence/asr_repo.go`、[frontend/src/pages/transcription/history.vue](../frontend/src/pages/transcription/history.vue)

---

## 四、按提交的代码变更汇总

### `166cc9d`（2026-06-22）fix batch task execution summary loading
批量转写历史执行状态加载优化：任务 DTO 直接返回 `execution_summary`，前端去除逐任务执行记录拉取。涉及 `asr/dto.go`、`domain/asr/entity.go`、`persistence/asr_repo.go`、`frontend/src/pages/transcription/history.vue` 及 E2E Mock 扩展。

### `5789f15`（2026-06-23）fix：修复二轮BUG
本轮主修复，覆盖会议异步重生成与失败计数、桌面端场景切换落库与残留抑制、工作流预置保护与节点录入控件、资源删除引用校验、OpenAPI 流式可用性提示，共 42 文件、+932 / −238：

- **后端**：`meeting/service.go`（+141，异步重生成 / 失败计数 / 可删除判定）、`asr/service.go`（流式可用性判定）、`asrengine/client.go`、`asr_handler.go`、`openapi_handler.go`、`errcode/codes.go`（流式不可用错误）、`workflow/{entity,service,handler}.go`（预置保护）、`workflow_repo.go`（`dict_ids` 引用计数）、`terminology/`、`sensitive/`、`voicecommand/` 服务的引用校验、`admin-api/main.go` 与 `asr-api/main.go` 的装配。
- **桌面端**：`useTranscribe.ts`（+301，断句超时 / 场景切换落库 / 残留抑制 / 会议按音频落库）、`useVoiceControl.ts`（切换返回旧场景、命令失败吞字）、`MeetingDetail.vue`（导出样式与表头插入）。
- **前端**：`workflow/index.vue`（预置删除置灰 + 归属按钮配色）、`workflow/nodes.vue`（唤醒词多行录入）、`workflow/editor.vue`（按能力加载控制指令库）、`terminology/index.vue`（正则错误提示）、`terminology/voice-commands.vue` / `sensitive.vue`（标识换行 + 透出后端提示）、`meeting/index.vue` / `meeting/detail.vue`（卡住会议可删除）、`transcription/history.vue`。
- **测试**：`asr/service_test.go`、`asr_handler_test.go`、`meeting/service_test.go`、`useTranscribe.test.ts`。
- **归档**：`docs/bugs/第二轮测试BUG.csv`（17 条）。

---

## 五、第一轮“待复测 / 复活”缺陷处置

下列 6 条在第二轮 CSV 中携带第一轮基线（`Fama_V1.0.0_20260529`）且“激活次数=1”，为第一轮已提交修复但复测中复活的缺陷，本轮重新修复：

| BUG | 模块 | 第二轮处置 |
| --- | --- | --- |
| 14852 | 普通版节点管理报错 | 工作流编辑页按“语音控制”能力加载控制指令库，规避普通版 403 |
| 14765 | 纠错规则正则错误提示不明显 | 弹窗内实时校验正则并置错误态显示红色提示 |
| 14726 | 词库被节点引用删除未提示先解除引用 | 后端补齐术语/敏感/控制指令库引用校验，前端透出后端提示 |
| 14691 | 摘要风格适配大模型 Markdown | 导出/预览样式对齐大模型 Markdown 元素 |
| 14690 | 摘要修改后强行插入表头 | 插入表头时补 Markdown 段落分隔，去除强制回写 |
| 14688 | 命令模式文本切换后写入历史 | 残留抑制改按录入时间判断，命令失败断句吞字 |

---

## 六、遗留与待复测

本轮 17 条 BUG 在 CSV 导出时状态均为“激活”，对应修复代码已在 `5789f15` / `166cc9d` 提交，待回归复测后统一关闭。

| 范围 | BUG 编号 |
| --- | --- |
| 第二轮新发现（11 条） | 14929、14924、14916、14912、14907、14899、14896、14895、14894、14889、14883 |
| 第一轮复活（6 条） | 14852、14765、14726、14691、14690、14688 |

---

## 七、测试与验证

- **后端单元测试**：
  - `meeting/service_test.go`：会议重新生成摘要的异步流程、失败计数累加与终态、卡住会议可删除判定（通过测试钩子 `backgroundDoneHook` 等待后台 goroutine 完成）。
  - `asr/service_test.go`、`asr_handler_test.go`：流式不可用（`ErrStreamEngineUnavailable`）路径与 503 响应。
- **桌面端单元测试**：`useTranscribe.test.ts` 覆盖断句超时跳过、场景切换落库、会议按音频落库与残留抑制等场景。
- **复测建议**：重点回归极长会议重生成、语音控制跨场景切换历史保存、普通版节点管理/工作流编辑、各类词库“被引用删除”提示与 OpenAPI `asr.stream` 未配置时的报错文案。

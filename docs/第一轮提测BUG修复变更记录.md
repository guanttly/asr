# 第一轮提测 BUG 修复代码变更记录

> 适用产品：巨鲨语音助手（语音转写系统 Private LAN Edition）
> 提测基线版本：**Fama_V1.0.0_20260529**
> 修复周期：**2026-06-03 ~ 2026-06-12**
> 修复版本：v0.9.2（构建 26612，发布说明见 [fama-221-release.md](fama-221-release.md)）
> BUG 清单来源：[bugs/第一轮测试BUG.csv](bugs/第一轮测试BUG.csv)（共 63 条）

---

## 一、概述

本轮提测共提报 **63 个 BUG**（类型均为“代码错误”，严重程度以“一般”为主，含若干“建议”优化项），覆盖数据看板、实时识别、批量转写、会议纪要、声纹库、工作流、术语/词库、用户与权限、OpenAPI 对接、桌面客户端及 ASR 识别质量等模块。

修复工作由 **10 个提交**完成，时间跨度 2026-06-03 至 2026-06-12。除围绕缺陷的直接修复外，本轮还补充了：

- **并行转写**与**会议大文件分片上传**（解决长音频与吞吐体验问题）；
- **统一的资源名称合法性校验**（术语库 / 语气词库 / 敏感词库 / 用户名 / 控制指令）；
- **术语纠错规则引擎增强**（近似词 / 正则 / 数字归一化、优先级、预览）；
- **角色权限收口**（普通用户访问管理页的前端守卫与后端校验）；
- **ASR 幻觉与重复文本抑制**；
- **回归测试**（前端 E2E `round1-bugfixes.spec.ts` 与多处 Go 单元测试）。

### BUG 状态分布

| 状态 | 数量 | 说明 |
| --- | --- | --- |
| 已解决 | 60 | 已在下述提交中修复 |
| 激活（待复测） | 3 | 编号 14852 / 14853 / 14854，对应修复已在 `0bb1ac8` / `96058fd` 提交，CSV 导出时尚未回归关闭 |

---

## 二、修复提交时间线

| 序号 | 提交 | 日期 | 提交说明 | 变更规模 |
| --- | --- | --- | --- | --- |
| 1 | `070e33e` | 2026-06-03 | fix: 修复一轮测试 BUG | 30 文件，+1049 / −101 |
| 2 | `80fa210` | 2026-06-03 | feat: 添加日志目录和访问日志路径 | 10 文件，+51 / −17 |
| 3 | `f398987` | 2026-06-04 | fix: 修复 openapi 代码 | 18 文件，+621 / −153 |
| 4 | `d6164fc` | 2026-06-05 | feat: 添加词库名称和用户名称的合法性校验 | 16 文件，+307 / −54 |
| 5 | `740249f` | 2026-06-06 | feat: 增强术语纠错规则管理 | 38 文件，+1046 / −123 |
| 6 | `96058fd` | 2026-06-08 | fix: 修复测试 BUG | 22 文件，+865 / −207 |
| 7 | `0d11681` | 2026-06-09 | fix: 修复 BUG | 22 文件，+261 / −16 |
| 8 | `84e94a2` | 2026-06-11 | fix: 修复 BUG | 21 文件，+509 / −74 |
| 9 | `0bb1ac8` | 2026-06-12 | fix: 修复 BUG，自测验证 | 15 文件，+489 / −60 |
| 10 | `cc9d3bf` | 2026-06-12 | fix: 修复幻觉问题，降低幻觉率 | 10 文件，+217 / −4 |

---

## 三、按模块分类的 BUG 修复明细

### 1. 数据看板与风险告警

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14751 | 点击同步提示同步失败（预期成功） | 修正同步接口调用与状态判定 | `96058fd` |
| 14752 | 风险告警数据多时排版错乱 | 重排风险告警布局，避免列挤压 | `96058fd` |
| 14754 | 筛选条件有匹配却显示空状态 | 修正筛选结果与空态判定逻辑 | `96058fd` |
| 14745 | 普通用户操作数据看板提示“看板加载失败”（应提示无权限或不展示） | 前端按角色不展示看板，配合后端权限返回 | `96058fd` |

**关键变更文件**：[frontend/src/pages/dashboard/index.vue](../frontend/src/pages/dashboard/index.vue)、[frontend/src/layouts/DefaultLayout.vue](../frontend/src/layouts/DefaultLayout.vue)、[frontend/src/router/index.ts](../frontend/src/router/index.ts)

### 2. 实时语音识别

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14802 | 后台实时语音识别不输出结果；大量短句快速连续输出时列表卡顿 | 修正实时输出渲染，提升连续短句渲染性能 | `070e33e` / `96058fd` |
| 14803 | 修改参数后点击“重置底噪”底噪参数未恢复初始值 | 修正重置底噪逻辑，仅恢复底噪相关项 | `0d11681` |

**关键变更文件**：[frontend/src/pages/realtime/index.vue](../frontend/src/pages/realtime/index.vue)、[frontend/src/api/asr.ts](../frontend/src/api/asr.ts)

### 3. 批量转写

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14824 | 提交错误音频 URL 任务不失败、一直处理中且无法删除 | 增加任务失败判定，失败任务可删除 | `070e33e` / `96058fd` |
| 14686 | 界面不展示处理中的任务 | 修正任务列表展示；后处理失败可继续 | `0d11681` |
| 14786 | 搜索框仅支持 ID，类型/状态无法搜索 | 扩展搜索维度（ID/类型/状态） | `0d11681` |
| 14785 | 搜索关键字超过 128 字符上限无限制 | 增加搜索输入长度限制 | `0d11681` |
| 14783 | 转写文本超长打开详情等待久 | 优化超长文本详情加载/渲染 | `070e33e` / `f398987` |
| 14776 | 转写单线程顺序执行慢，关闭后仍持续输出队列 | 实现并行转写并清理停录后的残留队列 | `070e33e` / `96058fd` |

**关键变更文件**：[frontend/src/pages/transcription/history.vue](../frontend/src/pages/transcription/history.vue)、`backend/internal/application/asr/service.go`、`desktop/src/composables/useTranscribe.ts`

### 4. 会议纪要

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14701 | 会议录音达 200MB 无分段处理，过大无法上传 | 新增会议音频**分片上传**接口与处理 | `070e33e` |
| 14835 | 失败会议的“总结失败次数”不对 | 修正会议失败次数统计 | `84e94a2` |
| 14829 | 极长会议摘要生成失败且无重试入口、自动重试不触发 | 补充失败重试机制与界面重试入口 | `84e94a2` |
| 14825 | 会议搜索框无法搜索 | 修正会议列表搜索 | `84e94a2` |
| 14692 | 过短会议无法生成纪要但错误提示不准确 | 修正过短会议的错误提示 | `84e94a2` |
| 14690 | 摘要内容修改后强行插入表头 | 修正摘要 Markdown 处理 | `070e33e` / `84e94a2` |
| 14691 | 摘要文本风格需适配大模型格式 | 调整摘要渲染风格 | `84e94a2` |

**关键变更文件**：`backend/internal/interfaces/api/meeting_chunk_upload.go`（新增）、`backend/internal/application/meeting/service.go`、`backend/internal/interfaces/api/meeting_handler.go`、[frontend/src/pages/meeting/index.vue](../frontend/src/pages/meeting/index.vue)

### 5. 声纹库

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14816 | 同名人重复注册无“已存在/覆盖策略”提示 | 增加同名声纹存在性校验与提示 | `0d11681` |

**关键变更文件**：`backend/internal/application/voiceprint/service.go`、`backend/internal/interfaces/api/voiceprint_handler.go`

### 6. 工作流管理

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14791 | 新建工作流可重名 | 增加工作流名称唯一性校验 | `0d11681` |
| 14793 | 未发布工作流在应用配置中可被选择 | 应用配置仅允许选择已发布工作流 | `0d11681` |
| 14792 | 新建时无法从系统模板重置节点 | 支持从系统模板导入/重置节点 | `0d11681` |
| 14797 | Legacy 类型多余且搜不到内容 | 移除 Legacy 场景类型 | `0d11681` |

**关键变更文件**：`backend/internal/application/workflow/service.go`、`backend/internal/infrastructure/persistence/workflow_repo.go`、[frontend/src/pages/workflow/index.vue](../frontend/src/pages/workflow/index.vue)

### 7. 节点管理

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14753 | 唤醒词识别无法添加第四个同音易错词 | 放开同音易错词数量限制 | `96058fd` |
| 14852 | 普通版进入节点管理报错（待复测） | 修正普通版节点管理页渲染与权限 | `96058fd` / `0bb1ac8` |

**关键变更文件**：[frontend/src/pages/workflow/nodes.vue](../frontend/src/pages/workflow/nodes.vue)

### 8. 术语库与纠错规则

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14747 | 标准术语已存在不提示重复 | 增加标准术语重复校验 | `d6164fc` |
| 14749 | 词库下存在词条仍直接删除成功 | 删除前校验是否存在词条 | `d6164fc` |
| 14758 | 术语库名称含非法字符 `@#$` 不提示 | 增加名称合法性校验 | `d6164fc` |
| 14838 | 选择纠错规则后左侧词典列表布局挤在一起 | 修正列表列宽自适应布局 | `84e94a2` |
| 14762 | 纠错规则含“词条替换旧”含义不明且无法编辑 | 重构纠错规则类型与编辑能力 | `740249f` |
| 14761 | 正则规则预览效果都一样 | 修正正则规则差异化预览 | `740249f` |
| 14763 | 提示正则重复但无快捷定位 | 优化重复正则定位提示 | `740249f` |
| 14764 | 执行顺序可输入负数/无限制数字 | 增加执行顺序数值范围限制 | `740249f` |
| 14765 | 正则表达式错误提示不明显 | 强化正则错误提示 | `740249f` |
| 14760 | 规则导入 Excel 无下载模板入口 | 提供导入模板下载 | `740249f` |
| 14759 | 术语词条/纠错规则导出 Excel 内容为空 | 修正 XLSX 导出数据填充 | `96058fd` |
| 14722 | 全部清空时数据多导致超时报错 | 优化清空逻辑避免超时 | `96058fd` |

**关键变更文件**：`backend/internal/infrastructure/nlpengine/corrector.go`（纠错引擎，+257 行）、`backend/internal/interfaces/api/term_handler.go`、`backend/pkg/xlsxio/xlsxio.go`、[frontend/src/pages/terminology/index.vue](../frontend/src/pages/terminology/index.vue)

### 9. 语气词库

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14732 | 语气词重复不提示已存在 | 增加语气词重复校验 | `d6164fc` |
| 14725 | 语气词库名称含非法字符不提示 | 增加名称合法性校验 | `d6164fc` |
| 14726 | 场景库被节点引用删除不提示先解除引用 | 删除前校验引用关系并提示 | `d6164fc` / `96058fd` |
| 14854 | 普通版默认叠加与当前词库标识叠在一起（待复测） | 修正语气词库标识展示 | `0bb1ac8` |

**关键变更文件**：`backend/internal/application/filler/service.go`、`backend/internal/application/filler/errors.go`、[frontend/src/pages/terminology/fillers.vue](../frontend/src/pages/terminology/fillers.vue)

### 10. 敏感词库

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14779 | 敏感词重复不提示已存在 | 增加敏感词重复校验 | `96058fd` |
| 14757 | 敏感词库名称含非法字符不提示 | 增加名称合法性校验 | `96058fd` |

**关键变更文件**：`backend/internal/application/sensitive/service.go`、`backend/internal/application/sensitive/errors.go`、[frontend/src/pages/terminology/sensitive.vue](../frontend/src/pages/terminology/sensitive.vue)

### 11. 控制指令库

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14768 | 无法新建控制指令组 | 修正指令组创建逻辑 | `96058fd` |
| 14781 | 意图值重复不提示已存在 | 增加意图值重复校验 | `96058fd` / `0d11681` |

**关键变更文件**：`backend/internal/application/voicecommand/service.go`、`backend/internal/interfaces/api/voice_command_handler.go`、[frontend/src/pages/terminology/voice-commands.vue](../frontend/src/pages/terminology/voice-commands.vue)

### 12. 用户管理与权限

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14735 | 新增用户名含 `@#` 不提示“仅中文字母数字下划线” | 增加用户名合法性校验 | `d6164fc` |
| 14748 | 普通用户可看到/操作无权限功能（看板/工作流/术语库/系统管理） | 前端按角色守卫 + 后端权限校验 | `96058fd` |

**关键变更文件**：`backend/internal/application/user/service.go`、`frontend/src/utils/resourceName.ts`（名称校验工具）、[frontend/src/pages/system/users.vue](../frontend/src/pages/system/users.vue)、[frontend/src/router/index.ts](../frontend/src/router/index.ts)

### 13. OpenAPI 对接管理

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14671 | 存在多个应用时列表不展示任何应用 | 修正应用列表查询与展示 | `f398987` |
| 14771 | OpenAPI 部分接口与文档不一致、无法访问（含回调） | 修正 legacy 接口路由与回调实现 | `f398987` |
| 14772 | 新建应用使用最大数值（如 Meta JSON 超长）创建失败 | 修正参数上限校验与提示 | `f398987` |

**关键变更文件**：`backend/internal/interfaces/api/legacy_handler.go`、`backend/internal/application/openplatform/service.go`、[frontend/src/pages/system/openapi.vue](../frontend/src/pages/system/openapi.vue)（+536 行）

### 14. 桌面客户端

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14695 | 报告模式自动锁定第一个输入窗口，影响操作 | 将自动注入/锁定改为用户可控 | `070e33e` |
| 14694 | 会议模式自动弹出光标录入界面 | 会议模式不触发光标注入 | `070e33e` |
| 14693 | 悬浮球图标占位过大，点击外围无法选中桌面 | 缩小拖动热区（中心约 45% 半径） | `070e33e` |
| 14688 | 命令模式识别文本切换模式后写入了历史 | 命令模式文本仅用于分类，不写入历史/注入 | `070e33e` |
| 14667 | 输入不支持的设备别名仍保存到用户资料 | 增加设备别名校验，非法不保存 | `070e33e` |
| 14826 | 修改总结地址后不同步到客户端 | 客户端同步会议总结服务地址配置 | `84e94a2` |
| 14784 | 生成会议纪要时无法查看逐字稿 | 客户端会议详情支持查看逐字稿 | `070e33e` / `84e94a2` |
| 14827 | 缺少开机自启开关 | 新增开机自启设置（Tauri/Electron） | `84e94a2` |

**关键变更文件**：`desktop/src/composables/useTranscribe.ts`、`desktop/src/composables/useVoiceControl.ts`、`desktop/src/utils/auth.ts`、`desktop/src/components/MeetingDetail.vue`、`desktop/src/components/SettingsPanel.vue`、`desktop/src-tauri/src/lib.rs`、`desktop-electron/electron/main/ipc.ts`

### 15. LLM 纠错与 ASR 识别质量

| BUG | 现象 | 修复说明 | 关联提交 |
| --- | --- | --- | --- |
| 14853 | 普通版 LLM 纠错节点配置后提示错误无法调用（待复测） | 修正 LLM 纠错调用并去除思考标签干扰 | `0bb1ac8` |
| 14833 | 批量转写 CER 测试出现重复文字，541 份样本平均出现约 3 次幻觉 | 增加幻觉与重复文本抑制逻辑 | `cc9d3bf` |

**关键变更文件**：`backend/internal/application/workflow/handler_llm_correction.go`、`backend/internal/application/asr/service.go`（幻觉抑制，+113 行）

---

## 四、按提交的代码变更汇总

### `070e33e`（2026-06-03）修复一轮测试 BUG
本轮规模最大的基础修复，覆盖后端 ASR 并行转写、会议分片上传、访问日志中间件，以及桌面端注入/会议模式行为。
- 后端：`asr/service.go`、`asr_handler.go`、`meeting_chunk_upload.go`（新增分片上传）、`meeting_handler.go`、`middleware/access_log.go`（新增访问日志）、`pkg/config`、`pkg/logging`。
- 桌面：`useTranscribe.ts`、`useVoiceControl.ts`、`utils/transcription.ts`、`utils/auth.ts`、`MeetingDetail.vue`、`SettingsPanel.vue`、`tauri.conf.json`。
- 前端：`api/asr.ts`、`system/openapi.vue`、`transcription/history.vue`。

### `80fa210`（2026-06-03）日志目录与访问日志路径
完善部署侧日志落盘：`deploy/jusha-asr-business/*`（Dockerfile、compose、install/uninstall/entrypoint 脚本）、`frontend/index.html` 与 favicon。属配套运维变更。

### `f398987`（2026-06-04）修复 OpenAPI 代码
重点修复对接管理：`legacy_handler.go`、`nlp/dto.go`、`user_repo.go`，前端 `system/openapi.vue`（+536 行）与 `transcription/history.vue`（+149 行）。

### `d6164fc`（2026-06-05）名称合法性校验
统一新增**词库 / 语气词库 / 用户名**等资源名称合法性校验：`filler/service.go`、`terminology/service.go`、`user/service.go`、`term_repo.go`、`workflow_repo.go`，前端 `users.vue`、`fillers.vue`、`terminology/index.vue`。

### `740249f`（2026-06-06）增强术语纠错规则管理
重写纠错引擎 `nlpengine/corrector.go`（+257 行），支持近似词/正则/数字归一化与优先级；`term_handler.go`、`rules_catalog_handler.go`、前端 `terminology/index.vue`（+359 行）、`login.vue`，并同步影像术语/规则目录文档与 XLSX。

### `96058fd`（2026-06-08）修复测试 BUG
覆盖敏感词/控制指令校验、XLSX 导出、看板与权限路由、节点管理唤醒词、桌面并行转写：`sensitive/service.go`、`voicecommand/service.go`、`xlsxio.go`、`utils/resourceName.ts`、`DefaultLayout.vue`、`dashboard/index.vue`、`nodes.vue`、`router/index.ts`、`useTranscribe.ts`（+372 行）。

### `0d11681`（2026-06-09）修复 BUG
工作流校验与声纹重名、实时重置底噪、批量搜索：`workflow/service.go`、`voiceprint/service.go`、`voicecommand/service.go`，前端 `workflow/index.vue`、`realtime/index.vue`、`transcription/history.vue`、`voice-commands.vue`。同步归档 `docs/bugs/20260609.csv`。

### `84e94a2`（2026-06-11）修复 BUG
会议失败次数/重试、会议搜索、客户端开机自启与地址同步：`asr/service.go`、`meeting/service.go`，桌面 `lib.rs`、`capabilities/desktop.json`、`Cargo.toml`、`SettingsPanel.vue`、`stores/app.ts`、Electron `ipc.ts`，前端 `meeting/index.vue`、`terminology/index.vue`，并完善部署脚本。归档 `docs/bugs/20260611.csv`。

### `0bb1ac8`（2026-06-12）修复 BUG，自测验证
LLM 纠错思考标签处理与回归测试：`workflow/handler_llm_correction.go` 及其思考标签单测、前端 `fillers.vue`/`sensitive.vue`/`nodes.vue`、新增前端 E2E `tests/e2e/round1-bugfixes.spec.ts` 与 `apiMock.ts`，合并归档 `docs/bugs/第一轮测试BUG.csv`（63 条）与发布说明 `docs/fama-221-release.md`。

### `cc9d3bf`（2026-06-12）修复幻觉问题
ASR 幻觉与重复文本抑制：`asr/service.go`（+113 行）与单测 `asr/service_test.go`，并更新影像规则目录。

---

## 五、遗留与待复测

下列 3 个 BUG 在 CSV 导出时状态仍为“激活”，对应修复代码已在上述提交中提交，待回归复测后关闭：

| BUG | 模块 | 对应修复提交 |
| --- | --- | --- |
| 14852 | 普通版节点管理进入报错 | `96058fd` / `0bb1ac8` |
| 14853 | 普通版 LLM 纠错配置后无法调用 | `0bb1ac8` |
| 14854 | 语气词库默认叠加标识重叠 | `0bb1ac8` |

---

## 六、测试与验证

- **前端 E2E 回归**：`frontend/tests/e2e/round1-bugfixes.spec.ts`（+91 行）针对本轮缺陷场景做端到端验证，配套 `tests/e2e/support/apiMock.ts` 扩展接口 Mock。
- **后端单元测试**：本轮新增/扩充多处 Go 单测，覆盖 ASR 服务、纠错引擎、敏感词/语气词/控制指令校验、工作流校验、LLM 思考标签处理等，例如：
  - `asr/service_test.go`（并行转写、幻觉抑制）
  - `nlpengine/corrector_test.go`（纠错规则）
  - `filler/service_test.go`、`sensitive/service_test.go`、`voicecommand/service_test.go`（校验）
  - `workflow/handler_llm_correction_thinking_test.go`（思考标签）
- **发布说明**：详见 [fama-221-release.md](fama-221-release.md)。

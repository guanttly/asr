# 语音实时录入输入目标绑定方案

## 1. 背景

当前应用基于以下技术栈：

```text
Rust / Tauri
Electron，兼容 Win7 场景
Vue 前端框架
```

应用已具备语音识别、剪贴板写入、快捷键触发等基础能力。

当前核心需求是：

> 用户在 RIS 报告编辑框中写报告时，可以切换到 PACS 查看影像；即使用户正在 PACS 中操作，语音识别后的文字仍然应写入用户预期绑定的 RIS 报告输入框。

该能力本质上不是 Vue、Tauri 或 Electron 的前端能力，而是 Windows 桌面环境下的输入目标锚定、窗口识别、焦点恢复和粘贴执行能力。

因此建议将该能力封装为一个独立的 Windows 原生输入桥应用。

---

## 2. 总体方案

独立开发一个 Rust 原生程序：

```text
voice-input-bridge.exe
```

该程序作为独立应用存在，可以被 Tauri 或 Electron 调用。

```text
Vue 前端
  │
  ├─ Tauri / Electron 主程序
  │
  └─ 调用 voice-input-bridge.exe
          │
          ├─ 监听当前焦点输入框
          ├─ 记录候选输入框
          ├─ 维护历史绑定目标
          ├─ 自动恢复最近可用目标
          ├─ 显示绑定 Overlay
          ├─ 执行剪贴板粘贴
          └─ 返回写入状态
```

职责划分如下：

| 模块 | 职责 |
|---|---|
| Vue | 录音按钮、识别结果展示、绑定状态展示、配置页面 |
| Tauri / Electron | 启动 sidecar、转发 ASR 文本、接收绑定状态 |
| Rust Input Bridge | 目标检测、历史恢复、Overlay 提示、粘贴执行 |
| RIS / HIS / 浏览器版 RIS | 最终文本输入目标 |

---

## 3. 核心设计原则

### 3.1 不直接依赖当前焦点

不能简单把语音结果写入当前焦点窗口，因为用户可能正在 PACS 中操作。

错误逻辑：

```text
语音识别完成
  ↓
写入当前焦点窗口
```

推荐逻辑：

```text
语音识别完成
  ↓
查找当前锁定目标
  ↓
锁定目标失效则查历史绑定目标
  ↓
历史目标不可用才使用当前焦点
```

---

### 3.2 最后聚焦框只作为候选

后台可以持续追踪最近一个有效输入框，但它只能作为候选目标。

```text
candidateTarget：最近一次有效聚焦输入框
lockedTarget：当前真正绑定的语音写入目标
```

规则：

```text
聚焦变化只更新 candidateTarget
快捷键绑定才更新 lockedTarget
语音写入只写 lockedTarget 或历史恢复目标
```

---

### 3.3 历史绑定优先

在没有明确重新绑定指令的情况下，系统总是倾向于使用用户最近一次绑定的文本框。

目标选择优先级：

```text
1. 当前 lockedTarget
2. 最近一次历史绑定目标
3. 历史绑定目标顺位恢复
4. 当前聚焦输入框
5. 无目标则提示用户重新绑定
```

---

### 3.4 可视化提示必须明确

为了避免误写，系统需要让用户清楚知道当前文本会写到哪里。

建议提供：

```text
绿色边框：表示当前绑定输入框
红色文字：提示当前语音写入目标
状态条：显示当前绑定目标
写入闪烁：提示本次写入成功
```

---

## 4. 应用架构

```text
┌──────────────────────────────────────┐
│ Vue UI                               │
│                                      │
│ - 录音按钮                           │
│ - 当前绑定目标展示                   │
│ - 历史绑定列表                       │
│ - 输入桥状态展示                     │
│ - 白名单 / 黑名单配置                │
└──────────────────────────────────────┘
                  │
                  │ IPC
                  ▼
┌──────────────────────────────────────┐
│ Tauri / Electron 主程序              │
│                                      │
│ - 启动 voice-input-bridge.exe        │
│ - 发送 ASR final 文本                │
│ - 发送绑定 / 解绑指令                │
│ - 接收输入桥事件                     │
└──────────────────────────────────────┘
                  │
                  │ stdin/stdout 或 Named Pipe
                  ▼
┌──────────────────────────────────────┐
│ voice-input-bridge.exe               │
│ Rust Native Input Bridge             │
│                                      │
│ - Target Monitor                     │
│ - Target Resolver                    │
│ - History Store                      │
│ - Overlay Manager                    │
│ - Clipboard Paste Engine             │
│ - Win32 Adapter                      │
│ - UI Automation Adapter              │
└──────────────────────────────────────┘
                  │
                  ▼
┌──────────────────────────────────────┐
│ RIS / HIS / 浏览器版 RIS             │
│                                      │
│ - 报告编辑框                         │
│ - 病历编辑框                         │
│ - 文本输入区域                       │
└──────────────────────────────────────┘
```

---

## 5. 进程形态

### 5.1 主程序

主程序可以是：

```text
Tauri 应用
Electron 应用
```

主要负责：

```text
录音
ASR 调用
语音识别结果展示
用户配置
调用输入桥
```

---

### 5.2 输入桥程序

输入桥程序为：

```text
voice-input-bridge.exe
```

主要负责：

```text
监听 Windows 焦点变化
识别当前输入框
绑定输入目标
维护历史绑定栈
恢复最近可用目标
显示 Overlay 提示
执行剪贴板写入
发送 Ctrl + V
恢复原窗口
返回执行结果
```

---

## 6. 通信方案

### 6.1 第一阶段推荐方案

第一版建议使用：

```text
NDJSON over stdin/stdout
```

也就是：

```text
一行一个 JSON 消息
```

优点：

```text
实现简单
调试方便
Tauri / Electron 都容易调用
不需要额外端口
不需要本地服务
```

---

### 6.2 后续升级方案

后续可以升级为：

```text
Named Pipe
```

例如：

```text
\\.\pipe\jusha_voice_input_bridge
```

适合输入桥作为常驻服务运行。

---

## 7. 核心状态机

输入桥内部维护以下状态：

```text
Idle
CandidateReady
Locked
Recovering
FallbackCurrentFocus
Invalid
```

### 7.1 状态说明

| 状态 | 含义 |
|---|---|
| Idle | 未检测到可用输入目标 |
| CandidateReady | 检测到候选输入框，但尚未绑定 |
| Locked | 已绑定明确输入目标 |
| Recovering | 当前绑定目标失效，正在从历史恢复 |
| FallbackCurrentFocus | 历史目标不可用，临时使用当前焦点 |
| Invalid | 没有任何可写入目标 |

---

### 7.2 状态流转

```text
用户点击 RIS 报告框
Idle → CandidateReady

用户按绑定快捷键
CandidateReady → Locked

用户切换到 PACS
Locked 保持不变

ASR final 文本到达
Locked → 执行写入

RIS 关闭
Locked → Recovering

历史目标恢复成功
Recovering → Locked

历史目标全部失败
Recovering → FallbackCurrentFocus 或 Invalid
```

---

## 8. 目标选择策略

### 8.1 目标优先级

```text
1. lockedTarget 存在且可用
2. 最近一次绑定的历史目标可用
3. 历史目标顺位恢复成功
4. 当前聚焦输入框可用
5. 无目标，拒绝写入并提示用户
```

---

### 8.2 伪代码

```rust
fn resolve_target() -> Option<InputTarget> {
    if let Some(target) = locked_target.clone() {
        if is_target_available(&target) {
            return Some(target);
        }
    }

    if let Some(target) = find_available_from_history() {
        set_locked_target(target.clone());
        return Some(target);
    }

    if let Some(target) = detect_current_focused_editable() {
        if is_allowed_fallback_target(&target) {
            return Some(target);
        }
    }

    None
}
```

---

## 9. 数据模型设计

### 9.1 运行时目标

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct RuntimeTarget {
    pub top_hwnd: isize,
    pub focus_hwnd: isize,
    pub process_id: u32,
    pub thread_id: u32,
}
```

说明：

```text
top_hwnd：顶层窗口句柄
focus_hwnd：实际输入控件句柄
process_id：目标进程 ID
thread_id：目标 UI 线程 ID
```

这部分只在当前 Windows 会话中可靠。

---

### 9.2 可恢复目标签名

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct TargetSignature {
    pub process_name: String,
    pub exe_path: Option<String>,

    pub top_title: Option<String>,
    pub top_class_name: Option<String>,

    pub control_class_name: Option<String>,
    pub automation_id: Option<String>,
    pub control_name: Option<String>,
    pub control_type: Option<String>,

    pub rect_hint: Option<Rect>,
}
```

说明：

```text
TargetSignature 用于历史目标恢复。
即使 hwnd 失效，也可以根据进程名、窗口标题、控件类名、UIA 信息重新寻找目标。
```

---

### 9.3 历史绑定项

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct BoundTargetHistoryItem {
    pub id: String,
    pub display_name: String,

    pub signature: TargetSignature,
    pub last_runtime: Option<RuntimeTarget>,

    pub last_bound_at: i64,
    pub last_used_at: i64,
    pub use_count: u32,

    pub app_type: AppType,
    pub priority: i32,
}
```

```rust
#[derive(Debug, Clone, Serialize, Deserialize)]
pub enum AppType {
    RIS,
    HIS,
    BrowserRIS,
    Other,
}
```

---

## 10. 目标检测方案

### 10.1 Win32 检测能力

输入桥持续监听 Windows 焦点变化。

主要使用：

```text
SetWinEventHook(EVENT_OBJECT_FOCUS)
SetWinEventHook(EVENT_SYSTEM_FOREGROUND)
GetForegroundWindow
GetGUIThreadInfo
GetWindowThreadProcessId
GetClassNameW
GetWindowTextW
IsWindow
IsWindowVisible
```

---

### 10.2 UI Automation 辅助检测

对于 WPF、CEF、WebView、浏览器版 RIS，仅依赖 hwnd 可能不够，需要使用 UI Automation 辅助识别。

可采集信息：

```text
AutomationId
Name
ControlType
BoundingRectangle
IsEnabled
IsKeyboardFocusable
ValuePattern
TextPattern
LegacyIAccessiblePattern
```

---

### 10.3 可接受输入框类型

优先识别：

```text
Edit
RichEdit
RichEdit20W
RICHEDIT50W
WindowsForms10.EDIT
ThunderRT6TextBox
UIA ControlType.Edit
UIA Document
浏览器 contenteditable 区域
```

---

### 10.4 需要排除的目标

```text
自己的窗口
Overlay 窗口
PACS 图像窗口
桌面
任务栏
文件资源管理器
浏览器地址栏
聊天软件输入框
密码框
只读控件
不可见控件
禁用控件
```

---

## 11. 显式绑定流程

用户点击 RIS 报告框后，按快捷键：

```text
Ctrl + Alt + L
```

输入桥执行：

```text
1. 获取当前 focused editable target
2. 如果当前焦点不可用，则使用 candidateTarget
3. 校验目标是否可输入
4. 生成 TargetSignature
5. 设置为 lockedTarget
6. 写入历史绑定栈
7. 显示绿色 Overlay
8. 通知主程序绑定成功
```

伪代码：

```rust
pub fn lock_current_target() -> Result<LockResult> {
    let target = detect_current_focused_editable()
        .or_else(|| candidate_target.clone())
        .ok_or(Error::NoEditableTarget)?;

    validate_target(&target)?;

    locked_target = Some(target.clone());

    history_store.upsert(target.to_history_item())?;

    overlay.show_bound_target(&target)?;

    emit_event(TargetEvent::Locked(target.into_view_model()));

    Ok(LockResult::success())
}
```

---

## 12. 自动选择流程

当 ASR final 文本到达，但用户本次没有明确重新绑定时：

```text
1. 查 lockedTarget
2. lockedTarget 可用则写入
3. lockedTarget 不可用则查历史绑定栈
4. 历史目标可恢复则写入
5. 历史目标不可恢复则尝试当前聚焦输入框
6. 当前聚焦不可用则拒绝写入
```

伪代码：

```rust
pub fn paste_text(text: String) -> Result<PasteResult> {
    let target = resolve_target()
        .ok_or(Error::NoAvailableTarget)?;

    paste_engine.paste_to_target(&target, &text)?;

    history_store.mark_used(&target)?;

    overlay.flash_success(&target)?;

    Ok(PasteResult {
        target_id: target.id,
        status: PasteStatus::Success,
    })
}
```

---

## 13. 历史目标恢复算法

### 13.1 恢复原则

不要只依赖 hwnd。

hwnd 在以下情况下会失效：

```text
窗口关闭
进程重启
RIS 页面刷新
远程窗口重建
控件重新创建
```

因此恢复时需要使用 TargetSignature 进行相似度匹配。

---

### 13.2 匹配评分

建议评分：

| 匹配项 | 分数 |
|---|---:|
| processName 匹配 | +40 |
| exePath 匹配 | +30 |
| topClassName 匹配 | +15 |
| topTitle 相似 | +10 |
| controlClassName 匹配 | +20 |
| automationId 匹配 | +30 |
| controlName 相似 | +10 |
| rectHint 接近 | +5 |
| 最近使用时间 | +0 ~ +20 |
| 使用次数 | +0 ~ +10 |

建议阈值：

```text
>= 80：可以自动恢复
60 ~ 79：需要提示用户确认
< 60：不恢复
```

---

### 13.3 恢复伪代码

```rust
fn find_available_from_history() -> Option<InputTarget> {
    let items = history_store.list_by_recent();

    for item in items {
        if let Some(runtime) = item.last_runtime.clone() {
            if is_runtime_target_alive(&runtime) {
                return Some(InputTarget::from_runtime_and_signature(
                    runtime,
                    item.signature.clone()
                ));
            }
        }

        if let Some(recovered) = recover_by_signature(&item.signature) {
            if score_target(&item.signature, &recovered) >= 80 {
                return Some(recovered);
            }
        }
    }

    None
}
```

---

## 14. Overlay 绑定提示方案

### 14.1 实现原则

不要修改 RIS 窗口，不要注入 DLL，不要在 RIS 内部绘制。

正确方式是：

> Rust 输入桥创建一个独立透明 Overlay 窗口，覆盖在目标输入框或目标窗口上方。

---

### 14.2 Overlay 特性

```text
透明背景
绿色边框
红色提示文字
置顶
不抢焦点
鼠标穿透
不出现在任务栏
不影响 RIS / PACS 操作
```

---

### 14.3 Win32 窗口样式

```text
WS_POPUP
WS_EX_LAYERED
WS_EX_TRANSPARENT
WS_EX_TOPMOST
WS_EX_TOOLWINDOW
WS_EX_NOACTIVATE
```

鼠标穿透：

```text
WM_NCHITTEST 返回 HTTRANSPARENT
```

---

### 14.4 Overlay 显示内容

绑定成功时：

```text
语音写入目标：RIS - 报告编辑框
```

写入成功时：

```text
已写入
```

目标失效时：

```text
绑定目标已失效，请重新绑定
```

---

### 14.5 Overlay 颜色状态

| 状态 | 表现 | 含义 |
|---|---|---|
| 绿色 | 绿框 + 红字 | 目标有效，可写入 |
| 黄色 | 黄框 / 黄色状态条 | 目标存在但不可见，写入时会临时切换 |
| 红色 | 红色状态条 | 目标失效，需要重新绑定 |

---

### 14.6 Overlay 显示策略

不建议一直显示大绿框。

推荐策略：

```text
1. 绑定成功后，大绿框显示 2 秒
2. 日常使用中，只显示小型状态条
3. 写入成功时，目标框闪烁一次
4. 目标被 PACS 遮挡时，不显示大绿框，只显示状态条
5. 目标失效时，显示红色提示
```

---

## 15. 粘贴执行流程

当前已经实现剪贴板 + 粘贴，输入桥需要将其标准化。

### 15.1 标准流程

```text
1. resolveTarget()
2. 校验目标
3. 保存当前前台窗口，例如 PACS
4. 保存当前剪贴板内容
5. 激活目标窗口
6. 聚焦目标输入框
7. 设置剪贴板文本
8. 发送 Ctrl + V
9. 恢复剪贴板
10. 恢复原前台窗口
11. 更新历史目标 lastUsedAt
12. Overlay 提示写入成功
```

---

### 15.2 伪代码

```rust
pub fn paste_to_target(target: &InputTarget, text: &str) -> Result<()> {
    validate_before_paste(target)?;

    let previous_foreground = win32::get_foreground_window();

    let clipboard_guard = ClipboardGuard::save_current()?;

    focus_target(target)?;

    clipboard::set_unicode_text(text)?;

    keyboard::send_ctrl_v()?;

    clipboard_guard.restore()?;

    if let Some(hwnd) = previous_foreground {
        win32::restore_foreground(hwnd);
    }

    Ok(())
}
```

---

## 16. 写入前安全校验

每次写入前必须校验：

```text
目标窗口是否存在
目标进程是否还在
目标是否可见
目标是否最小化
目标控件是否还存在
进程名是否仍然匹配
控件 className 是否仍然匹配
是否命中黑名单应用
是否命中允许写入白名单
```

---

## 17. 白名单与黑名单配置

建议支持配置：

```json
{
  "allowedApps": [
    "ris.exe",
    "his.exe",
    "chrome.exe",
    "msedge.exe"
  ],
  "blockedApps": [
    "pacs.exe",
    "wechat.exe",
    "qq.exe",
    "explorer.exe"
  ],
  "fallbackToCurrentFocus": true,
  "requireWhitelistForFallback": true
}
```

关键策略：

```text
历史绑定目标可以自动恢复
当前焦点兜底必须命中白名单
黑名单应用永不写入
```

---

## 18. IPC 协议设计

推荐 JSON-RPC 风格。

---

### 18.1 获取状态

主程序发送：

```json
{
  "id": "1",
  "method": "state.get",
  "params": {}
}
```

输入桥返回：

```json
{
  "id": "1",
  "result": {
    "state": "Locked",
    "lockedTarget": {
      "targetId": "target-ris-001",
      "displayName": "RIS - 报告编辑框",
      "status": "valid"
    }
  }
}
```

---

### 18.2 锁定当前输入框

主程序发送：

```json
{
  "id": "2",
  "method": "target.lockCurrent",
  "params": {}
}
```

输入桥返回：

```json
{
  "id": "2",
  "result": {
    "success": true,
    "targetId": "target-ris-001",
    "displayName": "RIS - 报告编辑框"
  }
}
```

---

### 18.3 粘贴文本

主程序发送：

```json
{
  "id": "3",
  "method": "text.paste",
  "params": {
    "text": "双肺纹理增多，未见明显实变影。",
    "source": "asr-final",
    "segmentId": "seg-20260514-001"
  }
}
```

输入桥返回：

```json
{
  "id": "3",
  "result": {
    "success": true,
    "targetId": "target-ris-001",
    "displayName": "RIS - 报告编辑框"
  }
}
```

---

### 18.4 使用历史目标

主程序发送：

```json
{
  "id": "4",
  "method": "target.useHistory",
  "params": {
    "targetId": "target-ris-001"
  }
}
```

---

### 18.5 解除绑定

主程序发送：

```json
{
  "id": "5",
  "method": "target.unlock",
  "params": {}
}
```

---

### 18.6 显示绑定提示

主程序发送：

```json
{
  "id": "6",
  "method": "overlay.flash",
  "params": {
    "durationMs": 2000
  }
}
```

---

## 19. 输入桥事件

输入桥主动向主程序发送事件。

---

### 19.1 候选输入框变化

```json
{
  "event": "candidate.changed",
  "payload": {
    "displayName": "RIS - 报告编辑框",
    "processName": "ris.exe",
    "topTitle": "放射科信息系统",
    "controlClassName": "RICHEDIT50W"
  }
}
```

---

### 19.2 目标已绑定

```json
{
  "event": "target.locked",
  "payload": {
    "targetId": "target-ris-001",
    "displayName": "RIS - 报告编辑框",
    "status": "valid"
  }
}
```

---

### 19.3 目标失效

```json
{
  "event": "target.invalid",
  "payload": {
    "targetId": "target-ris-001",
    "reason": "window_closed"
  }
}
```

---

### 19.4 历史目标恢复成功

```json
{
  "event": "target.recovered",
  "payload": {
    "targetId": "target-ris-001",
    "displayName": "RIS - 报告编辑框",
    "fromHistory": true
  }
}
```

---

### 19.5 写入完成

```json
{
  "event": "text.pasted",
  "payload": {
    "segmentId": "seg-20260514-001",
    "targetId": "target-ris-001",
    "status": "success"
  }
}
```

---

## 20. Electron 调用示例

```ts
import { spawn } from "child_process";
import path from "path";

const bridgePath = path.join(
  process.resourcesPath,
  "voice-input-bridge.exe"
);

const bridge = spawn(bridgePath, ["--stdio"], {
  windowsHide: true,
  stdio: ["pipe", "pipe", "pipe"]
});

function sendToBridge(message: any) {
  bridge.stdin.write(JSON.stringify(message) + "\n");
}

sendToBridge({
  id: "1",
  method: "target.lockCurrent",
  params: {}
});

sendToBridge({
  id: "2",
  method: "text.paste",
  params: {
    text: "右下肺见少许条索影。",
    source: "asr-final",
    segmentId: "seg-001"
  }
});
```

---

## 21. Tauri 调用建议

Tauri 可以将输入桥作为 sidecar 打包。

目录示例：

```text
src-tauri/
  binaries/
    voice-input-bridge-x86_64-pc-windows-msvc.exe
  tauri.conf.json
```

配置示例：

```json
{
  "bundle": {
    "externalBin": [
      "binaries/voice-input-bridge"
    ]
  }
}
```

主程序通过 sidecar 启动输入桥，并通过 stdin/stdout 与其通信。

---

## 22. Rust 工程结构

推荐使用 workspace：

```text
voice-input-bridge/
  Cargo.toml

  crates/
    bridge-app/
      src/main.rs

    bridge-core/
      src/state.rs
      src/model.rs
      src/resolver.rs
      src/config.rs

    bridge-win32/
      src/window.rs
      src/focus.rs
      src/clipboard.rs
      src/keyboard.rs
      src/event_hook.rs

    bridge-uia/
      src/uia.rs
      src/editable.rs

    bridge-overlay/
      src/overlay_window.rs
      src/drawing.rs

    bridge-ipc/
      src/stdio.rs
      src/named_pipe.rs
      src/protocol.rs

    bridge-history/
      src/store.rs
      src/scoring.rs
```

---

## 23. 推荐依赖

```toml
[dependencies]
serde = "1"
serde_json = "1"
uuid = "1"
tracing = "0.1"
tracing-subscriber = "0.3"
parking_lot = "0.12"
anyhow = "1"
thiserror = "1"

windows = { version = "0.58", features = [
  "Win32_Foundation",
  "Win32_UI_WindowsAndMessaging",
  "Win32_UI_Input_KeyboardAndMouse",
  "Win32_System_Threading",
  "Win32_System_ProcessStatus",
  "Win32_System_DataExchange",
  "Win32_System_Memory",
  "Win32_UI_Accessibility",
  "Win32_Graphics_Gdi"
] }
```

---

## 24. 本地存储设计

建议存储路径：

```text
%APPDATA%\jusha\VoiceInputBridge\
  config.json
  target-history.json
  logs\
```

---

### 24.1 target-history.json 示例

```json
{
  "version": 1,
  "targets": [
    {
      "id": "target-ris-001",
      "displayName": "RIS - 报告编辑框",
      "signature": {
        "processName": "ris.exe",
        "exePath": "C:\\RIS\\ris.exe",
        "topTitle": "放射科信息系统",
        "topClassName": "TMainForm",
        "controlClassName": "RICHEDIT50W",
        "controlName": "报告内容"
      },
      "lastBoundAt": 1778750000000,
      "lastUsedAt": 1778750200000,
      "useCount": 18,
      "appType": "RIS",
      "priority": 100
    }
  ]
}
```

---

## 25. 用户交互流程

### 25.1 首次绑定

```text
1. 用户打开 RIS
2. 用户点击报告编辑框
3. 用户按 Ctrl + Alt + L
4. 系统显示绿框红字：
   语音写入目标：RIS - 报告编辑框
5. 该目标进入历史绑定栈
```

---

### 25.2 日常使用

```text
1. 用户打开 RIS 和 PACS
2. 用户直接开始语音输入
3. 系统自动使用最近一次绑定的 RIS 报告框
4. 如果最近目标不可用，顺位恢复历史目标
5. 如果历史目标都不可用，才使用当前聚焦输入框
```

---

### 25.3 目标失效

提示：

```text
绑定目标已失效，请重新点击报告框并按 Ctrl + Alt + L
```

---

### 25.4 目标自动恢复

提示：

```text
已自动使用最近绑定目标：RIS - 报告编辑框
```

---

### 25.5 当前焦点兜底

提示：

```text
未找到历史绑定目标，已使用当前输入框
```

该提示需要醒目，因为兜底写入风险较高。

---

## 26. 日志策略

医疗场景中不要记录完整报告文本。

错误示例：

```text
2026-05-14 10:12:01 写入文本：双肺纹理增多，未见明显实变影。
```

推荐示例：

```text
2026-05-14 10:12:01 paste_success target=target-ris-001 segment=seg-001 length=18
```

日志字段建议：

```text
timestamp
event_type
target_id
process_name
success
error_code
text_length
segment_id
```

---

## 27. 安全策略

默认开启以下策略：

```text
1. 写入前必须解析目标
2. 不能解析目标则拒绝写入
3. 当前焦点兜底必须命中白名单
4. 黑名单应用永不写入
5. 不记录完整报告文本日志
6. Overlay 明确提示当前写入目标
7. 用户可一键解除绑定
8. 用户可查看最近绑定目标
9. 用户可删除历史绑定记录
```

---

## 28. Win7 兼容边界

由于存在 Win7 兼容诉求，需要单独维护兼容策略。

### 28.1 Electron

Win7 场景建议：

```text
Electron 固定在 22.x
不要升级到 Electron 23+
```

### 28.2 Rust

Win7 场景建议：

```text
单独维护 Win7 构建分支
固定 Rust toolchain
固定 windows crate 版本
避免使用 Win10-only API
优先使用 Win32 基础 API
Overlay 使用传统 Win32/GDI 实现
```

### 28.3 输入桥

输入桥需要避免依赖过新的 Windows 能力。

优先使用：

```text
Win32 API
GDI
SetWinEventHook
GetForegroundWindow
GetGUIThreadInfo
Clipboard API
SendInput
UI Automation 基础能力
```

---

## 29. MVP 实施阶段

### 第一阶段：输入桥基础能力

目标：

```text
可以锁定当前输入框
可以被 Tauri / Electron 调用
可以粘贴文本
可以恢复原窗口
```

实现内容：

```text
stdio IPC
GetForegroundWindow
GetGUIThreadInfo
剪贴板保存/恢复
Ctrl + V
基础日志
```

---

### 第二阶段：历史绑定栈

目标：

```text
不用每次重新绑定
自动使用最近绑定目标
目标关闭后顺位恢复
```

实现内容：

```text
target-history.json
目标签名
历史排序
可用性检测
恢复算法
```

---

### 第三阶段：Overlay 绿框红字

目标：

```text
用户一眼知道当前绑定窗口
降低误写风险
```

实现内容：

```text
透明 Win32 Overlay
绿色边框
红色文字
鼠标穿透
不抢焦点
状态条
```

---

### 第四阶段：UIA 增强

目标：

```text
支持 WPF、CEF、浏览器版 RIS
```

实现内容：

```text
UI Automation focused element
ControlType 判断
BoundingRectangle 获取
AutomationId / Name / RuntimeId 记录
```

---

### 第五阶段：Win7 专项构建

目标：

```text
单独产出 Win7 可运行版本
```

实现内容：

```text
固定 Electron 22
固定 Rust toolchain
固定依赖版本
回归测试 Win7 / Win10 / Win11
```

---

## 30. 最终推荐结论

该能力应作为独立 Rust 原生输入桥实现：

```text
voice-input-bridge.exe
```

职责边界：

```text
Tauri / Electron / Vue：
负责 UI、录音、ASR、配置、状态展示

Rust Input Bridge：
负责 Windows 输入目标绑定、历史恢复、Overlay 提示、剪贴板粘贴
```

核心策略：

```text
显式绑定优先
历史绑定自动恢复
最近可用目标优先
当前焦点最后兜底
Overlay 明确提示当前绑定目标
写入前严格校验
```

该方案适合 RIS + PACS 并行操作场景：

```text
用户在 PACS 上看图和操作
语音识别文本仍然优先写入最近绑定的 RIS 报告框
用户可以通过绿框、红字和状态条明确知道当前写入目标
系统通过历史恢复和白名单机制降低误写风险
```
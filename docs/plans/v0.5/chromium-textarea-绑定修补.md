# Chromium SPA 文本框绑定修补计划

## 1. 目标

让 voice-input-bridge 能稳定锁定到 Chromium 类窗口（Chrome / Edge / Electron 内嵌站 / 巨鲨远程诊断平台等基于 SPA 的 Web RIS）内部的具体 `<textarea>` / `<input>` / `[contenteditable]` 节点，而不是退化为整个 `Chrome_RenderWidgetHostHWND` 渲染宿主。

典型场景：

```text
浏览器地址：https://192.168.40.155:9443/remote-diagnosis/#/report/report-detail
目标控件：「影像所见」textarea / 「诊断意见」textarea / 其他 SPA 内编辑框
```

期望表现：

```text
绑定后显示：浏览器 RIS - 报告详情 - 影像所见
切换 PACS / 切回浏览器 / Tab 切换后仍可写入正确 textarea
首次冷启动绑定一次成功，无需手动重试
```

---

## 2. 现状根因

实现位置：`desktop-electron/native/inject-helper/src/windows_impl.rs`

### 2.1 HWND 这层只能拿到渲染宿主

`resolve_bindable_focus_hwnd`（约 1942 行）遇到 Chromium 顶层窗口时直接 fall back 到 `Chrome_RenderWidgetHostHWND` 或 `Chrome_WidgetWin_*`。这是 Chromium 的设计：所有 `<input>` / `<textarea>` 都不暴露为独立 HWND，只能通过 UIA / IA2 / DevTools 协议拿到。这是「锁了整页」的直接原因。

### 2.2 UIA 是临时实例，触发不了 Chromium 的可访问性模式

`detect_accessibility_focus_hint`（约 2469 行）每次都新建 `CoInitializeEx` + `CoCreateInstance(CUIAutomation)`，函数返回时 `ComInitGuard::drop` 又把 COM uninit 掉了。Chromium 看到的是「短连接、连完就断」，**不会进入完整可访问性模式**，结果就是：

- `GetFocusedElement()` 拿到的只是文档根 / 渲染面 pane
- 子树里看不到 `<textarea>` 这个 UIA Edit 节点
- `accessibility_hint_from_element_or_related` 在 `RawViewWalker` 上走祖先 + `FindAll(Descendants)`，但因为子节点压根没暴露，最终降级为「面积接近宿主」的元素 → 被 `accessibility_hint_is_precise` 判定不够精确 → 没产生有效 hint

Chromium 触发完整 a11y 树通常需要满足以下任一条件：

```text
进程内有持续存活的 IUIAutomation 客户端，并保持事件订阅
进程接收到 WM_GETOBJECT(OBJID_CLIENT) 之后没有立刻断连
启动参数 --force-renderer-accessibility（用户不可控）
```

我们必须自己做第 1、2 条。

### 2.3 UIA 命中后的信息没传进 runtime

即使 UIA 偶尔命中 textarea，结果只塞进 `signature.rect_hint / automation_id / control_name / control_type`（约 2799-2811 行），`runtime.focus_hwnd` 依旧是 `Chrome_RenderWidgetHostHWND`，`make_display_name` 用的 `control_class_name` 还是 HWND 类名。用户看到的 displayName 仍像「浏览器 RIS - 标签页标题 - Chrome_RenderWidgetHostHWND」，体感就是「没锁到框」。

### 2.4 `make_target_id` 在 SPA 内分辨力不足

`make_target_id`（约 1828 行）依赖签名做哈希。SPA 里两个不同 textarea：

```text
process_name 相同
top_title 相同（同一标签页）
control_class_name 相同（都是 Chrome_RenderWidgetHostHWND）
automation_id 多数为空（前端框架自动生成或没有 id）
control_name 在 UIA 没暖起来时也为空
```

→ 两个 textarea 哈希成同一个 target_id，历史绑定栈区分不出。

### 2.5 `accessibility_hint_is_precise` 阈值过严

`accessibility_hint_is_precise`（约 2747 行）要求 `control_type == "UIAutomation:Edit"` 或 `rect_substantially_smaller`。`rect_substantially_smaller`（约 2991 行）的阈值是「内框面积 × 100 < 外框面积 × 80」并且「边长差 > 48px」。一个占据右侧大半内容区的 textarea，rect 已经接近宿主可用区域，会被判定不够小，hint 被丢弃，退回宿主。

### 2.6 粘贴路径在 web 上靠的是 Chromium 内部焦点

`focus_target` 对 `is_web_like_target` 故意不调用 `SetFocus(focus)`（约 966 行），完全依赖 Chrome 自己记忆的 caret。这意味着：

```text
绑定时只是「记录目标」，写入时 Chrome 自身必须仍把 caret 留在该 textarea
切换 Tab / 失焦 / 点击其它元素后，Chrome caret 会跑掉
当前没有「绑定时记录 textarea UIA 标识，写入时用 UIA 重新 focus」的能力
```

修补必须把 textarea 的 UIA 句柄真正用起来。

---

## 3. 总体修补策略

把「Chromium 路径」从「HWND 模式 + 偶发 UIA 增强」改成「常驻 UIA + HWND 作为锚点」：

```text
启动期：开一个常驻 UIA 单例 + 焦点事件订阅 → 拉起 Chromium a11y 模式
绑定期：lock 时强制走 UIA 拿 textarea，写回 runtime + signature + display_name
写入期：粘贴前用 UIA 把 textarea 重新 focus 一次，避免 caret 漂移
兼容期：UIA 拿不到时回退到现行 HWND 锁宿主逻辑（不破坏现有行为）
```

---

## 4. 实施任务拆解

下列任务相互弱依赖，可按顺序合并；每项给出落点文件、函数和验收点。

### 任务 A：常驻 UIA 单例 + Chromium a11y 暖启动

**目标**：bridge 启动后保持一个全局 `IUIAutomation` 实例和一个 `AddFocusChangedEventHandler` 监听，让 Chromium 维持完整 a11y 树。

**改动点**：

- 新增模块：`windows_impl::uia_runtime`（同文件内 mod 或拆 `uia_runtime.rs`，建议拆文件）
- 在 `ensure_focus_tracker`（搜索关键字 `ensure_focus_tracker`）相邻位置增加 `ensure_uia_runtime()`，进程生命周期内一次性初始化
- 用一个独立 MTA / STA 线程承载 COM 与回调，避免阻塞 inject-helper 主线程

**结构**：

```rust
struct UiaRuntime {
    _thread: thread::JoinHandle<()>,
    shutdown_tx: mpsc::Sender<()>,
    automation: Arc<UiaHandle>,           // Send + Sync 包装的 IUIAutomation
    last_focused: parking_lot::Mutex<Option<CachedFocusedElement>>,
}

struct CachedFocusedElement {
    hwnd_top: HWND,
    runtime_id: Vec<i32>,
    rect: RectHint,
    control_type: UIA_CONTROLTYPE_ID,
    captured_at: Instant,
}
```

**关键 API**：

```rust
fn ensure_uia_runtime() -> &'static UiaRuntime
fn uia_runtime() -> Option<&'static UiaRuntime>
fn warm_chromium_accessibility(top_hwnd: HWND)
    // 对 Chromium 顶层窗口发送一次 SendMessageTimeoutW(WM_GETOBJECT, OBJID_CLIENT)
    // 用来推动 Chrome 立刻完成 a11y 树构建
```

**验收**：

```text
打开 Chrome / Edge，bridge 启动后 5 秒内
chrome://accessibility 显示 accessibility mode = complete
desktop-electron 调试日志出现 uia_runtime_started focus_event_subscribed
```

---

### 任务 B：focus changed 事件 → 提前缓存 textarea 标识

**目标**：用户点击 textarea 的瞬间，bridge 已经知道这是哪个元素，避免 lock 热键触发时才现场查 UIA 还没暖。

**改动点**：

- 在任务 A 的 `UiaRuntime` 里实现 `IUIAutomationFocusChangedEventHandler`
- 回调里把元素的 `RuntimeId / BoundingRectangle / ControlType / Name / AutomationId / HasKeyboardFocus / SupportedPatterns` 写进 `last_focused`
- 只缓存「来自 Chromium 顶层窗口 + ControlType ∈ {Edit, Document, ComboBox} + 支持 Value/Text Pattern」的元素

**关键 API**：

```rust
fn snapshot_chromium_focus(top_hwnd: HWND) -> Option<UiaFocusSnapshot>
    // 优先读 last_focused 缓存
    // 缓存为空或顶层不匹配则现场调 GetFocusedElement
```

**验收**：

```text
点击「影像所见」textarea，再点 PACS 任意位置
此时调用 snapshot_chromium_focus 仍能返回 textarea 的 runtime_id 与 Name
```

---

### 任务 C：把 UIA 命中下沉到 runtime + display_name

**目标**：当 UIA 拿到具体 textarea 时，把它的 `runtime_id` 等强标识 **写入 runtime 层**，并让 `make_display_name` 用 textarea 的 `Name` 而不是 HWND 类名。

**改动点**：

- `RuntimeTarget` 增加字段（保持向后兼容，缺省时按现有逻辑）：

```rust
pub struct RuntimeTarget {
    pub top_hwnd: isize,
    pub focus_hwnd: isize,
    pub process_id: u32,
    pub thread_id: u32,

    // 新增
    pub uia_runtime_id: Option<Vec<i32>>,
    pub uia_provider_kind: Option<String>, // "Chromium" / "Win32" / "Office"
}
```

- `build_target`（约 751 行）在 Chromium 路径下：
  - 调 `snapshot_chromium_focus(top_hwnd)` 拿 UIA snapshot
  - 把 `runtime_id` 写进 `RuntimeTarget.uia_runtime_id`
  - 用 UIA 的 `control_name` 覆盖 `control_class_name` 作为 display 用途（保留 HWND 类名进日志）
- `make_display_name`（约 1838 行）增加分支：

```rust
fn make_display_name(process_name: &str, title: Option<&str>, control_class: Option<&str>, uia_name: Option<&str>) -> String
// Chromium：浏览器 RIS - <title> - <uia_name 优先, 没有则 textarea(rect)>
```

- `make_target_id`（约 1828 行）参与哈希时若有 `uia_runtime_id` 优先用之，否则才用现有字段，确保 SPA 内同页两个 textarea 哈希不同

**验收**：

```text
绑定「影像所见」与「诊断意见」分别生成两条历史目标
target-history.json 中两条 id 不同，displayName 分别是
  浏览器 RIS - 巨鲨远程诊断平台 - 影像所见
  浏览器 RIS - 巨鲨远程诊断平台 - 诊断意见
```

---

### 任务 D：放宽 `accessibility_hint_is_precise` 并补 ARIA / contenteditable 路径

**目标**：让 textarea / contenteditable 大块编辑框也能稳定通过精确性校验。

**改动点**：

- `accessibility_hint_is_precise`（约 2747 行）：除现有条件外，新增 `(has_value_pattern || has_text_pattern) && has_keyboard_focus` 即视为精确
- `is_web_input_accessibility_focus`（约 2898 行）：放宽对 Document / Custom + TextPattern 的接受范围，匹配 React/Vue 把 contenteditable 实现为 div 的常见模式（UIA 报为 Custom + TextPattern + HasKeyboardFocus）
- `accessibility_candidate_score`（约 2773 行）：对 `HasKeyboardFocus && TextPattern && rect 内含 cursor`（点选模式将来用）的元素再加 20 分，避免被父 Pane 吃掉

**验收**：

```text
连续测试以下三类目标都能命中（手工或回放）：
  巨鲨平台 textarea（标准 textarea，ControlType=Edit）
  富文本编辑器（quill / tiptap，ControlType=Document 或 Custom + TextPattern）
  TinyMCE / CKEditor iframe（ControlType=Document）
```

---

### 任务 E：写入路径用 UIA 重新 focus textarea

**目标**：粘贴时先用 `runtime_id` 把 textarea 重新拿到、`SetFocus()` 一下，再发 Ctrl+V，避免 Chromium caret 漂移。

**改动点**：

- `focus_target`（约 929 行）`is_web_like_target` 分支：
  - 如果 `target.runtime.uia_runtime_id` 存在 → `IUIAutomation::ElementFromIAccessible` 不适用，改用 `FindFirstBuildCache` + `PropertyConditionEx` 按 `RuntimeIdPropertyId` 匹配
  - 命中后取 `LegacyIAccessiblePattern::Select()` 或 `InvokePattern` 兜底，再走 `SetForegroundWindow(top)` 让浏览器获得前台
  - 失败回退到现行「不抢焦点」逻辑
- `is_runtime_target_alive`（约 838 行）：Chromium 路径下若有 `uia_runtime_id`，存活校验改为「能否通过 runtime_id 在 UIA 树里找到 + 仍是 keyboard focusable」

**验收**：

```text
绑定「影像所见」后切到 PACS、再切到浏览器但 caret 留在其它输入框
按 ASR 写入：内容仍写到「影像所见」，不会写错 textarea
```

---

### 任务 F：失败兜底与日志

**目标**：UIA 这条路任何环节失败都必须能掉回现行 HWND 模式，并把失败原因落到 `append_log`。

**改动点**：

- 在 `detect_current_focused_editable`（约 689 行）的 Chromium 分支增加结构化日志：

```text
chromium_lock_attempt
  has_uia_runtime=true/false
  focus_cache_age_ms=...
  focus_runtime_id=...
  resolved_via=uia|hwnd
  fallback_reason=cold_uia|no_match|ancestor_too_large
```

- 失败回退到当前 `resolve_bindable_focus_hwnd` 既有行为，保证「修补只增能力，不破坏现状」

**验收**：

```text
关掉任务 A 的 UIA runtime（feature flag 强关）后
所有现有用例（Office / 微信 / PACS 报告框）回归测试通过
target-history.json 中老 target_id 仍可被恢复
```

---

## 5. 数据兼容性

- `RuntimeTarget` 新增字段使用 `#[serde(default)]`，旧 `target-history.json` 反序列化不报错
- `make_target_id` 引入新字段后，旧目标的 id 会变化 → 一次性「软迁移」：加载历史时若发现某条目签名能在新算法下命中已有运行时，写一个新 id 旁路项；不强行删除旧记录
- 若评估迁移风险大，可走 feature flag：`BRIDGE_TARGET_ID_V2=1` 时启用新 id，默认关闭，给灰度时间

---

## 6. 测试矩阵

| 场景 | 期望 |
|---|---|
| Chrome + 巨鲨远程诊断平台 / 影像所见 | UIA 命中 Edit，displayName 含「影像所见」 |
| Chrome + 巨鲨远程诊断平台 / 诊断意见 | UIA 命中 Edit，与影像所见 id 不同 |
| Edge + 同一站 | 等同 Chrome |
| Electron 内嵌（钉钉 / 飞书评论框） | UIA 命中或回退 HWND，不报错 |
| 富文本编辑器（contenteditable） | UIA 命中 Document/Custom + TextPattern |
| 老版 Chromium / 无 a11y 暴露 | 回退 HWND 锁宿主，行为同今天 |
| 桌面 RIS（非浏览器） | 完全走原 Win32 路径，无回归 |
| Office Word / Excel | 走 Office 路径，无回归 |
| Win7 + Chrome 109 | UIA 焦点事件可工作 → 主验证项 |

---

## 7. 风险与开关

- **风险 1：UIA 焦点事件回调在 RDP / 远程桌面下卡顿** → 留 `BRIDGE_DISABLE_UIA=1` 环境变量整体回退
- **风险 2：常驻 a11y 可能让 Chromium 增加少量内存** → 文档注明；提供「仅在前台为 Chromium 时启用」的优化路径作为后续
- **风险 3：runtime_id 在 Chrome 重启 / 页面刷新后失效** → 必须依靠 task C 的 signature 二次匹配兜底（Name + rect_hint）
- **风险 4：跨进程 COM 阻塞** → UIA runtime 必须在独立线程；主线程不要直接 await 阻塞调用

---

## 8. 实施顺序建议

```text
任务 A → 任务 B → 任务 C → 任务 D → 任务 E → 任务 F
        ↑                       ↑
        独立可验证               独立可验证
```

最小可用切片：A + B + C + F 即可让用户看到「绑到 textarea、displayName 正确、有日志」；D + E 是稳定性收尾。

---

## 9. 相关代码位置速查

| 关注点 | 位置 |
|---|---|
| Chromium HWND 路径 | `desktop-electron/native/inject-helper/src/windows_impl.rs:1942` `resolve_bindable_focus_hwnd` |
| UIA 检测入口 | 同文件 `:2469` `detect_accessibility_focus_hint` |
| 元素评分 | 同文件 `:2773` `accessibility_candidate_score` |
| 精确性判定 | 同文件 `:2747` `accessibility_hint_is_precise` |
| 接受性判定 | 同文件 `:2898` `is_web_input_accessibility_focus` |
| 目标构建 | 同文件 `:751` `build_target` |
| 展示名 | 同文件 `:1838` `make_display_name` |
| 目标 id 哈希 | 同文件 `:1828` `make_target_id` |
| 写入聚焦 | 同文件 `:929` `focus_target` |
| 存活校验 | 同文件 `:838` `is_runtime_target_alive` |
| Tauri 命令封装 | `desktop/src-tauri/src/input_bridge.rs` |
| 前端调用 | `desktop/src/composables/useInputBridge.ts`、`desktop/src/components/SettingsPanel.vue` |

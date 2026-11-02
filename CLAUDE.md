# LLMGatewayProxy — 通用多厂商 LLM 网关代理（免费版）

Wails v2 (Go + Vue 3) 桌面应用：把任意 OpenAI / Anthropic / Gemini 厂商统一成本地代理，供其它 Agent（Zoo Code、Claude Code、Codex、curl/SDK）调用。窗口关闭驻留托盘，单实例。**free 分支为免费版**：不包含支付/订阅流程与功能限制；commercial 分支保留商业版。

## 构建与运行

```bash
cd d:\test\llm-gateway-proxy
wails build          # 产物 build\bin\LLMGatewayProxy.exe（必须 wails build，不能 go build 直出——需打包前端资源）
wails dev            # 开发调试
go test ./...  # 后端单测
cd frontend && npm run build   # 仅前端
```

- 分发部署见 `DEPLOY.md`（部署包 = 单 exe）。
- 改了 `app.go` 绑定方法后 `wails build` 会重新生成 `frontend/wailsjs/go/main/App.js`；若 bindings 生成器静默失败（App.js 缺新方法、前端报 "X is not exported"），删除整个 `frontend/wailsjs` 目录再 `wails build` 可恢复；`runtime.js` 不会被重新生成，需保留/手写。
- 运行日志：`app-run.log`（根目录）。

## 架构

```
main.go            embed frontend/dist、Frameless 窗口、OnBeforeClose、单实例锁
app.go             全部 Wails 绑定层；startup 组装服务；方法统一返回 jsonOK/jsonErr 字符串
tray.go            energye/systray 托盘（图标必须 .ico；双击=SetOnClick；退出走 quitting 标志）
internal/
  config      配置与厂商管理；敏感字段（apiKey/localApiKey）DPAPI 加密落盘
  dpapi       Windows DPAPI 封装（"dpapi:"+base64 前缀）；非 Windows 恒等实现
  provider    Provider 接口 + Registry：按模型名路由（显式前缀 "厂商名/模型名" 消歧 + Conflict 检测）
  transform   协议双向转换：OAIChat ↔ Anthropic / Gemini / Responses（请求、响应、SSE 流状态机）
  proxy       HTTP 服务器：/v1/chat/completions、/v1/messages、/v1/responses（+count_tokens）；
              候选链 failover（5xx/429 且未写出字节才切换下一候选）
  logbuf      环形请求日志（事件 log:new 推给前端）
  recorder    可选请求录制（JSONL 文件 llm-record-YYYYMMDD.jsonl）
  stats       用量聚合（date|provider|model）+ 通配计价；GetStats 补齐连续日期轴
  health      厂商探活循环（模型列表接口，不发消息）；状态翻转发 health:change + 系统通知
  autostart / singleinstance / paths   开机自启注册表、单实例锁、%APPDATA% 路径
frontend/src/pages/  Dashboard / Providers / Models / Stats / Logs / Recordings / Playground / ProxySettings
```

### 协议转换矩阵

客户端协议（三端点）× 上游协议（openai/anthropic/gemini 三类 provider）：同构直通；异构一律经 **OpenAI Chat 中间表示** 串联（如 Responses 客户端 → Gemini 上游 = R2O → O2G，流式同理级联）。工具调用、流式 SSE、usage 全覆盖且有单测。

## 关键踩坑（勿重复）

- **wails 事件**：`EventsOff(事件名)` 移除该事件**全部**监听器。页面级订阅必须用 `EventsOn` 返回的 cancel 函数解绑（App.vue 持有全局 log:new 订阅，误删会导致请求日志不自动刷新）。
- **OnBeforeClose 返回 true 会拦截 runtime.Quit**：托盘退出需要 quitting 原子标志放行 + `os.Exit(0)` 兜底。
- **DPAPI**：必须先 copy 到新切片再 LocalFree，`defer LocalFree` + 返回 unsafe.Slice 是 use-after-free。
- **systray**：窗口类缺 CS_DBLCLKS 收不到双击事件，用 SetOnClick 模拟。
- **前端拖拽区**：窗口按钮要 `--wails-draggable: no-drag`，否则首次点击被吞（按钮按两次才生效）。
- **WSL2 访问**：NAT 下 127.0.0.1 不通宿主 → bind 0.0.0.0，WSL 内用网关 IP（`ip route show default`）。
- **Windows shell**：PowerShell 中文脚本要 UTF-8 BOM；curl 传中文模型名会乱码（用 node 写 UTF-8 临时文件 + `--data-binary @file`）；curl 测 /v1/messages 需 x-api-key 头。
- **测试配置隔离**：配置测试使用 `config.NewInMemoryDefault()`，避免读写真实 `%APPDATA%` 数据。

## 约定

- **UI 体系**：Linear 风深色 design tokens 全在 `style.css`（`--bg/--bg-card/--bg-elevated/--accent/--grad-accent` 等；`--bg2/--bg3/--text2` 为兼容别名勿删）；图标用 `lucide-vue-next`（禁用 emoji 当图标）；字体 Inter/JetBrains Mono 经 @fontsource 打包（中文回退雅黑）；输入框统一用全局 `.input` 类、开关统一 `.switch-row`（自绘 toggle）；弹窗遮罩 `.modal-mask`（z-index 1001，高于 toast）；页面高度用 `calc(100vh - var(--titlebar-h) - 2*var(--main-pad))` 勿写魔法数；标题栏为悬浮玻璃（absolute + backdrop-filter），拖拽机制在 TitleBar.vue 勿动。
- app.go 绑定方法返回值统一 `{"success":bool,"data":...|"error":...,"code"?...}` JSON 字符串，前端 `JSON.parse`。
- 配置/数据目录：`%APPDATA%\LLMGatewayProxy\`（config.json、stats.json、app.log、proxy-access.log、recordings\）。
- **UI 体系（2026-09 改版）**：Linear 风深色 design tokens 全在 `style.css`（`--bg/--bg-card/--bg-elevated/--accent/--grad-accent` 等；`--bg2/--bg3/--text2` 为兼容别名勿删）；图标用 `lucide-vue-next`（禁用 emoji 当图标）；字体 Inter/JetBrains Mono 经 @fontsource 打包（中文回退雅黑）；输入框统一用全局 `.input` 类、开关统一 `.switch-row`（自绘 toggle）；页面高度用 `calc(100vh - var(--titlebar-h) - 2*var(--main-pad))` 勿写魔法数；标题栏为悬浮玻璃（absolute + backdrop-filter），拖拽机制在 TitleBar.vue 勿动。

- app.go 绑定方法返回值统一 `{"success":bool,"data":...|"error":...}` JSON 字符串，前端 `JSON.parse`。
- **i18n（2026-10）**：新增用户可见文案必须走 i18n，禁止硬编码中文——前端 `t('模块.键')`（vue-i18n@9，消息在 `frontend/src/i18n/locales/{zh,en}/`，按页面模块拆文件，en 用 `MessageSchema` 类型与 zh 强制同构）；后端 `i18n.T("模块.键")`（`internal/i18n/`，zh/en 键集须一致，`i18n_test.go` 校验）。调用点禁 `fmt.Errorf(T(...))`（go vet 报 non-constant format string），写 `errors.New(T(...))` 或 `fmt.Errorf("%s: %w", T(...), err)`。语言存 `Config.Language`（""=跟随系统|zh|en）；切换经 SaveConfig，后端检测变化后 `i18n.SetLang`（通知/错误文案即时生效）。**托盘菜单语言只在启动时取词，禁止运行时 SetTitle 动态改词**——energye/systray 的菜单更新从非托盘线程执行时，与右键 `TrackPopupMenu` 模态循环并发会死锁托盘消息循环（WndProc 卡死→右键菜单弹不出，2026-10 实际踩坑）；托盘菜单语言重启后生效。前端日期格式化用 `utils/format.ts`（勿写 `toLocaleString('zh-CN')`）。哨兵错误本体不译，在展示边界翻译。日志 log.Printf 与注释不翻译。
  - **坑（曾致登录页渲染卡死）**：vue-i18n 消息文案里**禁止出现字面 `{}`、`<>`**——生产模式消息编译器把 `{a, b}` 当非法插值**直接抛错**（dev 仅降级告警，问题易漏），渲染函数抛错会卡死 `<Transition out-in>` 使整个页面出不来。含协议 URL/参数等字面花括号的技术文本走 locale 文件的独立导出常量。组件统一 `useI18n({ useScope: 'global' })`（裸 `useI18n()` 每次挂载刷 "Not found parent scope" 警告）。
- 配置/数据目录：`%APPDATA%\LLMGatewayProxy\`（config.json、stats.json、app.log、proxy-access.log、recordings\）。
- 代理默认端口 8717；本地 API Key 建议启用（已 DPAPI 加密存储）。
- 无默认厂商：`defaultProviderId` 空为合法稳态（未命中模型名直接 404）；旧配置的 "gw" 引用由 Normalize 静默清除。

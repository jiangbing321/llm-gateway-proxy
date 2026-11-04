# LLMGatewayProxy 部署指南

把本程序分发给用户使用时，按本文档操作。目标读者：接收程序的最终用户。

---

## 一、部署包内容

| 文件 | 说明 |
|---|---|
| `LLMGatewayProxy.exe` | 主程序，**单文件**（界面已内嵌，无需安装） |

打包位置：`build\bin\LLMGatewayProxy.exe`（用 `wails build` 生成）。

## 二、系统要求

- Windows 10（较新版本）/ Windows 11，x64
- **WebView2 Runtime**：Win11 自带；Win10 多数自带。若启动无界面，从微软官网安装 [Evergreen Runtime](https://developer.microsoft.com/microsoft-edge/webview2/)
- 资源占用很低（内存 ~100MB，磁盘 < 50MB/月，视录制量）

## 三、安装步骤

1. **把整个文件夹拷贝到固定位置**，如 `D:\Tools\LLMGatewayProxy\`。
   > ⚠️ 不要放在「下载」或临时目录：开机自启记录的是 exe 绝对路径，之后移动文件夹会导致自启失效（移动后重新开关一次自启即可）。

2. **双击 `LLMGatewayProxy.exe`** 运行。首次启动自动创建数据目录 `%APPDATA%\LLMGatewayProxy\`（配置、日志、录制数据都在这里）。

3. **配置厂商**：「厂商管理」页添加 OpenAI / Anthropic / Gemini 兼容的上游（填 BaseURL + API Key + 模型列表），实现多厂商路由与故障转移。

4. **代理已就绪**：默认开机自动启动代理，端口 `8717`。「仪表盘」页可查看/复制接入地址。

## 四、客户端接入

### OpenAI 兼容客户端（Zoo Code / Cline / 各种 SDK）

```
Base URL:  http://127.0.0.1:8717/v1
API Key:   （你在「代理设置」页配置的本地 API Key，未设置则任意值）
模型名:    见「模型列表」页
```

curl 示例：

```bash
curl -N http://127.0.0.1:8717/v1/chat/completions \
  -H "x-api-key: <你的本地Key>" \
  -H "Content-Type: application/json" \
  -d '{"model":"MODEL-ID","messages":[{"role":"user","content":"你好"}],"stream":true}'
```

### Claude Code（Anthropic 协议）

```bash
set ANTHROPIC_BASE_URL=http://127.0.0.1:8717
set ANTHROPIC_API_KEY=<你的本地Key>
```

代理同时暴露 `/v1/messages`（Anthropic）与 `/v1/responses`（OpenAI Responses）端点，跨协议自动转换（含工具调用与流式）。

### WSL 内使用（VSCode Remote-WSL）

WSL2 里 `127.0.0.1` 不通宿主机，需两步：
1. 「代理设置」页 → 监听地址改为 `0.0.0.0`（**务必同时设置本地 API Key**）
2. WSL 内执行 `ip route show default` 得到网关 IP（即宿主机），Base URL 填 `http://<网关IP>:8717/v1`

## 五、日常使用

- **关闭窗口 = 驻留系统托盘**，代理继续运行；托盘菜单（或单击图标）恢复界面、退出程序。
- **开机自启**：「代理设置」页开关（注册当前用户，无需管理员权限）。
- **请求日志**：每次调用的厂商/模型/耗时/tokens；点「加载历史」可查跨重启的落盘记录。
- **消息录制**：「录制浏览」页开启，完整记录请求/响应原文（JSONL 按天滚动），默认保留 30 天自动清理。
- **用量统计**：按天/厂商/模型的用量与成本估算，可自定义计价规则。
- **重置全部数据**：退出程序后删除 `%APPDATA%\LLMGatewayProxy\` 文件夹。

## 六、版本检查与升级

正式发布构建应注入当前版本号和 Cloudflare Worker 地址：

```powershell
$env:VERSION_REPORT_SECRET = '<从受控密钥管理器读取的 Worker 同一密钥>'
wails build -ldflags "-X main.Version=1.2.3 -X main.VersionReportSigningKey=$env:VERSION_REPORT_SECRET"
```

应用启动时异步检查最新版，并使用 HMAC-SHA256 签名上报版本号、操作系统和架构。签名密钥至少 32 字节，必须与 Worker 的 `VERSION_REPORT_SECRET` 完全一致；通过 `npx wrangler secret put VERSION_REPORT_SECRET` 配置 Worker，不要提交密钥到源码。客户端密钥可被逆向提取，签名只提供基础防伪造/重放能力。Cloudflare 会提供请求来源 IP，服务端按日、版本、平台和 IP 汇总并无限期保留原始 IP；不保存设备标识、API Key 或对话内容。未注入客户端密钥时跳过上报，版本检查照常运行。每次发布时需同步更新 Worker 的最新版、下载链接和更新说明配置。

## 七、安全与合规（务必阅读）

1. **本地 API Key**：建议始终设置（设置页可随机生成），防止本机其它程序/浏览器页面盗用。
2. **监听地址**：默认 `127.0.0.1` 仅本机访问。改为 `0.0.0.0` 意味着局域网内其它设备可访问，**必须**配合 API Key 使用。
3. **厂商 API Key 加密存储**：配置里的各厂商 Key 使用 Windows DPAPI 加密后落盘，仅当前 Windows 用户可解密。
4. **不要把 `%APPDATA%\LLMGatewayProxy\` 目录拷给他人**——内含你的各类 Key（即使加密也绑定你的用户）。

## 八、常见问题

| 现象 | 处理 |
|---|---|
| 启动后无界面 | 安装 WebView2 Runtime（见系统要求） |
| 标题栏玻璃模糊效果异常（发黑/不透明） | WebView2 版本过旧，更新 Evergreen Runtime 即可（不影响功能使用） |
| 双击没反应 | 已有实例在运行（托盘/后台），会自动唤起已有窗口 |
| 端口 8717 被占用 | 设置页改端口后保存（自动重启代理） |
| 改监听地址后弹防火墙提示 | 允许即可（仅专用网络亦可）；或保持 127.0.0.1 |
| WSL 连不上 | 见「四、WSL 内使用」；另确认 Windows 防火墙放行 |

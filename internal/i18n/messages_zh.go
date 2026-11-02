// Package i18n — 中文消息表（基准语言，键集合必须与 messages_en.go 完全一致，i18n_test.go 校验）。
package i18n

var messages = map[string]map[string]string{
	"zh": {
		// ---- 托盘 ----
		"tray.show":    "显示主界面",
		"tray.showTip": "显示主窗口",
		"tray.quit":    "退出",
		"tray.quitTip": "退出应用（代理将停止）",

		// ---- app.go：jsonErr 文案 / 通知 / 对话框 ----
		"app.healthNotInit":       "健康监控未初始化",
		"app.statsNotInit":        "统计未初始化",
		"app.pricingParseFailed":  "规则解析失败",
		"app.readFailed":          "读取失败",
		"app.parseFailed":         "解析失败",
		"app.configParseFailed":   "配置解析失败",
		"app.recorderNotInit":     "录制器未初始化",
		"app.recordDirNotSet":     "录制目录未配置",
		"app.dlg.recordDir":       "选择消息录制目录",
		"app.invalidDate":         "非法日期",
		"app.invalidLine":         "非法行号",
		"app.entryMissing":        "条目不存在（录制文件可能已被删除或重写，请刷新列表）",
		"app.proxyNotInit":        "代理未初始化",
		"app.providerTypeInvalid": "type 必须为 openai / anthropic / gemini",
		"app.baseUrlInvalid":      "BaseURL 必须以 http(s):// 开头",
		"app.proxyNotRunning":     "代理未运行：请先在仪表盘启动代理",
		"app.paramParseFailed":    "参数解析失败",
		"app.loopbackFailed":      "回环请求失败",
		"app.notify.title":        "厂商状态变更",
		"app.notify.recovered":    "恢复",
		"app.notify.down":         "不可用",
		"app.notify.body":         "%s %s（延迟 %dms）",

		// ---- auth ----

		// ---- transform ----
		"transform.sseLineTooLong":      "SSE 行超过 %d 字节上限",
		"transform.prevRespUnsupported": "本代理为无状态转发，不支持 previous_response_id（请客户端自行携带完整 input）",
		"transform.toolTypeUnsupported": "不支持的工具类型 %q（v1 仅支持 function）",
		"transform.inputInvalid":        "input 需为字符串或 items 数组",
		"transform.roleUnsupported":     "不支持的消息 role: %s",

		// ---- provider ----
		"provider.requestFailed": "厂商请求失败",
		"provider.conflict":      "模型 %s：%s 与 %s 都声明了，%s 优先（后者被忽略）",

		// ---- proxy ----
		"proxy.readBodyFailed":           "读取请求体失败",
		"proxy.unrouted":                 "未路由",
		"proxy.registryNotInit":          "路由未初始化",
		"proxy.modelNotFound":            "未找到模型 %q 的路由（也未配置默认厂商）。可用模型：%s",
		"proxy.upstreamFailed":           "上游 %s HTTP %d: %s",
		"proxy.transformFailed":          "响应转换失败",
		"proxy.allUpstreamFailed":        "全部上游失败（含回退）",
		"proxy.unknownError":             "未知错误",
		"proxy.anthropicReqParseFailed":  "Anthropic 请求解析失败",
		"proxy.responsesReqParseFailed":  "Responses 请求解析失败",
		"proxy.openaiReqParseFailed":     "OpenAI 请求解析失败",
		"proxy.anthropicRespParseFailed": "Anthropic 响应解析失败",
		"proxy.geminiRespParseFailed":    "Gemini 响应解析失败",
		"proxy.openaiRespParseFailed":    "OpenAI 响应解析失败",
		"proxy.endpointNotImplBody":      "未实现的端点 %s（Body 前 200 字: %s）",
		"proxy.endpointNotImpl":          "代理未实现 %s %s（支持：/v1/chat/completions、/v1/messages、/v1/messages/count_tokens、/v1/models）",
		"proxy.alreadyRunning":           "代理已在运行",
		"proxy.listenFailed":             "端口 %d 监听失败",
		"proxy.apiKeyRequired":           "本地代理要求正确的 x-api-key（在代理设置中配置）",

		// ---- health / config / autostart / dpapi ----
		"health.noModels":              "模型列表拉取失败（网络或鉴权异常）",
		"config.writeFailed":           "写入配置失败",
		"autostart.registryOpenFailed": "打开注册表 Run 项失败",
		"dpapi.emptyData":              "空数据",
		"dpapi.protectFailed":          "CryptProtectData 失败",
		"dpapi.unprotectFailed":        "CryptUnprotectData 失败",
		"dpapi.emptyOutput":            "DPAPI 输出为空",
	},
}

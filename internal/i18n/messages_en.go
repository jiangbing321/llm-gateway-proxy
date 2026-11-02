// Package i18n — 英文消息表（键集合必须与 messages_zh.go 完全一致，i18n_test.go 校验）。
package i18n

func init() {
	messages["en"] = map[string]string{
		// ---- tray ----
		"tray.show":    "Show main window",
		"tray.showTip": "Show the main window",
		"tray.quit":    "Quit",
		"tray.quitTip": "Quit the app (the proxy will stop)",

		// ---- app.go: jsonErr / notifications / dialogs ----
		"app.healthNotInit":       "Health monitor not initialized",
		"app.statsNotInit":        "Stats not initialized",
		"app.pricingParseFailed":  "Failed to parse pricing rules",
		"app.readFailed":          "Read failed",
		"app.parseFailed":         "Parse failed",
		"app.configParseFailed":   "Failed to parse config",
		"app.recorderNotInit":     "Recorder not initialized",
		"app.recordDirNotSet":     "Recording directory not configured",
		"app.dlg.recordDir":       "Select the recording directory",
		"app.invalidDate":         "Invalid date",
		"app.invalidLine":         "Invalid line number",
		"app.entryMissing":        "Entry not found (the recording file may have been deleted or rewritten; refresh the list)",
		"app.proxyNotInit":        "Proxy not initialized",
		"app.providerTypeInvalid": "type must be openai / anthropic / gemini",
		"app.baseUrlInvalid":      "BaseURL must start with http(s)://",
		"app.proxyNotRunning":     "Proxy not running: start it on the dashboard first",
		"app.paramParseFailed":    "Failed to parse parameters",
		"app.loopbackFailed":      "Loopback request failed",
		"app.notify.title":        "Provider status changed",
		"app.notify.recovered":    "recovered",
		"app.notify.down":         "down",
		"app.notify.body":         "%s is %s (latency %dms)",

		// ---- auth ----

		// ---- transform ----
		"transform.sseLineTooLong":      "SSE line exceeds the %d-byte limit",
		"transform.prevRespUnsupported": "This proxy is stateless and does not support previous_response_id (the client must send the full input)",
		"transform.toolTypeUnsupported": "Unsupported tool type %q (v1 supports function only)",
		"transform.inputInvalid":        "input must be a string or an items array",
		"transform.roleUnsupported":     "Unsupported message role: %s",

		// ---- provider ----
		"provider.requestFailed": "Provider request failed",
		"provider.conflict":      "Model %s: declared by both %s and %s; %s wins (the latter is ignored)",

		// ---- proxy ----
		"proxy.readBodyFailed":           "Failed to read the request body",
		"proxy.unrouted":                 "Unrouted",
		"proxy.registryNotInit":          "Router not initialized",
		"proxy.modelNotFound":            "No route for model %q (and no default provider configured). Available models: %s",
		"proxy.upstreamFailed":           "Upstream %s HTTP %d: %s",
		"proxy.transformFailed":          "Response transform failed",
		"proxy.allUpstreamFailed":        "All upstreams failed (including fallbacks)",
		"proxy.unknownError":             "Unknown error",
		"proxy.anthropicReqParseFailed":  "Failed to parse the Anthropic request",
		"proxy.responsesReqParseFailed":  "Failed to parse the Responses request",
		"proxy.openaiReqParseFailed":     "Failed to parse the OpenAI request",
		"proxy.anthropicRespParseFailed": "Failed to parse the Anthropic response",
		"proxy.geminiRespParseFailed":    "Failed to parse the Gemini response",
		"proxy.openaiRespParseFailed":    "Failed to parse the OpenAI response",
		"proxy.endpointNotImplBody":      "Unimplemented endpoint %s (first 200 chars of body: %s)",
		"proxy.endpointNotImpl":          "Endpoint %s %s not implemented by the proxy (supported: /v1/chat/completions, /v1/messages, /v1/messages/count_tokens, /v1/models)",
		"proxy.alreadyRunning":           "Proxy is already running",
		"proxy.listenFailed":             "Failed to listen on port %d",
		"proxy.apiKeyRequired":           "The local proxy requires a valid x-api-key (configured in proxy settings)",

		// ---- health / config / autostart / dpapi ----
		"health.noModels":              "Failed to fetch the model list (network or auth error)",
		"config.writeFailed":           "Failed to write config",
		"autostart.registryOpenFailed": "Failed to open the registry Run key",
		"dpapi.emptyData":              "Empty data",
		"dpapi.protectFailed":          "CryptProtectData failed",
		"dpapi.unprotectFailed":        "CryptUnprotectData failed",
		"dpapi.emptyOutput":            "DPAPI returned empty output",
	}
}

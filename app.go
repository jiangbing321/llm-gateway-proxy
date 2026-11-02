package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"llm-gateway-proxy/internal/autostart"
	"llm-gateway-proxy/internal/config"
	"llm-gateway-proxy/internal/health"
	"llm-gateway-proxy/internal/i18n"
	"llm-gateway-proxy/internal/logbuf"
	"llm-gateway-proxy/internal/paths"
	"llm-gateway-proxy/internal/provider"
	"llm-gateway-proxy/internal/proxy"
	"llm-gateway-proxy/internal/recorder"
	"llm-gateway-proxy/internal/singleinstance"
	"llm-gateway-proxy/internal/stats"
	"llm-gateway-proxy/internal/transform"
)

// App 是 Wails 绑定的应用对象。
type App struct {
	ctx       context.Context
	cfgMgr    *config.Manager
	logs      *logbuf.Ring
	rec       *recorder.Recorder
	stats     *stats.Stats
	healthMon *health.Monitor
	proxy     *proxy.Server
	registry  *provider.Registry
	quitting  atomic.Bool // 真正退出标志（区别于"关闭=驻留托盘"）
	instance  *singleinstance.Instance
	logWriter io.WriteCloser // app.log 轮转写句柄（退出时关闭）
}

// NewApp 组装应用（不包含 wails ctx 依赖的部分）。
func NewApp() *App {
	return &App{}
}

// startup 在 Wails 启动后初始化各服务。
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// GUI 模式无控制台：日志写文件便于排查；超 10MB 自动轮转（启动时 + 运行中）
	if dir, err := paths.DataDir(); err == nil {
		logbuf.RotateFile(filepath.Join(dir, "app.log"), 10<<20) // 清理上次运行的遗留增长
		if w, err := logbuf.OpenRotatingLog(filepath.Join(dir, "app.log"), 10<<20); err == nil {
			a.logWriter = w
			log.SetOutput(w)
			log.Printf("[app] 日志初始化完成")
		}
	}

	cfgMgr, err := config.NewManager()
	if err != nil {
		log.Printf("[app] 配置加载失败（使用默认配置）: %v", err)
		cfgMgr = mustDefaultConfig()
	}
	a.cfgMgr = cfgMgr
	// 语言先于网关/托盘初始化：托盘菜单与后续错误/通知文案均按当前语言取词
	i18n.SetLang(cfgMgr.Get().Language)

	a.logs = logbuf.NewRing(200)
	if dir, err := paths.DataDir(); err == nil {
		logbuf.RotateFile(filepath.Join(dir, "proxy-access.log"), 10<<20)
		if err := a.logs.EnableFileLog(filepath.Join(dir, "proxy-access.log"), 10<<20); err != nil {
			log.Printf("[app] 访问日志落盘失败: %v", err)
		}
	}
	// 用量统计（聚合 + 节流落盘）
	if st, err := stats.New(); err == nil {
		a.stats = st
		a.logs.Subscribe(func(e logbuf.Entry) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "log:new", e)
			}
			st.Record(e)
		})
	} else {
		log.Printf("[app] 统计初始化失败: %v", err)
		a.logs.Subscribe(func(e logbuf.Entry) {
			if a.ctx != nil {
				runtime.EventsEmit(a.ctx, "log:new", e)
			}
		})
	}

	// 多厂商路由注册表（用户配置）
	a.registry = provider.NewRegistry()
	a.rebuildRegistry()

	a.rec = recorder.New()
	cfgNow := a.cfgMgr.Get()
	a.rec.Configure(cfgNow.RecordEnabled, cfgNow.RecordDir)
	// 录制文件按保留天数自动清理（防磁盘无限增长）
	if removed := recorder.CleanOld(cfgNow.RecordDir, cfgNow.RecordRetentionDays); len(removed) > 0 {
		log.Printf("[app] 已清理 %d 个超期录制文件（保留 %d 天）: %v",
			len(removed), cfgNow.RecordRetentionDays, removed)
	}

	a.proxy = proxy.NewServer(a.cfgMgr.Get(), a.logs, a.rec, a.registry)
	a.proxy.OnStateChange(func(running bool, addr string, err error) {
		if a.ctx == nil {
			return
		}
		runtime.EventsEmit(a.ctx, "proxy:status", map[string]any{
			"running": running, "addr": addr, "error": errString(err),
		})
	})
	if a.cfgMgr.Get().ProxyEnabled {
		if err := a.proxy.Start(); err != nil {
			log.Printf("[app] 代理启动失败: %v", err)
		}
	}

	// 厂商健康监控（探活 + 状态翻转通知）
	hc := a.cfgMgr.Get()
	a.healthMon = health.NewMonitor(func() *provider.Registry { return a.registry },
		func(st health.Status) {
			if a.ctx == nil {
				return
			}
			runtime.EventsEmit(a.ctx, "health:change", st)
			// Windows 通知（权限未授予时静默失败，应用内事件仍生效）
			verb := i18n.T("app.notify.recovered")
			if st.State == "down" {
				verb = i18n.T("app.notify.down")
			}
			_ = runtime.SendNotification(a.ctx, runtime.NotificationOptions{
				Title: i18n.T("app.notify.title"),
				Body:  i18n.T("app.notify.body", st.ProviderName, verb, st.LatencyMS),
			})
		})
	if hc.HealthEnabled {
		a.applyHealthConfig(hc)
	}

	// 系统托盘：关闭窗口后驻留，托盘菜单控制显示/退出
	startTray(a.ShowMainWindow, a.requestQuit)

	// 二次启动唤醒：单实例 Event 到达 → 显示主窗口
	if a.instance != nil {
		go func() {
			for a.instance.WaitShowSignal() {
				if a.ctx != nil {
					runtime.WindowShow(a.ctx)
				}
			}
		}()
	}
}

// shutdown 清理。
func (a *App) shutdown(ctx context.Context) {
	if a.proxy != nil {
		a.proxy.Stop()
	}
	if a.rec != nil {
		a.rec.Close()
	}
	if a.instance != nil {
		a.instance.Release()
	}
	if a.stats != nil {
		a.stats.Flush()
	}
	if a.healthMon != nil {
		a.healthMon.Stop()
	}
	if a.logs != nil {
		a.logs.CloseFileLog()
	}
	if a.logWriter != nil {
		_ = a.logWriter.Close()
	}
}

// GetHealth 返回全部厂商健康状态。
func (a *App) GetHealth() string {
	if a.healthMon == nil {
		return jsonOK([]health.Status{})
	}
	return jsonOK(a.healthMon.Snapshot())
}

// CheckHealthNow 立即探活一次（手动刷新）。
func (a *App) CheckHealthNow() string {
	if a.healthMon == nil {
		return jsonErr(errors.New(i18n.T("app.healthNotInit")))
	}
	a.healthMon.CheckAll(a.ctx)
	return jsonOK(a.healthMon.Snapshot())
}

// GetStats 返回近 N 天用量聚合（含成本估算）。
func (a *App) GetStats(days int) string {
	if a.stats == nil {
		return jsonErr(errors.New(i18n.T("app.statsNotInit")))
	}
	return jsonOK(a.stats.GetStats(days))
}

// GetPricing 返回计价规则。
func (a *App) GetPricing() string {
	if a.stats == nil {
		return jsonErr(errors.New(i18n.T("app.statsNotInit")))
	}
	return jsonOK(a.stats.GetPricing())
}

// SavePricing 保存计价规则（JSON 数组字符串）。
func (a *App) SavePricing(rulesJSON string) string {
	if a.stats == nil {
		return jsonErr(errors.New(i18n.T("app.statsNotInit")))
	}
	var rules []stats.PricingRule
	if err := json.Unmarshal([]byte(rulesJSON), &rules); err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.pricingParseFailed"), err))
	}
	a.stats.SetPricing(rules)
	return jsonOK(rules)
}

// SetAutoStart 开/关开机自启（注册表 Run 项 + 配置记录）。
func (a *App) SetAutoStart(on bool) string {
	var err error
	if on {
		err = autostart.Enable()
	} else {
		err = autostart.Disable()
	}
	if err != nil {
		return jsonErr(err)
	}
	_, _ = a.cfgMgr.Update(func(c *config.Config) { c.AutoStart = on })
	return jsonOK(map[string]bool{"enabled": on})
}

// GetAutoStart 查询自启状态（以系统实际状态为准）。
func (a *App) GetAutoStart() string {
	return jsonOK(map[string]bool{"enabled": autostart.IsEnabled()})
}

// rebuildRegistry 按当前配置重建路由注册表并同步给代理。
func (a *App) rebuildRegistry() {
	cfg := a.cfgMgr.Get()
	a.registry.Rebuild(cfg.Providers, cfg.DefaultProviderID)
	if a.proxy != nil {
		a.proxy.UpdateRegistry(a.registry)
	}
}

// mustDefaultConfig 提供兜底配置管理器（数据目录不可写等极端情况）。
func mustDefaultConfig() *config.Manager {
	m, err := config.NewInMemoryDefault()
	if err != nil {
		panic(err) // 理论不可达
	}
	return m
}

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// ---- 绑定给前端的方法（返回 JSON 字符串，同 askit 风格） ----

func jsonOK(data any) string {
	raw, _ := json.Marshal(map[string]any{"success": true, "data": data})
	return string(raw)
}

func jsonErr(err error) string {
	raw, _ := json.Marshal(map[string]any{"success": false, "error": err.Error()})
	return string(raw)
}

// GetStatus 返回综合状态。
func (a *App) GetStatus() string {
	type status struct {
		ProxyRunning bool   `json:"proxyRunning"`
		ProxyURL     string `json:"proxyURL"`
		ProxyErr     string `json:"proxyError,omitempty"`
		Port         int    `json:"port"`
	}
	st := status{Port: a.cfgMgr.Get().Port}
	if a.proxy != nil {
		st.ProxyRunning = a.proxy.IsRunning()
		if st.ProxyRunning {
			st.ProxyURL = "http://" + a.proxy.Addr()
		}
	}
	return jsonOK(st)
}

// GetConfig 读取配置。
func (a *App) GetConfig() string {
	return jsonOK(a.cfgMgr.Get())
}

// SaveConfig 保存配置（JSON 字符串），端口/开关变化时重启代理。
// 厂商列表与默认路由不在保存范围内（由 SaveProvider/DeleteProvider/SetDefaultProvider 维护）：
// 前端各页面持有的是启动时的配置快照，若在此覆盖会把新添加的厂商冲掉。
func (a *App) SaveConfig(cfgJSON string) string {
	var in config.Config
	if err := json.Unmarshal([]byte(cfgJSON), &in); err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.configParseFailed"), err))
	}
	old := a.cfgMgr.Get()
	saved, err := a.cfgMgr.Update(func(c *config.Config) {
		in.Providers = c.Providers
		in.DefaultProviderID = c.DefaultProviderID
		*c = in
	})
	if err != nil {
		return jsonErr(err)
	}
	// 语言变化 → 后端文案/通知即时切换；托盘菜单不做运行时改词（跨线程 SetTitle
	// 与 TrackPopupMenu 并发会死锁托盘消息循环），重启后按新语言创建。
	if saved.Language != old.Language {
		i18n.SetLang(saved.Language)
	}

	if a.proxy != nil && (in.Port != old.Port || in.Bind != old.Bind || in.ProxyEnabled != old.ProxyEnabled) {
		if in.ProxyEnabled {
			if err := a.proxy.Restart(saved); err != nil {
				return jsonErr(err)
			}
		} else {
			a.proxy.Stop()
		}
	} else if a.proxy != nil {
		a.proxy.UpdateConfig(saved) // 仅 API key / 录制配置 变化
	}
	// 录制开关/目录热更新（无需重启代理）
	if a.rec != nil {
		a.rec.Configure(saved.RecordEnabled, saved.RecordDir)
	}
	// 健康监控开关/间隔变化 → 热启停（幂等：Start 已运行时为 no-op）
	if in.HealthEnabled != old.HealthEnabled || in.HealthIntervalSec != old.HealthIntervalSec {
		a.applyHealthConfig(saved)
	}
	return jsonOK(saved)
}

// applyHealthConfig 按配置同步健康监控：开→设间隔并启动；关→停止。
func (a *App) applyHealthConfig(cfg config.Config) {
	if a.healthMon == nil {
		return
	}
	if cfg.HealthEnabled {
		a.healthMon.SetInterval(time.Duration(cfg.HealthIntervalSec) * time.Second)
		a.healthMon.Start()
	} else {
		a.healthMon.Stop()
	}
}

// GetRecordInfo 返回录制状态：开关、目录、当天文件路径与大小。
func (a *App) GetRecordInfo() string {
	if a.rec == nil {
		return jsonErr(errors.New(i18n.T("app.recorderNotInit")))
	}
	file := a.rec.CurrentFile()
	var size int64
	if st, err := os.Stat(file); err == nil {
		size = st.Size()
	}
	return jsonOK(map[string]any{
		"enabled": a.rec.Enabled(),
		"file":    file,
		"size":    size,
	})
}

// PickRecordDir 选择录制目录。
func (a *App) PickRecordDir() string {
	dir, err := runtime.OpenDirectoryDialog(a.ctx, runtime.OpenDialogOptions{
		Title: i18n.T("app.dlg.recordDir"),
	})
	if err != nil {
		return jsonErr(err)
	}
	return jsonOK(map[string]string{"dir": dir})
}

// OpenRecordDir 在资源管理器中打开录制目录。
func (a *App) OpenRecordDir() string {
	dir := a.cfgMgr.Get().RecordDir
	if dir == "" {
		return jsonErr(errors.New(i18n.T("app.recordDirNotSet")))
	}
	if err := exec.Command("explorer", dir).Start(); err != nil {
		return jsonErr(err)
	}
	return jsonOK(nil)
}

// ---- 录制文件浏览器 ----

// RecordingFile 是一个录制文件的信息。
type RecordingFile struct {
	Date      string `json:"date"`
	SizeBytes int64  `json:"sizeBytes"`
	Lines     int    `json:"lines"`
}

// RecordingEntry 是浏览视图中的一条记录。
// 列表接口（ReadRecordings）只填元信息 + Line 行号，不携带 Body；
// 正文由 ReadRecordingEntry 按行号按需拉取（完整不截断），
// 避免整批 body（单条可达数十 MB）的内存与 IPC 放大。
type RecordingEntry struct {
	Time      time.Time `json:"ts"`
	Dir       string    `json:"dir"`
	Model     string    `json:"model"`
	Status    int       `json:"status"`
	Stream    bool      `json:"stream"`
	Transform string    `json:"transform,omitempty"`
	Body      string    `json:"body,omitempty"`
	Size      int       `json:"size,omitempty"` // 原始消息正文字节数（列表侧栏展示用）
	Line      int       `json:"line"`
}

// ListRecordings 列出录制文件（按日期倒序）。
func (a *App) ListRecordings() string {
	dir := a.cfgMgr.Get().RecordDir
	if dir == "" {
		return jsonOK([]RecordingFile{})
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return jsonOK([]RecordingFile{})
		}
		return jsonErr(err)
	}
	var out []RecordingFile
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasPrefix(name, "llm-record-") || !strings.HasSuffix(name, ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		date := strings.TrimSuffix(strings.TrimPrefix(name, "llm-record-"), ".jsonl")
		out = append(out, RecordingFile{
			Date: date, SizeBytes: info.Size(),
			Lines: countLines(filepath.Join(dir, name)),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Date > out[j].Date })
	if out == nil {
		out = []RecordingFile{}
	}
	return jsonOK(out)
}

func countLines(path string) int {
	// 流式计数：录制文件可能达几十 MB，避免整个读入内存
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	n := 0
	buf := make([]byte, 64<<10)
	for {
		c, err := f.Read(buf)
		n += bytes.Count(buf[:c], []byte{'\n'})
		if err != nil {
			break
		}
	}
	return n
}

// ReadRecordings 读取指定日期的录制内容（keyword 过滤，倒序，body 截断）。
// 流式扫描 + 定长环形缓冲只保留最近 maxEntries 条命中记录——
// 内存峰值与文件大小无关（此前整文件读入 + Split 复制，录制文件可达几十 MB）。
func (a *App) ReadRecordings(date, keyword string, maxEntries int) string {
	if maxEntries <= 0 || maxEntries > 2000 {
		maxEntries = 200
	}
	if !regexpDate.MatchString(date) {
		return jsonErr(errors.New(i18n.T("app.invalidDate")))
	}
	path := filepath.Join(a.cfgMgr.Get().RecordDir, "llm-record-"+date+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.readFailed"), err))
	}
	defer f.Close()

	kw := strings.ToLower(keyword)
	ring := make([]RecordingEntry, maxEntries)
	n := 0      // 命中条数
	lineNo := 0 // 全文件行号（含未命中行，作为详情拉取的稳定定位）
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 100<<20) // 单条可能含完整请求/响应体（上限 64MB 响应 + JSON 转义）
	for sc.Scan() {
		lineNo++
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e recorder.Entry
		if json.Unmarshal(line, &e) != nil {
			continue
		}
		if kw != "" && !strings.Contains(strings.ToLower(e.Body), kw) &&
			!strings.Contains(strings.ToLower(e.Model), kw) {
			continue
		}
		ring[n%maxEntries] = RecordingEntry{
			Time: e.Time, Dir: e.Dir, Model: e.Model, Status: e.Status,
			Stream: e.Stream, Transform: e.Transform, Size: len(e.Body), Line: lineNo,
		}
		n++
	}
	if err := sc.Err(); err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.readFailed"), err))
	}

	total := n
	if total > maxEntries {
		total = maxEntries
	}
	out := make([]RecordingEntry, 0, total) // 最新在前
	for i := 0; i < total; i++ {
		out = append(out, ring[(n-1-i)%maxEntries])
	}
	return jsonOK(out)
}

// ReadRecordingEntry 读取指定日期录制文件第 line 行（1 起、全文件计数）的完整内容（不截断）。
// 行号由 ReadRecordings 返回；录制文件只追加，行号对旧条目稳定。
func (a *App) ReadRecordingEntry(date string, line int) string {
	if !regexpDate.MatchString(date) {
		return jsonErr(errors.New(i18n.T("app.invalidDate")))
	}
	if line < 1 {
		return jsonErr(errors.New(i18n.T("app.invalidLine")))
	}
	path := filepath.Join(a.cfgMgr.Get().RecordDir, "llm-record-"+date+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.readFailed"), err))
	}
	defer f.Close()

	n := 0
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64<<10), 100<<20)
	for sc.Scan() {
		n++
		if n != line {
			continue
		}
		var e recorder.Entry
		if err := json.Unmarshal(sc.Bytes(), &e); err != nil {
			return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.parseFailed"), err))
		}
		return jsonOK(RecordingEntry{
			Time: e.Time, Dir: e.Dir, Model: e.Model, Status: e.Status,
			Stream: e.Stream, Transform: e.Transform, Body: e.Body, Size: len(e.Body), Line: n,
		})
	}
	if err := sc.Err(); err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.readFailed"), err))
	}
	return jsonErr(errors.New(i18n.T("app.entryMissing")))
}

// DeleteRecording 删除指定日期的录制文件。
func (a *App) DeleteRecording(date string) string {
	if !regexpDate.MatchString(date) {
		return jsonErr(errors.New(i18n.T("app.invalidDate")))
	}
	// 优先走 recorder：删除当天活动文件前先释放其句柄（Windows 共享冲突删不掉）
	if a.rec != nil {
		if err := a.rec.Delete(date); err != nil {
			return jsonErr(err)
		}
		return jsonOK(nil)
	}
	path := filepath.Join(a.cfgMgr.Get().RecordDir, "llm-record-"+date+".jsonl")
	if err := os.Remove(path); err != nil {
		return jsonErr(err)
	}
	return jsonOK(nil)
}

var regexpDate = regexp.MustCompile(`^\d{8}$`)

// SetProxyRunning 开关代理。
func (a *App) SetProxyRunning(on bool) string {
	if a.proxy == nil {
		return jsonErr(errors.New(i18n.T("app.proxyNotInit")))
	}
	if on {
		if err := a.proxy.Restart(a.cfgMgr.Get()); err != nil {
			return jsonErr(err)
		}
	} else {
		a.proxy.Stop()
	}
	return jsonOK(a.proxy.IsRunning())
}

// ListModels 返回聚合模型列表（全部启用的 provider）。
func (a *App) ListModels() string {
	if a.registry == nil {
		return jsonErr(errors.New(i18n.T("proxy.registryNotInit")))
	}
	type item struct {
		ID           string `json:"id"`
		DisplayName  string `json:"display_name"`
		Type         string `json:"type"`
		Provider     string `json:"provider"`
		ProviderName string `json:"provider_name"`
	}
	var out []item
	for _, p := range a.registry.Providers() {
		if !p.Enabled() {
			continue
		}
		for _, m := range p.Models(a.ctx) {
			out = append(out, item{
				ID: m.ID, DisplayName: m.DisplayName, Type: m.Type,
				Provider: p.ID(), ProviderName: p.Name(),
			})
		}
	}
	return jsonOK(out)
}

// ---- Provider 管理 ----

// ListProviders 列出全部 provider（含路由冲突信息）。
func (a *App) ListProviders() string {
	cfg := a.cfgMgr.Get()
	out := make([]config.Provider, len(cfg.Providers))
	copy(out, cfg.Providers)
	var conflicts []string
	if a.registry != nil {
		conflicts = a.registry.Conflicts()
	}
	if conflicts == nil {
		conflicts = []string{}
	}
	return jsonOK(map[string]any{
		"providers": out, "defaultProviderId": cfg.DefaultProviderID, "conflicts": conflicts,
	})
}

// SaveProvider 新增或更新一个 provider 配置。
func (a *App) SaveProvider(pJSON string) string {
	var in config.Provider
	if err := json.Unmarshal([]byte(pJSON), &in); err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.parseFailed"), err))
	}
	if in.Type != "openai" && in.Type != "anthropic" && in.Type != "gemini" {
		return jsonErr(errors.New(i18n.T("app.providerTypeInvalid")))
	}
	if in.BaseURL == "" || !strings.HasPrefix(in.BaseURL, "http") {
		return jsonErr(errors.New(i18n.T("app.baseUrlInvalid")))
	}
	_, err := a.cfgMgr.Update(func(c *config.Config) {
		if in.ID == "" {
			in.ID = fmt.Sprintf("prov_%d", time.Now().UnixNano()%1e8)
		}
		replaced := false
		for i, p := range c.Providers {
			if p.ID == in.ID {
				c.Providers[i] = in
				replaced = true
				break
			}
		}
		if !replaced {
			c.Providers = append(c.Providers, in)
		}
	})
	if err != nil {
		return jsonErr(err)
	}
	a.rebuildRegistry()
	// 检测保存后的路由冲突，作为 warning 返回（不影响保存）
	var warnings []string
	if a.registry != nil {
		for _, c := range a.registry.Conflicts() {
			if strings.Contains(c, in.Name) {
				warnings = append(warnings, c)
			}
		}
	}
	return jsonOK(map[string]any{"provider": in, "warnings": warnings})
}

// DeleteProvider 删除一个 provider 配置。
func (a *App) DeleteProvider(id string) string {
	_, err := a.cfgMgr.Update(func(c *config.Config) {
		kept := c.Providers[:0]
		for _, p := range c.Providers {
			if p.ID != id {
				kept = append(kept, p)
			}
		}
		c.Providers = kept
		if c.DefaultProviderID == id {
			c.DefaultProviderID = "" // 无默认 = 未命中模型名直接 404
		}
	})
	if err != nil {
		return jsonErr(err)
	}
	a.rebuildRegistry()
	return jsonOK(nil)
}

// SetDefaultProvider 设置默认路由 provider（空串 = 取消默认）。
func (a *App) SetDefaultProvider(id string) string {
	_, err := a.cfgMgr.Update(func(c *config.Config) {
		c.DefaultProviderID = ""
		for _, p := range c.Providers {
			if p.ID == id {
				c.DefaultProviderID = id
				return
			}
		}
	})
	if err != nil {
		return jsonErr(err)
	}
	a.rebuildRegistry()
	return jsonOK(nil)
}

// TestProvider 用给定配置（可未保存）发一个最小请求验证连通与鉴权。
func (a *App) TestProvider(pJSON string) string {
	var in config.Provider
	if err := json.Unmarshal([]byte(pJSON), &in); err != nil {
		return jsonErr(err)
	}
	var p provider.Provider
	switch in.Type {
	case "openai":
		p = provider.NewOpenAIProvider(in)
	case "anthropic":
		p = provider.NewAnthropicProvider(in)
	case "gemini":
		p = provider.NewGeminiProvider(in)
	default:
		return jsonErr(errors.New(i18n.T("app.providerTypeInvalid")))
	}
	// 构造最小请求：统一先建 OpenAI 中间表示，再按上游协议转换
	probe := &transform.OAIChatRequest{
		Model: firstModel(in),
		Messages: []transform.OAIMessage{
			{Role: "user", Content: json.RawMessage(`"hi"`)},
		},
	}
	var body []byte
	var err error
	switch p.Protocol() {
	case provider.ProtocolAnthropic:
		aReq, cerr := transform.ReqO2A(probe, 8192)
		if cerr != nil {
			return jsonErr(cerr)
		}
		body, err = json.Marshal(aReq)
	case provider.ProtocolGemini:
		gReq, cerr := transform.ReqO2G(probe)
		if cerr != nil {
			return jsonErr(cerr)
		}
		body, err = json.Marshal(gReq)
	default:
		body, err = json.Marshal(map[string]any{
			"model": firstModel(in), "max_tokens": 5, "stream": false,
			"messages": []map[string]string{{"role": "user", "content": "hi"}},
		})
	}
	if err != nil {
		return jsonErr(err)
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	resp, err := p.Do(ctx, firstModel(in), body, false)
	if err != nil {
		return jsonErr(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return jsonOK(map[string]any{
		"httpStatus": resp.StatusCode,
		"ok":         resp.StatusCode == 200,
		"sample":     truncateStr(string(raw), 200),
	})
}

// FetchProviderModels 从厂商拉取模型列表（用给定配置，可未保存）。
func (a *App) FetchProviderModels(pJSON string) string {
	var in config.Provider
	if err := json.Unmarshal([]byte(pJSON), &in); err != nil {
		return jsonErr(err)
	}
	var fetcher interface {
		FetchModels(ctx context.Context) ([]provider.ModelInfo, error)
	}
	switch in.Type {
	case "openai":
		fetcher = provider.NewOpenAIProvider(in)
	case "anthropic":
		fetcher = provider.NewAnthropicProvider(in)
	case "gemini":
		fetcher = provider.NewGeminiProvider(in)
	default:
		return jsonErr(errors.New(i18n.T("app.providerTypeInvalid")))
	}
	ctx, cancel := context.WithTimeout(a.ctx, 30*time.Second)
	defer cancel()
	models, err := fetcher.FetchModels(ctx)
	if err != nil {
		return jsonErr(err)
	}
	return jsonOK(models)
}

// firstModel 取 provider 的第一个模型；未配置时按类型给出各厂商有效的兜底模型名，
// 避免对 anthropic/gemini 发 openai 模型名导致"模型不存在"误报（掩盖真实错误）。
func firstModel(p config.Provider) string {
	if len(p.Models) > 0 {
		return p.Models[0]
	}
	switch p.Type {
	case "anthropic":
		return "claude-3-5-haiku-latest"
	case "gemini":
		return "gemini-2.0-flash"
	default: // openai / gateway
		return "gpt-4o-mini"
	}
}

// ---- Playground ----

// PlaygroundChat 发起真实回环请求（走完整路由+转换+录制链路）。
// 流式时逐 delta 发 "playground:delta" 事件，结束返回汇总 JSON。
func (a *App) PlaygroundChat(reqJSON string) string {
	cfg := a.cfgMgr.Get()
	if a.proxy == nil || !a.proxy.IsRunning() {
		return jsonErr(errors.New(i18n.T("app.proxyNotRunning")))
	}
	url := fmt.Sprintf("http://127.0.0.1:%d/v1/chat/completions", cfg.Port)

	var in struct {
		Model     string `json:"model"`
		Stream    bool   `json:"stream"`
		MaxTokens int    `json:"max_tokens"`
		System    string `json:"system"`
		Messages  []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal([]byte(reqJSON), &in); err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.paramParseFailed"), err))
	}
	// 组装 OpenAI 请求（Playground 统一说 OpenAI 协议，转换由代理负责）
	msgs := []map[string]string{}
	if in.System != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": in.System})
	}
	for _, m := range in.Messages {
		msgs = append(msgs, map[string]string{"role": m.Role, "content": m.Content})
	}
	body, _ := json.Marshal(map[string]any{
		"model": in.Model, "messages": msgs, "stream": in.Stream,
		"max_tokens": in.MaxTokens,
	})

	req, err := http.NewRequestWithContext(a.ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return jsonErr(err)
	}
	req.Header.Set("Content-Type", "application/json")
	if cfg.LocalAPIKey != "" {
		req.Header.Set("x-api-key", cfg.LocalAPIKey)
	}

	start := time.Now()
	resp, err := sharedPlaygroundHTTP.Do(req)
	if err != nil {
		return jsonErr(fmt.Errorf("%s: %w", i18n.T("app.loopbackFailed"), err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return jsonErr(fmt.Errorf("HTTP %d: %s", resp.StatusCode, truncateStr(string(raw), 300)))
	}

	if !in.Stream {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
			Usage *struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
			} `json:"usage"`
		}
		_ = json.Unmarshal(raw, &out)
		content := ""
		if len(out.Choices) > 0 {
			content = out.Choices[0].Message.Content
		}
		var pt, ct int
		if out.Usage != nil {
			pt, ct = out.Usage.PromptTokens, out.Usage.CompletionTokens
		}
		if a.ctx != nil {
			runtime.EventsEmit(a.ctx, "playground:delta", content)
		}
		return jsonOK(map[string]any{
			"content": content, "model": in.Model, "durationMs": time.Since(start).Milliseconds(),
			"promptTokens": pt, "completionTokens": ct, "stream": false,
		})
	}

	// 流式：逐事件转发 delta
	er := transform.NewEventReader(resp.Body)
	var full strings.Builder
	var pt, ct int
	for {
		ev, eerr := er.Next()
		if ev != nil && ev.Data != "[DONE]" {
			var chunk struct {
				Choices []struct {
					Delta struct {
						Content string `json:"content"`
					} `json:"delta"`
				} `json:"choices"`
				Usage *struct {
					PromptTokens     int `json:"prompt_tokens"`
					CompletionTokens int `json:"completion_tokens"`
				} `json:"usage"`
			}
			if json.Unmarshal([]byte(ev.Data), &chunk) == nil {
				if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
					full.WriteString(chunk.Choices[0].Delta.Content)
					if a.ctx != nil {
						runtime.EventsEmit(a.ctx, "playground:delta", chunk.Choices[0].Delta.Content)
					}
				}
				if chunk.Usage != nil {
					pt, ct = chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens
				}
			}
		}
		if eerr != nil {
			break
		}
	}
	return jsonOK(map[string]any{
		"content": full.String(), "model": in.Model, "durationMs": time.Since(start).Milliseconds(),
		"promptTokens": pt, "completionTokens": ct, "stream": true,
	})
}

// sharedPlaygroundHTTP 回环请求客户端（无超时上限依赖 ctx；SSE 长流）。
var sharedPlaygroundHTTP = &http.Client{}

// GetLogs 最近日志。
func (a *App) GetLogs() string {
	return jsonOK(a.logs.Snapshot())
}

// GetLogHistory 从 proxy-access.log（落盘 JSONL）读取最近 n 条历史日志，
// 含跨重启的记录——内存环形缓冲只有本次运行的最近 200 条。
// 历史条目的 ID 重编号为负数，避免与本次运行实时推送的正数 ID 在前端 v-for key 冲突。
func (a *App) GetLogHistory(n int) string {
	if n <= 0 || n > 2000 {
		n = 200
	}
	dir, err := paths.DataDir()
	if err != nil {
		return jsonErr(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "proxy-access.log"))
	if err != nil {
		if os.IsNotExist(err) {
			return jsonOK([]logbuf.Entry{})
		}
		return jsonErr(err)
	}
	var out []logbuf.Entry
	lines := strings.Split(string(raw), "\n")
	for i := len(lines) - 1; i >= 0 && len(out) < n; i-- {
		if lines[i] == "" {
			continue
		}
		var e logbuf.Entry
		if json.Unmarshal([]byte(lines[i]), &e) == nil {
			out = append(out, e)
		}
	}
	// 倒序收集（新→旧）反转为旧→新，并重编 ID 为负数
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	for i := range out {
		out[i].ID = -(int64(i) + 1)
	}
	if out == nil {
		out = []logbuf.Entry{}
	}
	return jsonOK(out)
}

// ClearLogs 清空日志。
func (a *App) ClearLogs() string {
	a.logs.Clear()
	return jsonOK(nil)
}

// CopyText 写剪贴板。
func (a *App) CopyText(text string) string {
	if a.ctx != nil {
		runtime.ClipboardSetText(a.ctx, text)
	}
	return jsonOK(nil)
}

// ---- 无边框窗口控制（同 askit） ----

// WindowToggleMaximise 切换最大化。
func (a *App) WindowToggleMaximise() {
	runtime.WindowToggleMaximise(a.ctx)
}

// WindowMinimise 最小化。
func (a *App) WindowMinimise() {
	runtime.WindowMinimise(a.ctx)
}

// WindowClose 关闭（隐藏到托盘）。
func (a *App) WindowClose() {
	runtime.WindowHide(a.ctx)
}

// IsQuitting 返回是否处于真正退出流程（OnBeforeClose 依据它放行关闭）。
func (a *App) IsQuitting() bool {
	return a.quitting.Load()
}

// requestQuit 真正退出应用：置标志（OnBeforeClose 放行）→ 停代理 → wails Quit。
// 兜底：若 wails 退出流程 1 秒内未结束进程，强制 os.Exit。
func (a *App) requestQuit() {
	if !a.quitting.CompareAndSwap(false, true) {
		return // 已在退出中
	}
	if a.proxy != nil {
		a.proxy.Stop()
	}
	if a.ctx != nil {
		runtime.Quit(a.ctx)
	}
	go func() {
		time.Sleep(1 * time.Second)
		os.Exit(0)
	}()
}

// ShowMainWindow 从托盘恢复窗口。
func (a *App) ShowMainWindow() {
	runtime.WindowShow(a.ctx)
}

// GetHostIPs 列出本机网卡 IPv4（标注 WSL 虚拟网卡），供 WSL 内拼访问地址。
func (a *App) GetHostIPs() string {
	type nic struct {
		Name string `json:"name"`
		IP   string `json:"ip"`
		WSL  bool   `json:"wsl"`
	}
	var nics []nic
	ifaces, err := net.Interfaces()
	if err == nil {
		for _, ifc := range ifaces {
			if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
				continue
			}
			addrs, err := ifc.Addrs()
			if err != nil {
				continue
			}
			for _, addr := range addrs {
				ipnet, ok := addr.(*net.IPNet)
				if !ok || ipnet.IP.To4() == nil {
					continue
				}
				name := ifc.Name
				nics = append(nics, nic{
					Name: name,
					IP:   ipnet.IP.String(),
					WSL:  strings.Contains(strings.ToLower(name), "wsl"),
				})
			}
		}
	}
	return jsonOK(nics)
}

func truncateStr(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "..."
}

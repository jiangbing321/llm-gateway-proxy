package main

import (
	_ "embed"
	"log"

	"github.com/energye/systray"

	"llm-gateway-proxy/internal/i18n"
)

//go:embed build/tray.ico
var trayIconICO []byte

// startTray 启动系统托盘（goroutine 中运行，与 wails 主循环并行）。
// showFn 恢复主窗口；quitFn 完整退出应用（由 App.requestQuit 实现）。
// 菜单文案按启动时语言创建（i18n.T）——运行时不做 SetTitle 动态改词：
// 跨线程修改托盘菜单与 TrackPopupMenu 模态循环并发会死锁消息循环
// （WndProc 卡死→右键菜单弹不出），语言切换重启后生效。
func startTray(showFn func(), quitFn func()) {
	go func() {
		systray.Run(func() {
			systray.SetIcon(trayIconICO)
			systray.SetTooltip("LLM Gateway Proxy") // 产品名，不翻译
			systray.SetTitle("GW-Proxy")

			mShow := systray.AddMenuItem(i18n.T("tray.show"), i18n.T("tray.showTip"))
			mQuit := systray.AddMenuItem(i18n.T("tray.quit"), i18n.T("tray.quitTip"))

			mShow.Click(showFn)
			// 左键单击即显示主界面：energye/systray 的窗口类缺 CS_DBLCLKS 样式，
			// Windows 不发送 WM_LBUTTONDBLCLK（SetOnDClick 收不到）；改用单击触发，
			// 双击回调保留兜底（若库未来修复）。
			systray.SetOnClick(func(_ systray.IMenu) {
				showFn()
			})
			systray.SetOnDClick(func(_ systray.IMenu) {
				showFn()
			})
			mQuit.Click(func() {
				quitFn()       // = app.requestQuit()：置退出标志 → 停代理 → wails Quit（+os.Exit 兜底）
				systray.Quit() // 托盘图标移除
			})
		}, func() {
			log.Printf("[tray] 托盘已退出")
		})
	}()
}

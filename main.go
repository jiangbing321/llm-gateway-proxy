package main

import (
	"context"
	"embed"
	"net/http"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"

	"llm-gateway-proxy/internal/singleinstance"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// 单实例锁：已有实例 → 唤醒其窗口后本进程退出
	inst, ok := singleinstance.Acquire()
	if !ok {
		return
	}

	app := NewApp()
	app.instance = inst

	err := wails.Run(&options.App{
		Title:            "LLM Gateway Proxy",
		Width:            1080,
		Height:           720,
		MinWidth:         860,
		MinHeight:        560,
		Frameless:        true,
		CSSDragProperty:  "--wails-draggable",
		CSSDragValue:     "drag",
		AssetServer: &assetserver.Options{
			Assets: assets,
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline' 'unsafe-eval'; style-src 'self' 'unsafe-inline'; img-src 'self' data: blob:; connect-src 'self'")
					next.ServeHTTP(w, r)
				})
			},
		},
		BackgroundColour: &options.RGBA{R: 13, G: 17, B: 23, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		// 关闭按钮 → 隐藏到托盘驻留（返回 true 阻止窗口销毁）；
		// 托盘"退出"置 app.quitting 标志后放行关闭（否则 Quit 也会被拦截）
		OnBeforeClose: func(ctx context.Context) bool {
			if app.IsQuitting() {
				return false // 真正退出：允许关闭
			}
			runtime.WindowHide(ctx)
			return true // 驻留托盘
		},
		Bind: []interface{}{
			app,
		},
		Windows: &windows.Options{
			WebviewIsTransparent: false,
			WindowIsTranslucent:  false,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}

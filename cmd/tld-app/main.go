package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"runtime"
	"strings"

	tld "github.com/mertcikla/tld/v2"
	cmdversion "github.com/mertcikla/tld/v2/cmd/version"
	"github.com/mertcikla/tld/v2/internal/localserver"
	"github.com/mertcikla/tld/v2/internal/workspace"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
)

func main() {
	if handled, err := runDesktopUpdateCommand(os.Args, os.Stdout); handled {
		if err != nil {
			log.Fatal(err)
		}
		return
	}

	if err := configureAppStoreRuntimePaths(); err != nil {
		log.Fatalf("failed to configure app store runtime paths: %v", err)
	}

	_ = workspace.EnsureGlobalConfig()

	cfg, err := workspace.LoadGlobalConfig()
	if err != nil {
		log.Fatalf("failed to load global config: %v", err)
	}

	dataDir, err := workspace.ResolveDataDir(cfg, "")
	if err != nil {
		log.Fatalf("failed to resolve data dir: %v", err)
	}

	app, err := localserver.Bootstrap(dataDir)
	if err != nil {
		log.Fatalf("failed to start local server: %v", err)
	}

	if err := localserver.RegisterProcess(localserver.ProcessRecord{
		Kind:    localserver.ProcessKindServer,
		PID:     os.Getpid(),
		DataDir: dataDir,
		Addr:    app.Addr,
	}); err != nil {
		log.Fatalf("failed to register server process: %v", err)
	}
	defer func() { _ = localserver.RemoveProcess(os.Getpid()) }()

	srv := &http.Server{Addr: app.Addr, Handler: app.Handler}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("backend server error: %v", err)
		}
	}()
	defer func() { _ = srv.Close() }()

	rootFS, err := tld.StaticFS()
	if err != nil {
		log.Fatalf("failed to get static fs: %v", err)
	}

	distFS, err := fs.Sub(rootFS, "frontend/dist")
	if err != nil {
		log.Fatalf("failed to sub frontend/dist: %v", err)
	}

	desktopBridge := NewDesktopBridge()
	err = wails.Run(&options.App{
		Title:     "tlDiagram",
		Width:     1200,
		Height:    800,
		MinWidth:  720,
		MinHeight: 720,
		// Keep Windows frameless builds resizable while the frontend draws caption buttons.
		DisableResize: false,
		Frameless:     runtime.GOOS == "windows",
		OnStartup: func(ctx context.Context) {
			desktopBridge.startup(ctx)
		},
		Bind: []any{
			desktopBridge,
		},
		AssetServer: &assetserver.Options{
			Assets:  distFS,
			Handler: app.Handler,
			Middleware: func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if r.URL.Path == "/" || r.URL.Path == "/index.html" {
						htmlBytes, err := fs.ReadFile(distFS, "index.html")
						if err == nil {
							injected := strings.Replace(
								string(htmlBytes),
								"<head>",
								fmt.Sprintf(`<head>
    <script>
        window.__TLD_SERVER_URL__ = 'http://%s';
        window.__TLD_APP__ = true;
        window.__TLD_APP_STORE__ = %t;
        window.__TLD_PLATFORM__ = %q;
        window.__TLD_VERSION__ = %q;

        // WKWebView does not fire wheel events for trackpad pinch like Chrome does.
        // It fires gesture events. ReactFlow (via d3-zoom) relies on wheel+ctrlKey for pinch.
        // We polyfill this by translating gesture events into wheel events.
        let pinchScale = 1;
        document.addEventListener('gesturestart', function(e) {
            e.preventDefault();
            pinchScale = 1;
        }, { passive: false });
        document.addEventListener('gesturechange', function(e) {
            e.preventDefault();
            let delta = pinchScale - e.scale;
            pinchScale = e.scale;
            e.target.dispatchEvent(new WheelEvent('wheel', {
                clientX: e.clientX,
                clientY: e.clientY,
                deltaY: delta * 300,
                ctrlKey: true,
                bubbles: true,
                cancelable: true
            }));
        }, { passive: false });
        document.addEventListener('gestureend', function(e) {
            e.preventDefault();
        }, { passive: false });
    </script>`, app.Addr, appStoreBuild, runtime.GOOS, cmdversion.Version),
								1,
							)
							w.Header().Set("Content-Type", "text/html; charset=utf-8")
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write([]byte(injected))
							return
						}
					}
					next.ServeHTTP(w, r)
				})
			},
		},
		BackgroundColour: &options.RGBA{R: 255, G: 255, B: 255, A: 255},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
		},
		Windows: &windows.Options{
			Theme:        windows.Dark,
			BackdropType: windows.Mica,
		},
	})

	if err != nil {
		log.Fatal("Error:", err.Error())
	}
}

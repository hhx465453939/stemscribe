// Package webui serves the exported static UI from an embedded filesystem.
//
// ADR-001 targets a Wails/WebView2 shell. That shell needs cgo + the WebView2
// loader in the build chain, which is the remaining CHG-0001 item. Until it
// lands, the app must still be openable by double-clicking the exe, so the
// no-argument path serves the same static export over loopback and hands it to
// the system browser. Same bundle, same UI, zero extra build dependencies.
package webui

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os/exec"
	"runtime"
	"time"
)

// assets is populated by scripts/build.ps1 (frontend/out -> assets/) before
// `go build`. The committed placeholder keeps the embed pattern valid on a fresh
// clone, so `go build ./...` works without a frontend build.
//
//go:embed all:assets
var assets embed.FS

// FS exposes the embedded UI rooted so that index.html sits at the root.
//
// The native shell (cmd/stemscribe-gui) mounts this straight into WebView2, which
// keeps a single copy of the UI for both the window and the loopback fallback.
func FS() (fs.FS, error) { return fs.Sub(assets, "assets") }

// Embedded reports whether real UI files were embedded (as opposed to only the
// placeholder). Used to tell the user "run scripts/build.ps1" instead of showing
// a blank page.
func Embedded() bool {
	sub, err := fs.Sub(assets, "assets")
	if err != nil {
		return false
	}
	if _, err = fs.Stat(sub, "index.html"); err != nil {
		return false
	}
	return true
}

// Handler returns the HTTP handler for the UI.
func Handler() http.Handler {
	sub, err := fs.Sub(assets, "assets")
	if err != nil || !Embedded() {
		return http.HandlerFunc(servePlaceholder)
	}
	// http.FileServer already resolves directory requests to index.html, which is
	// exactly the shape Next's static export produces (trailingSlash: true).
	return http.FileServer(http.FS(sub))
}

// Serve starts a loopback-only server and returns the URL to open.
//
// Binding to 127.0.0.1 (not 0.0.0.0) keeps the UI off the network: the app has no
// authentication, so exposing it to the LAN would be a vulnerability, not a
// feature.
func Serve(ctx context.Context) (string, error) {
	return ServeHandler(ctx, Handler())
}

// ServeHandler serves a caller-supplied handler, which is how the run API is
// mounted next to the static UI. The native window mounts the same value into
// Wails' AssetServer instead, so both shells exercise one implementation.
func ServeHandler(ctx context.Context, h http.Handler) (string, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("listen on loopback: %w", err)
	}

	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}()
	go func() { _ = srv.Serve(ln) }()

	return "http://" + ln.Addr().String() + "/", nil
}

// OpenBrowser hands the URL to the OS default browser.
func OpenBrowser(url string) error {
	switch runtime.GOOS {
	case "windows":
		// rundll32 avoids spawning a shell that would inherit and hold our console.
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	case "darwin":
		return exec.Command("open", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

// ErrNoUI is returned when the caller asks for a UI that was never embedded.
var ErrNoUI = errors.New("no UI embedded")

func servePlaceholder(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="zh-CN"><head><meta charset="utf-8"><title>Stemscribe</title>
<style>
  :root{--ink:#0a0b0d;--surface:#14171b;--bone:#e9e6e0;--dim:#9aa0a6;--line:#23272e;--lamp:#c9a227}
  body{margin:0;min-height:100vh;background:var(--ink);color:var(--bone);
       font:14px/1.55 Inter,"Noto Sans SC",system-ui,sans-serif;
       display:grid;place-items:center;padding:48px}
  .card{max-width:620px;border:1px solid var(--line);border-left:2px solid var(--lamp);
        border-radius:2px;background:var(--surface);padding:24px 28px;
        background-image:radial-gradient(ellipse at 8% 0%,rgba(201,162,39,.10),transparent 62%);
        background-repeat:no-repeat}
  h1{margin:0 0 8px;font-size:20px;letter-spacing:-.01em}
  p{margin:0 0 12px;color:var(--dim)}
  code{font-family:"JetBrains Mono",ui-monospace,monospace;font-size:12px;
       color:var(--bone);background:#1b1f25;border:1px solid var(--line);
       border-radius:2px;padding:2px 6px}
  .brand{display:flex;align-items:center;gap:8px;margin-bottom:16px;
         font-weight:700;letter-spacing:-.02em}
  .brand::before{content:"";width:2px;height:16px;background:var(--lamp)}
</style></head>
<body><main class="card">
  <div class="brand">STEMSCRIBE</div>
  <h1>界面还没有打进这个可执行文件</h1>
  <p>这个二进制是在没有前端产物的状态下编译的，所以只有内嵌占位页。</p>
  <p>生成带界面的版本：</p>
  <p><code>pwsh scripts/build.ps1</code></p>
  <p>它会先把 <code>frontend/out</code> 打进 <code>cmd/stemscribe/assets</code>，再编译。</p>
</main></body></html>`))
}

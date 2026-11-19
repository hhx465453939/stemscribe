package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"

	"github.com/hhx465453939/stemscribe/internal/config"
	"github.com/hhx465453939/stemscribe/internal/runctl"
	"github.com/hhx465453939/stemscribe/internal/webapi"
	"github.com/hhx465453939/stemscribe/internal/webui"
)

// cmdShell serves the embedded UI on loopback and opens it.
//
// Why a browser instead of a native window right now: the Wails/WebView2 shell
// planned in ADR-001 needs cgo plus the WebView2 loader in the build chain, and
// that is the remaining CHG-0001 item. A double-clickable binary that shows the
// real UI is more useful today than a window that cannot be built, and the
// served bundle is byte-identical to what the native shell will load.
func cmdShell(args []string) int {
	fs := flag.NewFlagSet("shell", flag.ContinueOnError)
	noOpen := fs.Bool("no-open", false, "serve without launching a browser (for screenshots/tests)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	if !webui.Embedded() {
		fmt.Fprintln(os.Stderr, "warning: this binary was built without UI assets.")
		fmt.Fprintln(os.Stderr, "         run: pwsh scripts/build.ps1")
		fmt.Fprintln(os.Stderr, "")
	}

	// Ctrl+C is the documented way to stop a console-hosted loopback server.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	// The run API is mounted next to the static UI here, and the native window mounts
	// the same handler -- one implementation, two shells (ADR-022).
	runctl.Version = version
	svc := runctl.NewService()
	wd, _ := config.DefaultWorkdir()
	handler := webapi.New(svc, webui.Handler(), wd)

	url, err := webui.ServeHandler(ctx, handler)
	if err != nil {
		fmt.Fprintf(os.Stderr, "shell: %v\n", err)
		return 1
	}

	fmt.Printf("Stemscribe %s\n", version)
	fmt.Printf("UI: %s\n", url)
	if !webui.Embedded() {
		fmt.Println("状态: 未内嵌界面（占位页）")
	}
	fmt.Println("关闭此窗口或按 Ctrl+C 即停止。")

	if !*noOpen {
		if err = webui.OpenBrowser(url); err != nil {
			fmt.Fprintf(os.Stderr, "shell: 无法自动打开浏览器（请手动访问上面的地址）: %v\n", err)
		}
	}

	<-ctx.Done()
	fmt.Println("\n已停止。")
	return 0
}

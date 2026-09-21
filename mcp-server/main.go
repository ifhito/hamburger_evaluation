// hamburger-mcp は、Hamburger Evaluation の HTTP API を MCP（stdio）の tool として見せる薄い窓である。
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	cfg, err := loadConfig(os.Getenv)
	if err != nil {
		fmt.Fprintln(os.Stderr, "hamburger-mcp:", err)
		os.Exit(1)
	}
	token := "unset"
	if cfg.token != "" {
		token = "set"
	}
	// stdout は MCP のプロトコル専用なので、ログは stderr に出す。トークンの値は出さない。
	fmt.Fprintf(os.Stderr, "hamburger-mcp %s: api=%s write=%t token=%s\n", version, cfg.apiURL, cfg.allowWrite, token)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := newServer(cfg).Run(ctx, &mcp.StdioTransport{}); err != nil && ctx.Err() == nil {
		fmt.Fprintln(os.Stderr, "hamburger-mcp:", err)
		os.Exit(1)
	}
}

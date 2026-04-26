package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	mcplog "protonmail-mcp/internal/log"
)

func main() {
	level := slog.LevelInfo
	if v := os.Getenv("PROTONMAIL_MCP_LOG_LEVEL"); v == "debug" {
		level = slog.LevelDebug
	}
	logger := mcplog.New(level, os.Stderr)
	slog.SetDefault(logger)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "login":
			if err := runLogin(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "login:", err)
				os.Exit(1)
			}
			return
		case "status":
			if err := runStatus(ctx); err != nil {
				fmt.Fprintln(os.Stderr, "status:", err)
				os.Exit(1)
			}
			return
		case "logout":
			fmt.Fprintln(os.Stderr, "logout: not yet implemented")
			os.Exit(2)
		default:
			fmt.Fprintf(os.Stderr, "unknown subcommand %q\n", os.Args[1])
			os.Exit(2)
		}
	}

	fmt.Fprintln(os.Stderr, "MCP server not yet implemented (auth-only build). Run `protonmail-mcp login` then `protonmail-mcp status`.")
	os.Exit(2)
}

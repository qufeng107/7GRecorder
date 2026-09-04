package main

import (
	"fmt"
	"os"

	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/db"
	"github.com/7grecorder/7grecorder/backend/internal/httpserver"
	"github.com/7grecorder/7grecorder/backend/internal/version"
	"github.com/gogf/gf/v2/os/gctx"
)

func main() {
	ctx := gctx.New()
	cfg := config.LoadFromEnv()
	command := "all"
	if len(os.Args) > 1 {
		command = os.Args[1]
	}

	switch command {
	case "all", "serve":
		httpserver.Run(ctx, cfg)
	case "migrate":
		if err := db.Migrate(ctx, cfg); err != nil {
			fmt.Fprintf(os.Stderr, "migration failed: %v\n", err)
			os.Exit(1)
		}
	case "worker":
		fmt.Fprintln(os.Stderr, "worker command is reserved for a future split runtime; production uses all")
		os.Exit(2)
	case "version":
		fmt.Println(version.Info())
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", command)
		os.Exit(2)
	}
}

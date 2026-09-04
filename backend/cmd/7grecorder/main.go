package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/7grecorder/7grecorder/backend/internal/account"
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
	case "admin":
		if err := runAdminCommand(ctx, cfg, os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "admin command failed: %v\n", err)
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

func runAdminCommand(ctx context.Context, cfg config.Config, args []string) error {
	if len(args) == 0 {
		return errors.New("missing admin subcommand")
	}
	switch args[0] {
	case "bootstrap":
		flags := flag.NewFlagSet("admin bootstrap", flag.ContinueOnError)
		flags.SetOutput(os.Stderr)
		username := flags.String("username", "", "admin username")
		password := flags.String("password", "", "admin password")
		if err := flags.Parse(args[1:]); err != nil {
			return err
		}
		if *password == "" {
			return errors.New("password is required")
		}
		if err := db.Migrate(ctx, cfg); err != nil {
			return err
		}
		database, err := db.Open(ctx, cfg)
		if err != nil {
			return err
		}
		defer database.Close()
		user, err := account.NewStore(database).BootstrapSuperAdmin(ctx, *username, *password)
		if err != nil {
			return err
		}
		fmt.Printf("created SUPER_ADMIN user id=%d username=%s\n", user.ID, user.Username)
		return nil
	default:
		return fmt.Errorf("unknown admin subcommand %q", args[0])
	}
}

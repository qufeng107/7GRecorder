package httpserver

import (
	"context"
	"database/sql"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/7grecorder/7grecorder/backend/internal/config"
	"github.com/7grecorder/7grecorder/backend/internal/version"
	"github.com/gogf/gf/v2/frame/g"
	"github.com/gogf/gf/v2/net/ghttp"
	_ "github.com/mattn/go-sqlite3"
)

type healthResponse struct {
	Status string `json:"status"`
	SHA    string `json:"sha"`
	Time   string `json:"time"`
}

func Run(ctx context.Context, cfg config.Config) {
	s := g.Server()
	s.SetAddr(cfg.ListenAddr)

	s.BindHandler("/health/live", func(r *ghttp.Request) {
		r.Response.WriteJson(healthResponse{
			Status: "live",
			SHA:    version.BuildSHA,
			Time:   time.Now().UTC().Format(time.RFC3339),
		})
	})

	s.BindHandler("/health/ready", func(r *ghttp.Request) {
		if err := readiness(ctx, cfg); err != nil {
			r.Response.Status = http.StatusServiceUnavailable
			r.Response.WriteJson(g.Map{
				"error": g.Map{
					"code":       "NOT_READY",
					"message":    "Backend is not ready.",
					"details":    err.Error(),
					"request_id": r.GetCtxVar("RequestId").String(),
				},
			})
			return
		}
		r.Response.WriteJson(healthResponse{
			Status: "ready",
			SHA:    version.BuildSHA,
			Time:   time.Now().UTC().Format(time.RFC3339),
		})
	})

	s.BindHandler("/api/v1/system/health", func(r *ghttp.Request) {
		r.Response.WriteJson(g.Map{
			"status":       "ok",
			"release_sha":  version.BuildSHA,
			"optional":     g.Map{},
			"generated_at": time.Now().UTC().Format(time.RFC3339),
		})
	})

	s.BindHandler("/api/v1/system/version", func(r *ghttp.Request) {
		r.Response.WriteJson(g.Map{
			"version": version.Info(),
			"sha":     version.BuildSHA,
		})
	})

	s.Run()
}

func readiness(ctx context.Context, cfg config.Config) error {
	for _, dir := range []string{
		cfg.DataRoot,
		filepath.Dir(cfg.SQLitePath),
		cfg.TempRoot,
		filepath.Join(cfg.DataRoot, "recordings"),
		filepath.Join(cfg.DataRoot, "songs"),
		filepath.Join(cfg.DataRoot, "backups", "db"),
	} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	db, err := sql.Open("sqlite3", cfg.SQLitePath+"?_foreign_keys=on&_busy_timeout=5000")
	if err != nil {
		return err
	}
	defer db.Close()

	if _, err := db.ExecContext(ctx, "PRAGMA journal_mode=WAL;"); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, "PRAGMA foreign_keys=ON;"); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "CREATE TABLE IF NOT EXISTS readiness_probe (id INTEGER PRIMARY KEY, checked_at DATETIME NOT NULL);")
	return err
}

package main

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/store"
	"github.com/example/azeroth-portal/internal/web"
)

//go:embed all:dist
var assets embed.FS

func main() {
	c, e := config.Load()
	if e != nil {
		slog.Error("configuration", "error", e)
		os.Exit(1)
	}
	var s *store.Store
	if !c.MockMode {
		s, e = store.Open(c)
		if e != nil {
			slog.Error("database", "error", e)
			os.Exit(1)
		}
		defer s.Close()
	}
	static, _ := fs.Sub(assets, "dist")
	srv := &http.Server{Addr: c.Addr, Handler: web.New(s, c, static).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	slog.Info("portal ready", "address", c.Addr, "realm", c.RealmName, "mock", c.MockMode)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		slog.Error("server", "error", e)
		os.Exit(1)
	}
}

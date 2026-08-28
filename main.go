package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"strings"
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
	if len(os.Args) > 1 {
		if os.Args[1] != "bootstrap-account" {
			slog.Error("command", "error", "unknown command")
			os.Exit(2)
		}
		gmLevel, realmID := 3, -1
		if _, e = fmt.Sscan(os.Getenv("BOOTSTRAP_GM_LEVEL"), &gmLevel); e != nil && os.Getenv("BOOTSTRAP_GM_LEVEL") != "" {
			slog.Error("bootstrap account", "error", "invalid BOOTSTRAP_GM_LEVEL")
			os.Exit(1)
		}
		if _, e = fmt.Sscan(os.Getenv("BOOTSTRAP_REALM_ID"), &realmID); e != nil && os.Getenv("BOOTSTRAP_REALM_ID") != "" {
			slog.Error("bootstrap account", "error", "invalid BOOTSTRAP_REALM_ID")
			os.Exit(1)
		}
		e = store.BootstrapAccount(c, os.Getenv("BOOTSTRAP_USERNAME"), os.Getenv("BOOTSTRAP_PASSWORD"), os.Getenv("BOOTSTRAP_EMAIL"), gmLevel, realmID)
		if e != nil {
			slog.Error("bootstrap account", "error", e)
			os.Exit(1)
		}
		slog.Info("bootstrap account ready", "username", strings.ToUpper(strings.TrimSpace(os.Getenv("BOOTSTRAP_USERNAME"))), "gmLevel", gmLevel, "realmID", realmID)
		return
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

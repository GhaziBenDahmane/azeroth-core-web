package main

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/realmagent"
	"github.com/example/azeroth-portal/internal/store"
	"github.com/example/azeroth-portal/internal/web"
)

//go:embed all:dist
var assets embed.FS

func main() {
	if len(os.Args) > 1 && os.Args[1] == "realm-agent" {
		agentConfig, err := realmagent.LoadConfig()
		if err != nil {
			slog.Error("realm agent configuration", "error", err)
			os.Exit(1)
		}
		if err = realmagent.Run(agentConfig); err != nil {
			slog.Error("realm agent", "error", err)
			os.Exit(1)
		}
		return
	}
	c, e := config.Load()
	if e != nil {
		slog.Error("configuration", "error", e)
		os.Exit(1)
	}
	if len(os.Args) > 1 {
		if os.Args[1] == "migrate" {
			for _, realm := range c.Realms {
				realmConfig := c.ForRealm(realm)
				realmStore, err := store.ConnectForMigration(realmConfig)
				if err != nil {
					slog.Error("migration connection", "realm", realm.Key, "error", err)
					os.Exit(1)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
				err = realmStore.Migrate(ctx)
				cancel()
				realmStore.Close()
				if err != nil {
					slog.Error("migration", "realm", realm.Key, "error", err)
					os.Exit(1)
				}
			}
			slog.Info("portal schema is current", "version", store.CurrentSchemaVersion)
			return
		}
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
	static, _ := fs.Sub(assets, "dist")
	realmHandlers := make(map[string]http.Handler, len(c.Realms))
	webServers := make([]*web.Server, 0, len(c.Realms))
	stores := make([]*store.Store, 0, len(c.Realms))
	for _, realm := range c.Realms {
		realmConfig := c.ForRealm(realm)
		var realmStore *store.Store
		if !c.MockMode {
			if c.AutoMigrate {
				realmStore, e = store.Open(realmConfig)
			} else {
				realmStore, e = store.Connect(realmConfig)
			}
			if e != nil {
				for _, opened := range stores {
					opened.Close()
				}
				slog.Error("realm database", "realm", realm.Key, "error", e)
				os.Exit(1)
			}
			stores = append(stores, realmStore)
		}
		portalServer := web.New(realmStore, realmConfig, static)
		webServers = append(webServers, portalServer)
		realmHandlers[realm.Key] = portalServer.Handler()
	}
	defer func() {
		for _, opened := range stores {
			opened.Close()
		}
	}()
	srv := &http.Server{Addr: c.Addr, Handler: web.MultiRealm(c.RealmKey, c.CookieSecure, realmHandlers), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 60 * time.Second}
	slog.Info("portal ready", "address", c.Addr, "realm", c.RealmName, "realms", len(c.Realms), "mock", c.MockMode)
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case sig := <-stop:
		slog.Info("shutdown requested", "signal", sig.String())
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("http shutdown", "error", err)
		}
		for _, portalServer := range webServers {
			portalServer.Close(ctx)
		}
	case e = <-errCh:
		if e != nil && e != http.ErrServerClosed {
			slog.Error("server", "error", e)
			os.Exit(1)
		}
	}
}

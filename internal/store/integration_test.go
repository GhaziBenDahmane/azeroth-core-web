package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/example/azeroth-portal/internal/config"
)

// TestMigrationMatrix is opt-in locally and runs against both MySQL and
// MariaDB in CI. It verifies a fresh schema, every numbered migration, the
// current-version guard, and migration idempotency using real servers.
func TestMigrationMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+dsnOptions(base))
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	names := []string{"portal_auth" + suffix, "portal_characters" + suffix, "portal_world" + suffix}
	for _, name := range names {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		name := name
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`") })
	}
	c := config.Config{
		AuthDSN: base + names[0], CharactersDSN: base + names[1], WorldDSN: base + names[2],
		AuthDB: names[0], CharactersDB: names[1], WorldDB: names[2], RealmKey: "integration", DefaultRealmKey: "integration",
	}
	s, err := ConnectForMigration(c)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer s.Close()
	if err = s.Migrate(ctx); err != nil {
		t.Fatalf("fresh migration: %v", err)
	}
	if err = s.RequireCurrentSchema(ctx); err != nil {
		t.Fatalf("version guard: %v", err)
	}
	if err = s.Migrate(ctx); err != nil {
		t.Fatalf("idempotent migration: %v", err)
	}
	var version uint32
	if err = s.Auth.QueryRowContext(ctx, "SELECT MAX(version) FROM portal_schema_migrations").Scan(&version); err != nil || version != CurrentSchemaVersion {
		t.Fatalf("schema version = %d, err = %v; want %d", version, err, CurrentSchemaVersion)
	}
}

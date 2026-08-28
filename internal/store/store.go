package store

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	_ "github.com/go-sql-driver/mysql"
)

type Store struct {
	Auth, Characters, World *sql.DB
	C                       config.Config
}

func Open(c config.Config) (*Store, error) {
	a, err := sql.Open("mysql", c.AuthDSN+dsnOptions(c.AuthDSN))
	if err != nil {
		return nil, err
	}
	ch, err := sql.Open("mysql", c.CharactersDSN+dsnOptions(c.CharactersDSN))
	if err != nil {
		a.Close()
		return nil, err
	}
	w, err := sql.Open("mysql", c.WorldDSN+dsnOptions(c.WorldDSN))
	if err != nil {
		a.Close()
		ch.Close()
		return nil, err
	}
	s := &Store{a, ch, w, c}
	for _, db := range []*sql.DB{a, ch, w} {
		db.SetConnMaxLifetime(3 * time.Minute)
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(3)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if err := a.PingContext(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("auth database: %w", err)
	}
	if err := ch.PingContext(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("characters database: %w", err)
	}
	if err := w.PingContext(ctx); err != nil {
		s.Close()
		return nil, fmt.Errorf("world database: %w", err)
	}
	if err := s.Migrate(ctx); err != nil {
		s.Close()
		return nil, err
	}
	seedCtx, seedCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer seedCancel()
	if seedErr := s.SeedDefaultServices(seedCtx); seedErr != nil {
		slog.Warn("default services could not be seeded", "error", seedErr)
	}
	if seeded, seedErr := s.SeedDefaultCatalog(seedCtx); seedErr != nil {
		slog.Warn("default shop catalog could not be fully seeded", "error", seedErr)
	} else {
		slog.Info("default WotLK shop catalog ready", "packages", seeded)
	}
	return s, nil
}
func dsnOptions(d string) string {
	if len(d) > 0 && containsQuestion(d) {
		return "&parseTime=true&charset=utf8mb4"
	}
	return "?parseTime=true&charset=utf8mb4"
}
func containsQuestion(s string) bool {
	for _, r := range s {
		if r == '?' {
			return true
		}
	}
	return false
}
func (s *Store) Close() { s.Auth.Close(); s.Characters.Close(); s.World.Close() }

func (s *Store) Migrate(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS portal_sessions (
		 token_hash BINARY(32) PRIMARY KEY, account_id INT UNSIGNED NOT NULL, expires_at TIMESTAMP NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 ip_address VARCHAR(45) NOT NULL DEFAULT '', user_agent VARCHAR(255) NOT NULL DEFAULT '',
		 INDEX idx_portal_session_account (account_id), INDEX idx_portal_session_expiry (expires_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_account_security (
		 account_id INT UNSIGNED PRIMARY KEY, totp_secret VARBINARY(64) NULL, totp_enabled TINYINT(1) NOT NULL DEFAULT 0,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS portal_wallets (
		 account_id INT UNSIGNED PRIMARY KEY, balance INT UNSIGNED NOT NULL DEFAULT 0,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS portal_products (
		 id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, seed_key VARCHAR(80) NULL, name VARCHAR(100) NOT NULL, description VARCHAR(500) NOT NULL DEFAULT '',
		 item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL DEFAULT 1, price INT UNSIGNED NOT NULL,
		 category VARCHAR(40) NOT NULL DEFAULT 'Items', image_url VARCHAR(500) NOT NULL DEFAULT '', active TINYINT(1) NOT NULL DEFAULT 1,
		 class_id TINYINT UNSIGNED NOT NULL DEFAULT 0, tier_label VARCHAR(30) NOT NULL DEFAULT '', service_level TINYINT UNSIGNED NOT NULL DEFAULT 0,
		 gold_amount INT UNSIGNED NOT NULL DEFAULT 0, service_action VARCHAR(30) NOT NULL DEFAULT '',
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, INDEX idx_portal_products_active (active), UNIQUE KEY idx_portal_products_seed_key (seed_key)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_product_items (
		 product_id INT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL DEFAULT 1,
		 PRIMARY KEY (product_id,item_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_orders (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL,
		 product_id INT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL, total INT UNSIGNED NOT NULL,
		 status ENUM('pending','delivering','delivered','review','failed','refunded') NOT NULL DEFAULT 'pending', error_message VARCHAR(500) NOT NULL DEFAULT '',
		 attempts INT UNSIGNED NOT NULL DEFAULT 0, service_level TINYINT UNSIGNED NOT NULL DEFAULT 0, gold_amount INT UNSIGNED NOT NULL DEFAULT 0,
		 service_action VARCHAR(30) NOT NULL DEFAULT '',
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, delivery_started_at TIMESTAMP NULL, delivered_at TIMESTAMP NULL,
		 INDEX idx_portal_orders_account (account_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_order_items (
		 order_id BIGINT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL DEFAULT 1,
		 PRIMARY KEY (order_id,item_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_credit_ledger (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, actor_account_id INT UNSIGNED NOT NULL, target_account_id INT UNSIGNED NOT NULL,
		 amount INT NOT NULL, reason VARCHAR(255) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_credit_target (target_account_id,created_at), INDEX idx_portal_credit_actor (actor_account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_payment_events (
		 event_id VARCHAR(255) PRIMARY KEY, checkout_id VARCHAR(255) NOT NULL, account_id INT UNSIGNED NOT NULL,
		 credits INT UNSIGNED NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 UNIQUE KEY idx_portal_checkout (checkout_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_moderation_log (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, actor_account_id INT UNSIGNED NOT NULL, target_account_id INT UNSIGNED NOT NULL DEFAULT 0,
		 target VARCHAR(64) NOT NULL, action VARCHAR(30) NOT NULL, duration VARCHAR(30) NOT NULL DEFAULT '', reason VARCHAR(255) NOT NULL DEFAULT '',
		 status ENUM('executed','review') NOT NULL, error_message VARCHAR(500) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_moderation_target (target_account_id,created_at), INDEX idx_portal_moderation_actor (actor_account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_support_tickets (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL DEFAULT 0,
		 subject VARCHAR(100) NOT NULL, message TEXT NOT NULL, status ENUM('open','answered','closed') NOT NULL DEFAULT 'open',
		 gm_account_id INT UNSIGNED NOT NULL DEFAULT 0, response TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 INDEX idx_portal_ticket_account (account_id,created_at), INDEX idx_portal_ticket_status (status,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_password_resets (
		 token_hash BINARY(32) PRIMARY KEY, account_id INT UNSIGNED NOT NULL, expires_at TIMESTAMP NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, INDEX idx_portal_reset_account (account_id)) ENGINE=InnoDB`,
	}
	for _, q := range statements {
		if _, err := s.Auth.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("portal migration: %w", err)
		}
	}
	for _, q := range []string{
		`ALTER TABLE portal_products ADD COLUMN IF NOT EXISTS class_id TINYINT UNSIGNED NOT NULL DEFAULT 0`,
		`ALTER TABLE portal_products ADD COLUMN IF NOT EXISTS tier_label VARCHAR(30) NOT NULL DEFAULT ''`,
		`ALTER TABLE portal_products ADD COLUMN IF NOT EXISTS service_level TINYINT UNSIGNED NOT NULL DEFAULT 0`,
		`ALTER TABLE portal_products ADD COLUMN IF NOT EXISTS gold_amount INT UNSIGNED NOT NULL DEFAULT 0`,
		`ALTER TABLE portal_products ADD COLUMN IF NOT EXISTS seed_key VARCHAR(80) NULL`,
		`ALTER TABLE portal_products ADD COLUMN IF NOT EXISTS service_action VARCHAR(30) NOT NULL DEFAULT ''`,
		`ALTER TABLE portal_orders MODIFY COLUMN status ENUM('pending','delivering','delivered','review','failed','refunded') NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE portal_orders ADD COLUMN IF NOT EXISTS attempts INT UNSIGNED NOT NULL DEFAULT 0`,
		`ALTER TABLE portal_orders ADD COLUMN IF NOT EXISTS service_level TINYINT UNSIGNED NOT NULL DEFAULT 0`,
		`ALTER TABLE portal_orders ADD COLUMN IF NOT EXISTS gold_amount INT UNSIGNED NOT NULL DEFAULT 0`,
		`ALTER TABLE portal_orders ADD COLUMN IF NOT EXISTS service_action VARCHAR(30) NOT NULL DEFAULT ''`,
		`ALTER TABLE portal_orders ADD COLUMN IF NOT EXISTS delivery_started_at TIMESTAMP NULL`,
		`ALTER TABLE portal_sessions ADD COLUMN IF NOT EXISTS last_seen_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP`,
		`ALTER TABLE portal_sessions ADD COLUMN IF NOT EXISTS ip_address VARCHAR(45) NOT NULL DEFAULT ''`,
		`ALTER TABLE portal_sessions ADD COLUMN IF NOT EXISTS user_agent VARCHAR(255) NOT NULL DEFAULT ''`,
	} {
		if _, err := s.Auth.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("portal product migration: %w", err)
		}
	}
	var seedIndex int
	if err := s.Auth.QueryRowContext(ctx, "SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=? AND table_name='portal_products' AND index_name='idx_portal_products_seed_key'", s.C.AuthDB).Scan(&seedIndex); err != nil {
		return fmt.Errorf("portal catalog index check: %w", err)
	}
	if seedIndex == 0 {
		if _, err := s.Auth.ExecContext(ctx, "CREATE UNIQUE INDEX idx_portal_products_seed_key ON portal_products(seed_key)"); err != nil {
			return fmt.Errorf("portal catalog index: %w", err)
		}
	}
	_, _ = s.Auth.ExecContext(ctx, `DELETE FROM portal_sessions WHERE expires_at < NOW()`)
	return nil
}

func (s *Store) SeedDefaultServices(ctx context.Context) error {
	services := []struct {
		key, name, description, action string
		price                          uint32
	}{
		{"service-race-change", "Race Change", "Choose a new race from your current faction on your next login.", "race_change", 35},
		{"service-faction-change", "Faction Change", "Choose a compatible race from the opposite faction on your next login.", "faction_change", 50},
	}
	for _, service := range services {
		_, err := s.Auth.ExecContext(ctx, `INSERT INTO portal_products(seed_key,name,description,item_id,quantity,price,category,tier_label,service_action,active)
			VALUES(?,?,?,0,0,?,'Services','Character',?,1)
			ON DUPLICATE KEY UPDATE name=VALUES(name),description=VALUES(description),price=VALUES(price),category=VALUES(category),tier_label=VALUES(tier_label),service_action=VALUES(service_action),active=1`,
			service.key, service.name, service.description, service.price, service.action)
		if err != nil {
			return fmt.Errorf("seed %s: %w", service.key, err)
		}
	}
	return nil
}

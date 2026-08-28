package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/go-sql-driver/mysql"
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
		 realm_key VARCHAR(64) NOT NULL DEFAULT 'default',
		 product_id INT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL, total INT UNSIGNED NOT NULL,
		 status ENUM('pending','delivering','delivered','review','failed','refunded') NOT NULL DEFAULT 'pending', error_message VARCHAR(500) NOT NULL DEFAULT '',
		 attempts INT UNSIGNED NOT NULL DEFAULT 0, service_level TINYINT UNSIGNED NOT NULL DEFAULT 0, gold_amount INT UNSIGNED NOT NULL DEFAULT 0,
		 service_action VARCHAR(30) NOT NULL DEFAULT '',
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, delivery_started_at TIMESTAMP NULL, delivered_at TIMESTAMP NULL,
		 INDEX idx_portal_orders_account (account_id, created_at), INDEX idx_portal_orders_realm_status (realm_key,status,id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
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
		 realm_key VARCHAR(64) NOT NULL DEFAULT 'default',
		 target VARCHAR(64) NOT NULL, action VARCHAR(30) NOT NULL, duration VARCHAR(30) NOT NULL DEFAULT '', reason VARCHAR(255) NOT NULL DEFAULT '',
		 status ENUM('executed','review') NOT NULL, error_message VARCHAR(500) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_moderation_target (target_account_id,created_at), INDEX idx_portal_moderation_actor (actor_account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_command_log (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, actor_account_id INT UNSIGNED NOT NULL,
		 realm_key VARCHAR(64) NOT NULL DEFAULT 'default',
		 command VARCHAR(255) NOT NULL, response TEXT NOT NULL, success TINYINT(1) NOT NULL DEFAULT 0,
		 ip_address VARCHAR(45) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_command_actor (actor_account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_support_tickets (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL DEFAULT 0,
		 realm_key VARCHAR(64) NOT NULL DEFAULT 'default',
		 subject VARCHAR(100) NOT NULL, message TEXT NOT NULL, status ENUM('open','answered','closed') NOT NULL DEFAULT 'open',
		 gm_account_id INT UNSIGNED NOT NULL DEFAULT 0, response TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 INDEX idx_portal_ticket_account (account_id,created_at), INDEX idx_portal_ticket_status (status,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_password_resets (
		 token_hash BINARY(32) PRIMARY KEY, account_id INT UNSIGNED NOT NULL, expires_at TIMESTAMP NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, INDEX idx_portal_reset_account (account_id)) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS portal_email_verifications (
		 token_hash BINARY(32) PRIMARY KEY, account_id INT UNSIGNED NOT NULL, expires_at TIMESTAMP NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 UNIQUE KEY idx_portal_verification_account (account_id), INDEX idx_portal_verification_expiry (expires_at)) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS portal_settings (
		 setting_key VARCHAR(80) PRIMARY KEY, setting_value TEXT NOT NULL,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_news (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, title VARCHAR(120) NOT NULL, summary VARCHAR(1000) NOT NULL DEFAULT '', url VARCHAR(500) NOT NULL DEFAULT '',
		 kind ENUM('news','announcement','maintenance') NOT NULL DEFAULT 'news', publish_at DATETIME NULL, expires_at DATETIME NULL,
		 active TINYINT(1) NOT NULL DEFAULT 1, created_by INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 INDEX idx_portal_news_publish (active,publish_at,expires_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_coupons (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, code VARCHAR(40) NOT NULL, discount_percent TINYINT UNSIGNED NOT NULL DEFAULT 0,
		 discount_credits INT UNSIGNED NOT NULL DEFAULT 0, starts_at DATETIME NULL, ends_at DATETIME NULL, max_uses INT UNSIGNED NOT NULL DEFAULT 0,
		 per_account_limit INT UNSIGNED NOT NULL DEFAULT 1, active TINYINT(1) NOT NULL DEFAULT 1, created_by INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE KEY idx_portal_coupon_code (code)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_coupon_uses (
		 coupon_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, order_id BIGINT UNSIGNED NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(coupon_id,order_id),
		 INDEX idx_portal_coupon_account(coupon_id,account_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_character_services (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL,
		 realm_key VARCHAR(64) NOT NULL DEFAULT 'default',
		 action VARCHAR(30) NOT NULL, character_name VARCHAR(32) NOT NULL DEFAULT '', success TINYINT(1) NOT NULL DEFAULT 0,
		 response VARCHAR(500) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_character_service(account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_admin_audit (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, actor_account_id INT UNSIGNED NOT NULL, action VARCHAR(50) NOT NULL,
		 target VARCHAR(120) NOT NULL DEFAULT '', details VARCHAR(500) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_admin_audit(actor_account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range statements {
		if _, err := s.Auth.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("portal migration: %w", err)
		}
	}
	if s.C.EnableSetup {
		if _, err := s.Auth.ExecContext(ctx, "INSERT IGNORE INTO portal_settings(setting_key,setting_value) VALUES('setup_complete','0')"); err != nil {
			return fmt.Errorf("portal setup migration: %w", err)
		}
		markExistingGM := fmt.Sprintf("UPDATE portal_settings SET setting_value='1' WHERE setting_key='setup_complete' AND setting_value='0' AND EXISTS(SELECT 1 FROM `%s`.account_access WHERE gmlevel>0)", s.C.AuthDB)
		if _, err := s.Auth.ExecContext(ctx, markExistingGM); err != nil {
			return fmt.Errorf("portal setup state: %w", err)
		}
	}
	columns := []struct{ table, column, definition string }{
		{"portal_products", "class_id", "TINYINT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_products", "tier_label", "VARCHAR(30) NOT NULL DEFAULT ''"},
		{"portal_products", "service_level", "TINYINT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_products", "gold_amount", "INT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_products", "seed_key", "VARCHAR(80) NULL"},
		{"portal_products", "service_action", "VARCHAR(30) NOT NULL DEFAULT ''"},
		{"portal_products", "starts_at", "DATETIME NULL"},
		{"portal_products", "ends_at", "DATETIME NULL"},
		{"portal_products", "per_account_limit", "INT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_orders", "attempts", "INT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_orders", "service_level", "TINYINT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_orders", "gold_amount", "INT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_orders", "service_action", "VARCHAR(30) NOT NULL DEFAULT ''"},
		{"portal_orders", "delivery_started_at", "TIMESTAMP NULL"},
		{"portal_orders", "subtotal", "INT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_orders", "discount", "INT UNSIGNED NOT NULL DEFAULT 0"},
		{"portal_orders", "coupon_code", "VARCHAR(40) NOT NULL DEFAULT ''"},
		{"portal_orders", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
		{"portal_moderation_log", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
		{"portal_command_log", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
		{"portal_support_tickets", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
		{"portal_character_services", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
		{"portal_sessions", "last_seen_at", "TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP"},
		{"portal_sessions", "ip_address", "VARCHAR(45) NOT NULL DEFAULT ''"},
		{"portal_sessions", "user_agent", "VARCHAR(255) NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := s.ensureColumn(ctx, column.table, column.column, column.definition); err != nil {
			return fmt.Errorf("portal column migration %s.%s: %w", column.table, column.column, err)
		}
	}
	if s.C.RealmKey == s.C.DefaultRealmKey && s.C.RealmKey != "default" {
		for _, table := range []string{"portal_orders", "portal_moderation_log", "portal_command_log", "portal_support_tickets", "portal_character_services"} {
			if _, err := s.Auth.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET realm_key=? WHERE realm_key='default'", table), s.C.RealmKey); err != nil {
				return fmt.Errorf("portal legacy realm migration %s: %w", table, err)
			}
		}
	}
	if err := s.ensureIndex(ctx, "portal_orders", "idx_portal_orders_realm_status", "realm_key,status,id"); err != nil {
		return fmt.Errorf("portal order realm index migration: %w", err)
	}
	if _, err := s.Auth.ExecContext(ctx, `ALTER TABLE portal_orders MODIFY COLUMN status ENUM('pending','delivering','delivered','review','failed','refunded') NOT NULL DEFAULT 'pending'`); err != nil {
		return fmt.Errorf("portal order status migration: %w", err)
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

func (s *Store) ensureIndex(ctx context.Context, table, index, columns string) error {
	var exists int
	if err := s.Auth.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name=?`, s.C.AuthDB, table, index).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	_, err := s.Auth.ExecContext(ctx, fmt.Sprintf("CREATE INDEX `%s` ON `%s` (%s)", index, table, columns))
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1061 {
		return nil
	}
	return err
}

// ensureColumn avoids ADD COLUMN IF NOT EXISTS, which is unavailable on older
// MySQL releases still commonly used by AzerothCore installations.
func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	var exists int
	if err := s.Auth.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.columns WHERE table_schema=? AND table_name=? AND column_name=?`, s.C.AuthDB, table, column).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	_, err := s.Auth.ExecContext(ctx, fmt.Sprintf("ALTER TABLE `%s` ADD COLUMN `%s` %s", table, column, definition))
	var mysqlErr *mysql.MySQLError
	if errors.As(err, &mysqlErr) && mysqlErr.Number == 1060 { // Another instance migrated first.
		return nil
	}
	return err
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

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

const CurrentSchemaVersion uint32 = 42

func Open(c config.Config) (*Store, error) {
	return open(c, true, false)
}

// Connect opens and validates AzerothCore database connections without
// changing schema or catalog data. It is used when migrations are managed as
// an explicit deployment step.
func Connect(c config.Config) (*Store, error) {
	return open(c, false, true)
}

// ConnectForMigration opens the databases without requiring an existing portal
// schema so the explicit migrate command can initialize a fresh deployment.
func ConnectForMigration(c config.Config) (*Store, error) {
	return open(c, false, false)
}

func open(c config.Config, migrate, requireCurrent bool) (*Store, error) {
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
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer pingCancel()
	if err := a.PingContext(pingCtx); err != nil {
		s.Close()
		return nil, fmt.Errorf("auth database: %w", err)
	}
	if err := ch.PingContext(pingCtx); err != nil {
		s.Close()
		return nil, fmt.Errorf("characters database: %w", err)
	}
	if err := w.PingContext(pingCtx); err != nil {
		s.Close()
		return nil, fmt.Errorf("world database: %w", err)
	}
	if migrate {
		migrationCtx, migrationCancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer migrationCancel()
		if err := s.Migrate(migrationCtx); err != nil {
			s.Close()
			return nil, err
		}
	} else if requireCurrent {
		if err := s.RequireCurrentSchema(pingCtx); err != nil {
			s.Close()
			return nil, err
		}
	}
	if !migrate {
		return s, nil
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

func (s *Store) RequireCurrentSchema(ctx context.Context) error {
	var exists int
	if err := s.Auth.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema=? AND table_name='portal_schema_migrations'`, s.C.AuthDB).Scan(&exists); err != nil {
		return fmt.Errorf("inspect portal schema: %w", err)
	}
	if exists == 0 {
		return fmt.Errorf("portal schema is not initialized; run `azeroth-portal migrate`")
	}
	var version uint32
	if err := s.Auth.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM portal_schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read portal schema version: %w", err)
	}
	if version != CurrentSchemaVersion {
		return fmt.Errorf("portal schema version %d is not current (expected %d); run `azeroth-portal migrate`", version, CurrentSchemaVersion)
	}
	return nil
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
	if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_schema_migrations (
		version INT UNSIGNED PRIMARY KEY, name VARCHAR(120) NOT NULL,
		applied_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
		return fmt.Errorf("create migration ledger: %w", err)
	}
	lockName := "azeroth_portal_schema:" + s.C.AuthDB
	var locked int
	if err := s.Auth.QueryRowContext(ctx, "SELECT GET_LOCK(?,10)", lockName).Scan(&locked); err != nil {
		return fmt.Errorf("acquire migration lock: %w", err)
	}
	if locked != 1 {
		return fmt.Errorf("acquire migration lock: timed out")
	}
	defer func() { _, _ = s.Auth.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", lockName) }()
	var version uint32
	if err := s.Auth.QueryRowContext(ctx, "SELECT COALESCE(MAX(version),0) FROM portal_schema_migrations").Scan(&version); err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if version < 1 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_competitive_events (realm_key VARCHAR(64) NOT NULL, source_event_id VARCHAR(128) NOT NULL, event_type VARCHAR(20) NOT NULL, received_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(realm_key,source_event_id,event_type)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
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
		 realm_key VARCHAR(64) NOT NULL DEFAULT 'default', featured TINYINT(1) NOT NULL DEFAULT 0,
		 sale_price INT UNSIGNED NOT NULL DEFAULT 0, stock_limit INT UNSIGNED NOT NULL DEFAULT 0, sold_count INT UNSIGNED NOT NULL DEFAULT 0, category_order INT NOT NULL DEFAULT 0,
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
			`CREATE TABLE IF NOT EXISTS portal_order_steps (
		 order_id BIGINT UNSIGNED NOT NULL, step_key VARCHAR(64) NOT NULL, kind VARCHAR(30) NOT NULL,
		 status ENUM('pending','executing','completed','failed') NOT NULL DEFAULT 'pending', attempts INT UNSIGNED NOT NULL DEFAULT 0,
		 response VARCHAR(500) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP, completed_at TIMESTAMP NULL,
		 PRIMARY KEY (order_id,step_key), INDEX idx_portal_order_steps_status (status,updated_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
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
		 active TINYINT(1) NOT NULL DEFAULT 1, created_by INT UNSIGNED NOT NULL DEFAULT 0, realm_key VARCHAR(64) NOT NULL DEFAULT 'default',
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
			`CREATE TABLE IF NOT EXISTS portal_daily_rewards (
		 account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL, claim_date DATE NOT NULL, credits INT UNSIGNED NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(account_id,realm_key,claim_date)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_vote_rewards (
		 event_id VARCHAR(128) PRIMARY KEY, account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL,
		 credits INT UNSIGNED NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_vote_account (account_id,realm_key,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_referrals (
		 account_id INT UNSIGNED PRIMARY KEY, code VARCHAR(40) NOT NULL, referred_by INT UNSIGNED NOT NULL DEFAULT 0,
		 uses INT UNSIGNED NOT NULL DEFAULT 0, credits_earned INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE KEY idx_portal_referral_code(code)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_raid_kills (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, guild_id INT UNSIGNED NOT NULL DEFAULT 0,
		 guild_name VARCHAR(80) NOT NULL, raid VARCHAR(80) NOT NULL, boss VARCHAR(80) NOT NULL, difficulty VARCHAR(30) NOT NULL,
		 duration_seconds INT UNSIGNED NOT NULL DEFAULT 0, killed_at DATETIME NOT NULL,
		 INDEX idx_portal_raid_rank (realm_key,raid,difficulty,duration_seconds), INDEX idx_portal_raid_recent (realm_key,killed_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
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
			{"portal_products", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
			{"portal_products", "featured", "TINYINT(1) NOT NULL DEFAULT 0"},
			{"portal_products", "sale_price", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_products", "stock_limit", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_products", "sold_count", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_products", "category_order", "INT NOT NULL DEFAULT 0"},
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
			{"portal_news", "realm_key", "VARCHAR(64) NOT NULL DEFAULT 'default'"},
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
			for _, table := range []string{"portal_products", "portal_news", "portal_orders", "portal_moderation_log", "portal_command_log", "portal_support_tickets", "portal_character_services"} {
				if _, err := s.Auth.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET realm_key=? WHERE realm_key='default'", table), s.C.RealmKey); err != nil {
					return fmt.Errorf("portal legacy realm migration %s: %w", table, err)
				}
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "UPDATE portal_products SET seed_key=CONCAT(?,':',seed_key) WHERE realm_key=? AND seed_key IS NOT NULL AND seed_key NOT LIKE '%:%'", s.C.RealmKey, s.C.RealmKey); err != nil {
			return fmt.Errorf("portal product seed migration: %w", err)
		}
		if err := s.ensureIndex(ctx, "portal_orders", "idx_portal_orders_realm_status", "realm_key,status,id"); err != nil {
			return fmt.Errorf("portal order realm index migration: %w", err)
		}
		if err := s.ensureIndex(ctx, "portal_products", "idx_portal_products_realm", "realm_key,active,category_order"); err != nil {
			return fmt.Errorf("portal product realm index migration: %w", err)
		}
		if err := s.ensureIndex(ctx, "portal_news", "idx_portal_news_realm", "realm_key,active,publish_at"); err != nil {
			return fmt.Errorf("portal news realm index migration: %w", err)
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
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(1,'baseline and resumable fulfillment')"); err != nil {
			return fmt.Errorf("record migration version 1: %w", err)
		}
		version = 1
	}
	if version < 2 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_vote_sites (
			 id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, slug VARCHAR(50) NOT NULL,
			 name VARCHAR(100) NOT NULL, url VARCHAR(500) NOT NULL, description VARCHAR(500) NOT NULL DEFAULT '',
			 reward_credits INT UNSIGNED NOT NULL DEFAULT 0, cooldown_minutes INT UNSIGNED NOT NULL DEFAULT 720,
			 active TINYINT(1) NOT NULL DEFAULT 1, sort_order INT NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY idx_portal_vote_site_slug(realm_key,slug), INDEX idx_portal_vote_sites_active(realm_key,active,sort_order)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_vote_clicks (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, site_id INT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL,
			 realm_key VARCHAR(64) NOT NULL, clicked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_vote_click_account(account_id,realm_key,clicked_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_vote_events (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, site_id INT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL,
			 realm_key VARCHAR(64) NOT NULL, provider_event_id VARCHAR(128) NOT NULL, credits INT UNSIGNED NOT NULL,
			 ip_hash BINARY(32) NULL, voted_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY idx_portal_vote_provider_event(site_id,provider_event_id),
			 INDEX idx_portal_vote_account(account_id,realm_key,voted_at), INDEX idx_portal_vote_leaderboard(realm_key,voted_at,account_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 2: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(2,'multi-site voting')"); err != nil {
			return fmt.Errorf("record migration version 2: %w", err)
		}
		version = 2
	}
	if version < 3 {
		if err := s.ensureColumn(ctx, "portal_email_verifications", "pending_email", "VARCHAR(255) NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("portal migration 3: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(3,'verified email changes')"); err != nil {
			return fmt.Errorf("record migration version 3: %w", err)
		}
		version = 3
	}
	if version < 4 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_notifications (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL,
		 kind VARCHAR(30) NOT NULL, title VARCHAR(120) NOT NULL, message VARCHAR(500) NOT NULL DEFAULT '', action_url VARCHAR(500) NOT NULL DEFAULT '',
		 read_at DATETIME NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_notifications_account(account_id,realm_key,read_at,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 4: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(4,'player notifications')"); err != nil {
			return fmt.Errorf("record migration version 4: %w", err)
		}
		version = 4
	}
	if version < 5 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_staff_roles (
		 account_id INT UNSIGNED PRIMARY KEY, role VARCHAR(30) NOT NULL, granted_by INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 INDEX idx_portal_staff_role(role)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 5: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(5,'database staff roles')"); err != nil {
			return fmt.Errorf("record migration version 5: %w", err)
		}
		version = 5
	}
	if version < 6 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_ticket_messages (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, ticket_id BIGINT UNSIGNED NOT NULL, author_account_id INT UNSIGNED NOT NULL,
			 author_role VARCHAR(20) NOT NULL, message TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_ticket_messages(ticket_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`INSERT INTO portal_ticket_messages(ticket_id,author_account_id,author_role,message,created_at)
			 SELECT t.id,t.account_id,'player',t.message,t.created_at FROM portal_support_tickets t
			 WHERE NOT EXISTS(SELECT 1 FROM portal_ticket_messages m WHERE m.ticket_id=t.id AND m.author_role='player')`,
			`INSERT INTO portal_ticket_messages(ticket_id,author_account_id,author_role,message,created_at)
			 SELECT t.id,t.gm_account_id,'staff',t.response,t.updated_at FROM portal_support_tickets t WHERE t.response<>'' AND t.gm_account_id>0
			 AND NOT EXISTS(SELECT 1 FROM portal_ticket_messages m WHERE m.ticket_id=t.id AND m.author_role='staff')`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 6: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(6,'support conversations')"); err != nil {
			return fmt.Errorf("record migration version 6: %w", err)
		}
		version = 6
	}
	if version < 7 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_downloads (
		 id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, name VARCHAR(100) NOT NULL,
		 platform VARCHAR(30) NOT NULL, url VARCHAR(500) NOT NULL, version VARCHAR(40) NOT NULL DEFAULT '', file_size VARCHAR(40) NOT NULL DEFAULT '',
		 sha256 CHAR(64) NOT NULL DEFAULT '', signature_url VARCHAR(500) NOT NULL DEFAULT '', notes VARCHAR(500) NOT NULL DEFAULT '',
		 active TINYINT(1) NOT NULL DEFAULT 1, sort_order INT NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_downloads_realm(realm_key,active,sort_order)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 7: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(7,'managed client downloads')"); err != nil {
			return fmt.Errorf("record migration version 7: %w", err)
		}
		version = 7
	}
	if version < 8 {
		if err := s.ensureColumn(ctx, "portal_raid_kills", "source_event_id", "VARCHAR(128) NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("portal migration 8 raid event: %w", err)
		}
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_raid_members (kill_id BIGINT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL DEFAULT 0, character_name VARCHAR(32) NOT NULL, class_id TINYINT UNSIGNED NOT NULL DEFAULT 0, role_name VARCHAR(20) NOT NULL DEFAULT '', PRIMARY KEY(kill_id,character_name)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_pvp_matches (id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, source_event_id VARCHAR(128) NOT NULL, bracket TINYINT UNSIGNED NOT NULL, team_name VARCHAR(100) NOT NULL, opponent_name VARCHAR(100) NOT NULL, result VARCHAR(10) NOT NULL, rating_change SMALLINT NOT NULL DEFAULT 0, played_at DATETIME NOT NULL, UNIQUE KEY idx_portal_pvp_event(realm_key,source_event_id), INDEX idx_portal_pvp_recent(realm_key,played_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_pvp_match_members (match_id BIGINT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL DEFAULT 0, character_name VARCHAR(32) NOT NULL, PRIMARY KEY(match_id,character_name), INDEX idx_portal_pvp_character(character_name,match_id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 8: %w", err)
			}
		}
		if err := s.ensureIndex(ctx, "portal_raid_kills", "idx_portal_raid_event", "realm_key,source_event_id"); err != nil {
			return fmt.Errorf("portal migration 8 raid index: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(8,'competitive event ingestion')"); err != nil {
			return fmt.Errorf("record migration version 8: %w", err)
		}
		version = 8
	}
	if version < 9 {
		statements := []string{
			`ALTER TABLE portal_account_security MODIFY COLUMN totp_secret VARBINARY(255) NULL`,
			`CREATE TABLE IF NOT EXISTS portal_totp_recovery_codes (
			 account_id INT UNSIGNED NOT NULL, code_hash BINARY(32) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(account_id,code_hash), INDEX idx_portal_totp_recovery_account(account_id)) ENGINE=InnoDB`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 9: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(9,'encrypted TOTP and recovery codes')"); err != nil {
			return fmt.Errorf("record migration version 9: %w", err)
		}
		version = 9
	}
	if version < 10 {
		if err := s.ensureColumn(ctx, "portal_sessions", "step_up_until", "DATETIME NULL"); err != nil {
			return fmt.Errorf("portal migration 10: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(10,'staff step-up authentication')"); err != nil {
			return fmt.Errorf("record migration version 10: %w", err)
		}
		version = 10
	}
	if version < 11 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_privacy_requests (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL,
		 request_type ENUM('deletion') NOT NULL, status ENUM('pending','processing','completed','rejected','cancelled') NOT NULL DEFAULT 'pending',
		 player_note VARCHAR(500) NOT NULL DEFAULT '', staff_note VARCHAR(500) NOT NULL DEFAULT '', handled_by INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 completed_at DATETIME NULL, INDEX idx_portal_privacy_account(account_id,created_at), INDEX idx_portal_privacy_queue(status,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 11: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(11,'privacy requests')"); err != nil {
			return fmt.Errorf("record migration version 11: %w", err)
		}
		version = 11
	}
	if version < 12 {
		columns := []struct{ name, definition string }{
			{"slug", "VARCHAR(160) NULL"},
			{"body", "MEDIUMTEXT NOT NULL"},
			{"cover_url", "VARCHAR(500) NOT NULL DEFAULT ''"},
			{"tags", "VARCHAR(500) NOT NULL DEFAULT ''"},
			{"author_name", "VARCHAR(100) NOT NULL DEFAULT ''"},
			{"status", "VARCHAR(20) NOT NULL DEFAULT 'published'"},
		}
		for _, column := range columns {
			if err := s.ensureColumn(ctx, "portal_news", column.name, column.definition); err != nil {
				return fmt.Errorf("portal migration 12 column %s: %w", column.name, err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, `UPDATE portal_news SET status=CASE WHEN active=1 THEN 'published' ELSE 'archived' END WHERE status='' OR status IS NULL`); err != nil {
			return fmt.Errorf("portal migration 12 state backfill: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_news_revisions (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, news_id BIGINT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL,
		 editor_account_id INT UNSIGNED NOT NULL DEFAULT 0, title VARCHAR(120) NOT NULL, slug VARCHAR(160) NULL,
		 summary VARCHAR(1000) NOT NULL DEFAULT '', body MEDIUMTEXT NOT NULL, url VARCHAR(500) NOT NULL DEFAULT '',
		 cover_url VARCHAR(500) NOT NULL DEFAULT '', tags VARCHAR(500) NOT NULL DEFAULT '', author_name VARCHAR(100) NOT NULL DEFAULT '',
		 kind VARCHAR(20) NOT NULL, status VARCHAR(20) NOT NULL, publish_at DATETIME NULL, expires_at DATETIME NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_news_revision(news_id,id), INDEX idx_portal_news_revision_realm(realm_key,created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 12 revisions: %w", err)
		}
		if err := s.ensureUniqueIndex(ctx, "portal_news", "idx_portal_news_slug", "realm_key,slug"); err != nil {
			return fmt.Errorf("portal migration 12 slug index: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(12,'article publishing and revisions')"); err != nil {
			return fmt.Errorf("record migration version 12: %w", err)
		}
		version = 12
	}
	if version < 13 {
		columns := []struct{ name, definition string }{
			{"category", "VARCHAR(40) NOT NULL DEFAULT 'general'"},
			{"priority", "VARCHAR(20) NOT NULL DEFAULT 'normal'"},
			{"tags", "VARCHAR(500) NOT NULL DEFAULT ''"},
			{"assigned_to", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"due_at", "DATETIME NULL"},
			{"first_response_at", "DATETIME NULL"},
			{"resolved_at", "DATETIME NULL"},
		}
		for _, column := range columns {
			if err := s.ensureColumn(ctx, "portal_support_tickets", column.name, column.definition); err != nil {
				return fmt.Errorf("portal migration 13 column %s: %w", column.name, err)
			}
		}
		statements := []string{
			`ALTER TABLE portal_support_tickets MODIFY COLUMN status ENUM('open','answered','pending_player','pending_staff','resolved','closed') NOT NULL DEFAULT 'open'`,
			`CREATE TABLE IF NOT EXISTS portal_ticket_events (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, ticket_id BIGINT UNSIGNED NOT NULL, actor_account_id INT UNSIGNED NOT NULL DEFAULT 0,
			 event_type VARCHAR(40) NOT NULL, details VARCHAR(1000) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_ticket_events(ticket_id,id)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_canned_replies (
			 id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, title VARCHAR(100) NOT NULL, body TEXT NOT NULL,
			 active TINYINT(1) NOT NULL DEFAULT 1, created_by INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 INDEX idx_portal_canned_replies(realm_key,active,title)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 13: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(13,'support triage and history')"); err != nil {
			return fmt.Errorf("record migration version 13: %w", err)
		}
		version = 13
	}
	if version < 14 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_pages (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, slug VARCHAR(120) NOT NULL,
			 title VARCHAR(160) NOT NULL, summary VARCHAR(1000) NOT NULL DEFAULT '', body MEDIUMTEXT NOT NULL,
			 status VARCHAR(20) NOT NULL DEFAULT 'draft', show_navigation TINYINT(1) NOT NULL DEFAULT 0,
			 show_footer TINYINT(1) NOT NULL DEFAULT 0, sort_order INT NOT NULL DEFAULT 0,
			 seo_title VARCHAR(160) NOT NULL DEFAULT '', seo_description VARCHAR(300) NOT NULL DEFAULT '',
			 updated_by INT UNSIGNED NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY idx_portal_page_slug(realm_key,slug), INDEX idx_portal_pages_public(realm_key,status,sort_order)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_page_revisions (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, page_id BIGINT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL,
			 editor_account_id INT UNSIGNED NOT NULL DEFAULT 0, title VARCHAR(160) NOT NULL, slug VARCHAR(120) NOT NULL,
			 summary VARCHAR(1000) NOT NULL DEFAULT '', body MEDIUMTEXT NOT NULL, status VARCHAR(20) NOT NULL,
			 show_navigation TINYINT(1) NOT NULL DEFAULT 0, show_footer TINYINT(1) NOT NULL DEFAULT 0, sort_order INT NOT NULL DEFAULT 0,
			 seo_title VARCHAR(160) NOT NULL DEFAULT '', seo_description VARCHAR(300) NOT NULL DEFAULT '',
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, INDEX idx_portal_page_revisions(page_id,id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 14: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(14,'custom content pages')"); err != nil {
			return fmt.Errorf("record migration version 14: %w", err)
		}
		version = 14
	}
	if version < 15 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_events (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, title VARCHAR(160) NOT NULL,
		 description VARCHAR(2000) NOT NULL DEFAULT '', category VARCHAR(40) NOT NULL DEFAULT 'community', location VARCHAR(120) NOT NULL DEFAULT '',
		 starts_at DATETIME NOT NULL, ends_at DATETIME NULL, url VARCHAR(500) NOT NULL DEFAULT '', status VARCHAR(20) NOT NULL DEFAULT 'scheduled',
		 max_participants INT UNSIGNED NOT NULL DEFAULT 0, created_by INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 INDEX idx_portal_events_upcoming(realm_key,status,starts_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 15: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(15,'realm events calendar')"); err != nil {
			return fmt.Errorf("record migration version 15: %w", err)
		}
		version = 15
	}
	if version < 16 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_transfer_requests (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL,
		 source_realm VARCHAR(120) NOT NULL, character_name VARCHAR(32) NOT NULL, source_profile_url VARCHAR(500) NOT NULL DEFAULT '',
		 player_note VARCHAR(1000) NOT NULL DEFAULT '', status VARCHAR(20) NOT NULL DEFAULT 'submitted',
		 staff_note VARCHAR(1000) NOT NULL DEFAULT '', handled_by INT UNSIGNED NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 completed_at DATETIME NULL, INDEX idx_portal_transfer_account(account_id,realm_key,created_at),
		 INDEX idx_portal_transfer_queue(realm_key,status,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 16: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(16,'character transfer requests')"); err != nil {
			return fmt.Errorf("record migration version 16: %w", err)
		}
		version = 16
	}
	if version < 17 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_credit_packages (
			 id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, slug VARCHAR(50) NOT NULL,
			 name VARCHAR(100) NOT NULL, credits INT UNSIGNED NOT NULL, stripe_price_id VARCHAR(255) NOT NULL,
			 bonus_label VARCHAR(100) NOT NULL DEFAULT '', active TINYINT(1) NOT NULL DEFAULT 1, sort_order INT NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY idx_portal_credit_package_slug(realm_key,slug), INDEX idx_portal_credit_packages(realm_key,active,sort_order)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_gifts (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, purchaser_account_id INT UNSIGNED NOT NULL, recipient_account_id INT UNSIGNED NOT NULL,
			 realm_key VARCHAR(64) NOT NULL, checkout_id VARCHAR(255) NOT NULL, credits INT UNSIGNED NOT NULL, message VARCHAR(500) NOT NULL DEFAULT '',
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, UNIQUE KEY idx_portal_gift_checkout(checkout_id),
			 INDEX idx_portal_gift_recipient(recipient_account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 17: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(17,'credit packages and gifting')"); err != nil {
			return fmt.Errorf("record migration version 17: %w", err)
		}
		version = 17
	}
	if version < 18 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_gift_codes (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, code_hash BINARY(32) NOT NULL,
			 code_hint VARCHAR(20) NOT NULL, credits INT UNSIGNED NOT NULL, max_uses INT UNSIGNED NOT NULL DEFAULT 1,
			 used_count INT UNSIGNED NOT NULL DEFAULT 0, expires_at DATETIME NULL, active TINYINT(1) NOT NULL DEFAULT 1,
			 created_by INT UNSIGNED NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY idx_portal_gift_code_hash(code_hash), INDEX idx_portal_gift_codes(realm_key,active,expires_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_gift_code_uses (
			 gift_code_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, credits INT UNSIGNED NOT NULL,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(gift_code_id,account_id),
			 INDEX idx_portal_gift_code_account(account_id,created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 18: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(18,'hashed gift codes')"); err != nil {
			return fmt.Errorf("record migration version 18: %w", err)
		}
		version = 18
	}
	if version < 19 {
		if err := s.ensureColumn(ctx, "portal_staff_roles", "realm_key", "VARCHAR(64) NOT NULL DEFAULT '*'"); err != nil {
			return fmt.Errorf("portal migration 19 realm scope: %w", err)
		}
		if err := s.ensureColumn(ctx, "portal_staff_roles", "expires_at", "DATETIME NULL"); err != nil {
			return fmt.Errorf("portal migration 19 expiry: %w", err)
		}
		if err := s.ensureColumn(ctx, "portal_staff_roles", "permissions_json", "TEXT NOT NULL"); err != nil {
			return fmt.Errorf("portal migration 19 permissions: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "ALTER TABLE portal_staff_roles DROP PRIMARY KEY, ADD PRIMARY KEY(account_id,realm_key)"); err != nil {
			return fmt.Errorf("portal migration 19 primary key: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(19,'scoped temporary staff access')"); err != nil {
			return fmt.Errorf("record migration version 19: %w", err)
		}
		version = 19
	}
	if version < 20 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_community_issues (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, account_id INT UNSIGNED NOT NULL,
			 kind VARCHAR(20) NOT NULL, title VARCHAR(160) NOT NULL, body TEXT NOT NULL, category VARCHAR(40) NOT NULL DEFAULT 'general',
			 status VARCHAR(24) NOT NULL DEFAULT 'open', priority VARCHAR(20) NOT NULL DEFAULT 'normal', labels VARCHAR(500) NOT NULL DEFAULT '',
			 staff_response TEXT NOT NULL, vote_count INT UNSIGNED NOT NULL DEFAULT 0, comment_count INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 INDEX idx_portal_issues_list(realm_key,kind,status,updated_at), INDEX idx_portal_issues_account(account_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_community_issue_votes (
			 issue_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(issue_id,account_id), INDEX idx_portal_issue_votes_account(account_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_community_issue_comments (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, issue_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL,
			 author_role VARCHAR(20) NOT NULL DEFAULT 'player', body TEXT NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_issue_comments(issue_id,id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 20: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(20,'community suggestions and issue tracking')"); err != nil {
			return fmt.Errorf("record migration version 20: %w", err)
		}
		version = 20
	}
	if version < 21 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_guild_recruitment (
			 realm_key VARCHAR(64) NOT NULL, guild_id INT UNSIGNED NOT NULL, headline VARCHAR(160) NOT NULL,
			 description TEXT NOT NULL, looking_for VARCHAR(500) NOT NULL DEFAULT '', schedule VARCHAR(300) NOT NULL DEFAULT '',
			 contact VARCHAR(300) NOT NULL DEFAULT '', active TINYINT(1) NOT NULL DEFAULT 1, updated_by INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 PRIMARY KEY(realm_key,guild_id), INDEX idx_portal_guild_recruiting(realm_key,active,updated_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_guild_applications (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, guild_id INT UNSIGNED NOT NULL,
			 account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL, message VARCHAR(2000) NOT NULL,
			 status VARCHAR(24) NOT NULL DEFAULT 'submitted', response VARCHAR(2000) NOT NULL DEFAULT '', staff_note VARCHAR(2000) NOT NULL DEFAULT '', handled_by INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 INDEX idx_portal_guild_applicant(account_id,realm_key,created_at), INDEX idx_portal_guild_application_queue(realm_key,guild_id,status,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 21: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(21,'guild recruitment and applications')"); err != nil {
			return fmt.Errorf("record migration version 21: %w", err)
		}
	}
	if version < 22 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_arena_seasons (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, name VARCHAR(100) NOT NULL,
			 slug VARCHAR(100) NOT NULL, status VARCHAR(20) NOT NULL DEFAULT 'archived', starts_at DATETIME NULL, ends_at DATETIME NULL,
			 created_by INT UNSIGNED NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_arena_season(realm_key,slug), INDEX idx_portal_arena_seasons(realm_key,status,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_arena_snapshots (
			 season_id BIGINT UNSIGNED NOT NULL, bracket TINYINT UNSIGNED NOT NULL, rank_no INT UNSIGNED NOT NULL,
			 team_id INT UNSIGNED NOT NULL, team_name VARCHAR(100) NOT NULL, rating SMALLINT UNSIGNED NOT NULL,
			 season_games SMALLINT UNSIGNED NOT NULL, season_wins SMALLINT UNSIGNED NOT NULL, members_json TEXT NOT NULL,
			 captured_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(season_id,bracket,rank_no), INDEX idx_portal_arena_snapshot_team(season_id,team_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 22: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(22,'arena season snapshots')"); err != nil {
			return fmt.Errorf("record migration version 22: %w", err)
		}
	}
	if version < 23 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_resources (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, kind VARCHAR(20) NOT NULL,
		 title VARCHAR(160) NOT NULL, slug VARCHAR(160) NOT NULL, summary VARCHAR(1000) NOT NULL DEFAULT '', body TEXT NOT NULL,
		 version VARCHAR(40) NOT NULL DEFAULT '', download_url VARCHAR(2000) NOT NULL DEFAULT '', image_url VARCHAR(2000) NOT NULL DEFAULT '',
		 tags VARCHAR(500) NOT NULL DEFAULT '', status VARCHAR(20) NOT NULL DEFAULT 'draft', sort_order INT NOT NULL DEFAULT 0,
		 created_by INT UNSIGNED NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 UNIQUE KEY uq_portal_resource(realm_key,slug), INDEX idx_portal_resources(realm_key,kind,status,sort_order,updated_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 23: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(23,'addon and weakaura library')"); err != nil {
			return fmt.Errorf("record migration version 23: %w", err)
		}
	}
	if version < 24 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_character_privacy (
		 account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL, character_guid INT UNSIGNED NOT NULL,
		 hidden TINYINT(1) NOT NULL DEFAULT 0, show_gear TINYINT(1) NOT NULL DEFAULT 1, show_activity TINYINT(1) NOT NULL DEFAULT 1,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
		 PRIMARY KEY(realm_key,character_guid), INDEX idx_portal_character_privacy_account(account_id,realm_key)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 24: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(24,'character armory privacy')"); err != nil {
			return fmt.Errorf("record migration version 24: %w", err)
		}
	}
	if version < 25 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_payment_transactions (
			 checkout_id VARCHAR(255) PRIMARY KEY, payment_intent VARCHAR(255) NOT NULL DEFAULT '', realm_key VARCHAR(64) NOT NULL,
			 purchaser_account_id INT UNSIGNED NOT NULL, recipient_account_id INT UNSIGNED NOT NULL, credits INT UNSIGNED NOT NULL,
			 amount_total BIGINT UNSIGNED NOT NULL DEFAULT 0, currency VARCHAR(10) NOT NULL DEFAULT '', status VARCHAR(30) NOT NULL DEFAULT 'paid',
			 receipt_url VARCHAR(2000) NOT NULL DEFAULT '', refunded_credits INT UNSIGNED NOT NULL DEFAULT 0, dispute_id VARCHAR(255) NOT NULL DEFAULT '',
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_payment_intent(payment_intent), INDEX idx_portal_payment_account(recipient_account_id,created_at), INDEX idx_portal_payment_realm(realm_key,status,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_payment_webhooks (
			 event_id VARCHAR(255) PRIMARY KEY, event_type VARCHAR(100) NOT NULL, object_id VARCHAR(255) NOT NULL DEFAULT '',
			 payload_sha256 BINARY(32) NOT NULL, processed TINYINT(1) NOT NULL DEFAULT 0, error_message VARCHAR(500) NOT NULL DEFAULT '',
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, processed_at TIMESTAMP NULL
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 25: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(25,'payment receipts refunds and disputes')"); err != nil {
			return fmt.Errorf("record migration version 25: %w", err)
		}
	}
	if version < 26 {
		statements := []string{
			`ALTER TABLE portal_admin_audit
			 ADD COLUMN realm_key VARCHAR(64) NOT NULL DEFAULT '',
			 ADD COLUMN request_id VARCHAR(64) NOT NULL DEFAULT '',
			 ADD COLUMN ip_address VARCHAR(45) NOT NULL DEFAULT '',
			 ADD COLUMN user_agent VARCHAR(500) NOT NULL DEFAULT '',
			 ADD COLUMN before_json JSON NULL,
			 ADD COLUMN after_json JSON NULL,
			 ADD COLUMN metadata_json JSON NULL`,
			`CREATE INDEX idx_portal_admin_audit_realm ON portal_admin_audit(realm_key,created_at)`,
			`ALTER TABLE portal_sessions ADD COLUMN identity_id BIGINT UNSIGNED NOT NULL DEFAULT 0`,
			`CREATE INDEX idx_portal_sessions_identity ON portal_sessions(identity_id,expires_at)`,
			`CREATE TABLE IF NOT EXISTS portal_identities (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, email VARCHAR(255) NOT NULL DEFAULT '', display_name VARCHAR(100) NOT NULL DEFAULT '',
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_identity_accounts (
			 identity_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, label VARCHAR(100) NOT NULL DEFAULT '', is_primary TINYINT(1) NOT NULL DEFAULT 0,
			 linked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(identity_id,account_id), UNIQUE KEY uq_portal_identity_game_account(account_id),
			 INDEX idx_portal_identity_accounts(identity_id,is_primary,linked_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_wishlist (
			 account_id INT UNSIGNED NOT NULL, realm_key VARCHAR(64) NOT NULL, product_id INT UNSIGNED NOT NULL,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(account_id,realm_key,product_id), INDEX idx_portal_wishlist_product(realm_key,product_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_audit_filters (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, name VARCHAR(80) NOT NULL,
			 query_json JSON NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_audit_filter(account_id,name), INDEX idx_portal_audit_filter_account(account_id,updated_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_battleground_matches (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, source_event_id VARCHAR(128) NOT NULL,
			 battleground VARCHAR(100) NOT NULL, winning_team VARCHAR(20) NOT NULL, duration_seconds INT UNSIGNED NOT NULL DEFAULT 0,
			 played_at DATETIME NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_battleground_event(realm_key,source_event_id), INDEX idx_portal_battleground_recent(realm_key,played_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_battleground_members (
			 match_id BIGINT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL DEFAULT 0, character_name VARCHAR(32) NOT NULL,
			 team_name VARCHAR(20) NOT NULL, class_id TINYINT UNSIGNED NOT NULL DEFAULT 0, killing_blows INT UNSIGNED NOT NULL DEFAULT 0,
			 honorable_kills INT UNSIGNED NOT NULL DEFAULT 0, deaths INT UNSIGNED NOT NULL DEFAULT 0,
			 damage_done BIGINT UNSIGNED NOT NULL DEFAULT 0, healing_done BIGINT UNSIGNED NOT NULL DEFAULT 0,
			 PRIMARY KEY(match_id,character_name), INDEX idx_portal_battleground_character(character_name,match_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_ranking_exclusions (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, scope VARCHAR(20) NOT NULL,
			 target_key VARCHAR(100) NOT NULL, reason VARCHAR(500) NOT NULL, active TINYINT(1) NOT NULL DEFAULT 1,
			 starts_at DATETIME NULL, ends_at DATETIME NULL, created_by INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 INDEX idx_portal_ranking_exclusion(realm_key,scope,active,ends_at), UNIQUE KEY uq_portal_ranking_exclusion(realm_key,scope,target_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 26: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(26,'structured audit context')"); err != nil {
			return fmt.Errorf("record migration version 26: %w", err)
		}
	}
	if version < 27 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_identity_providers (
			 identity_id BIGINT UNSIGNED NOT NULL, provider VARCHAR(30) NOT NULL, provider_user_id VARCHAR(128) NOT NULL,
			 username VARCHAR(100) NOT NULL DEFAULT '', email VARCHAR(255) NOT NULL DEFAULT '', avatar_url VARCHAR(500) NOT NULL DEFAULT '',
			 linked_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 PRIMARY KEY(identity_id,provider), UNIQUE KEY uq_portal_provider_user(provider,provider_user_id), INDEX idx_portal_provider_identity(identity_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_oauth_states (
			 state_hash BINARY(32) PRIMARY KEY, provider VARCHAR(30) NOT NULL, flow_mode VARCHAR(20) NOT NULL,
			 identity_id BIGINT UNSIGNED NOT NULL DEFAULT 0, redirect_path VARCHAR(255) NOT NULL DEFAULT '/account/security', code_verifier VARCHAR(128) NOT NULL,
			 expires_at DATETIME NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_oauth_expiry(expires_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 27: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(27,'external identity providers')"); err != nil {
			return fmt.Errorf("record migration version 27: %w", err)
		}
	}
	if version < 28 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_passkey_credentials (
			 credential_id VARBINARY(1024) PRIMARY KEY, identity_id BIGINT UNSIGNED NOT NULL, name VARCHAR(100) NOT NULL DEFAULT 'Passkey',
			 public_key_x BINARY(32) NOT NULL, public_key_y BINARY(32) NOT NULL, sign_count BIGINT UNSIGNED NOT NULL DEFAULT 0,
			 transports VARCHAR(100) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, last_used_at DATETIME NULL,
			 INDEX idx_portal_passkey_identity(identity_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_passkey_challenges (
			 challenge_hash BINARY(32) PRIMARY KEY, flow_mode VARCHAR(20) NOT NULL, identity_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			 expires_at DATETIME NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_passkey_expiry(expires_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_media_assets (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, content_hash BINARY(32) NOT NULL,
			 file_name VARCHAR(180) NOT NULL, mime_type VARCHAR(80) NOT NULL, width INT UNSIGNED NOT NULL, height INT UNSIGNED NOT NULL,
			 alt_text VARCHAR(300) NOT NULL DEFAULT '', data MEDIUMBLOB NOT NULL, active TINYINT(1) NOT NULL DEFAULT 1,
			 uploaded_by INT UNSIGNED NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_media_hash(realm_key,content_hash), INDEX idx_portal_media_realm(realm_key,active,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_navigation_items (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, area VARCHAR(20) NOT NULL,
			 label VARCHAR(80) NOT NULL, url VARCHAR(500) NOT NULL, sort_order INT NOT NULL DEFAULT 0, new_tab TINYINT(1) NOT NULL DEFAULT 0,
			 active TINYINT(1) NOT NULL DEFAULT 1, created_by INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 INDEX idx_portal_navigation(realm_key,area,active,sort_order)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 28: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(28,'passkeys managed media and navigation')"); err != nil {
			return fmt.Errorf("record migration version 28: %w", err)
		}
	}
	if version < 29 {
		statements := []string{
			`ALTER TABLE portal_raid_kills
			 ADD COLUMN eligible TINYINT(1) NOT NULL DEFAULT 0,
			 ADD COLUMN eligibility_reason VARCHAR(500) NOT NULL DEFAULT 'Legacy event has not been verified',
			 ADD COLUMN verified_members SMALLINT UNSIGNED NOT NULL DEFAULT 0,
			 ADD COLUMN source_kind VARCHAR(30) NOT NULL DEFAULT 'ingest'`,
			`CREATE INDEX idx_portal_raid_eligible ON portal_raid_kills(realm_key,eligible,raid,difficulty,duration_seconds)`,
			`CREATE TABLE IF NOT EXISTS portal_raid_eligibility_rules (
			 realm_key VARCHAR(64) PRIMARY KEY, min_members_10 TINYINT UNSIGNED NOT NULL DEFAULT 8,
			 max_members_10 TINYINT UNSIGNED NOT NULL DEFAULT 10, min_members_25 TINYINT UNSIGNED NOT NULL DEFAULT 20,
			 max_members_25 TINYINT UNSIGNED NOT NULL DEFAULT 25, min_duration_seconds INT UNSIGNED NOT NULL DEFAULT 60,
			 max_duration_seconds INT UNSIGNED NOT NULL DEFAULT 21600, max_event_age_hours INT UNSIGNED NOT NULL DEFAULT 168,
			 require_character_guids TINYINT(1) NOT NULL DEFAULT 1, updated_by INT UNSIGNED NOT NULL DEFAULT 0,
			 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 29: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(29,'verified raid ranking eligibility')"); err != nil {
			return fmt.Errorf("record migration version 29: %w", err)
		}
	}
	if version < 30 {
		columns := []struct{ table, column, definition string }{
			{"portal_products", "tags", "VARCHAR(500) NOT NULL DEFAULT ''"},
			{"portal_products", "visibility_segment", "VARCHAR(30) NOT NULL DEFAULT 'all'"},
			{"portal_products", "variant_required", "TINYINT(1) NOT NULL DEFAULT 0"},
			{"portal_products", "bundle_template_id", "BIGINT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_orders", "variant_id", "BIGINT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_orders", "variant_name", "VARCHAR(100) NOT NULL DEFAULT ''"},
			{"portal_coupons", "allow_sale", "TINYINT(1) NOT NULL DEFAULT 0"},
			{"portal_coupons", "min_subtotal", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_coupons", "category", "VARCHAR(40) NOT NULL DEFAULT ''"},
		}
		for _, column := range columns {
			if err := s.ensureColumn(ctx, column.table, column.column, column.definition); err != nil {
				return fmt.Errorf("portal migration 30 %s.%s: %w", column.table, column.column, err)
			}
		}
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_product_variants (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, product_id INT UNSIGNED NOT NULL, name VARCHAR(100) NOT NULL,
			 sku VARCHAR(80) NOT NULL, price_adjustment INT NOT NULL DEFAULT 0, active TINYINT(1) NOT NULL DEFAULT 1,
			 sort_order INT NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_product_variant_sku(product_id,sku), INDEX idx_portal_product_variants(product_id,active,sort_order)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_product_variant_items (
			 variant_id BIGINT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL DEFAULT 1,
			 PRIMARY KEY(variant_id,item_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_shop_collections (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, slug VARCHAR(100) NOT NULL,
			 name VARCHAR(100) NOT NULL, description VARCHAR(500) NOT NULL DEFAULT '', image_url VARCHAR(500) NOT NULL DEFAULT '',
			 active TINYINT(1) NOT NULL DEFAULT 1, featured TINYINT(1) NOT NULL DEFAULT 0, sort_order INT NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_collection_slug(realm_key,slug), INDEX idx_portal_collections(realm_key,active,sort_order)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_collection_products (
			 collection_id BIGINT UNSIGNED NOT NULL, product_id INT UNSIGNED NOT NULL, sort_order INT NOT NULL DEFAULT 0,
			 PRIMARY KEY(collection_id,product_id), INDEX idx_portal_collection_product(product_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_bundle_templates (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, name VARCHAR(100) NOT NULL,
			 description VARCHAR(500) NOT NULL DEFAULT '', created_by INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_bundle_name(realm_key,name)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_bundle_template_items (
			 bundle_id BIGINT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL DEFAULT 1,
			 PRIMARY KEY(bundle_id,item_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_stock_movements (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, product_id INT UNSIGNED NOT NULL,
			 quantity_delta INT NOT NULL, movement_type VARCHAR(30) NOT NULL, reference_id VARCHAR(100) NOT NULL DEFAULT '',
			 reason VARCHAR(500) NOT NULL DEFAULT '', actor_account_id INT UNSIGNED NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_stock_product(product_id,created_at), INDEX idx_portal_stock_realm(realm_key,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_coupon_events (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, coupon_id BIGINT UNSIGNED NOT NULL, actor_account_id INT UNSIGNED NOT NULL DEFAULT 0,
			 action VARCHAR(30) NOT NULL, snapshot_json JSON NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_coupon_events(coupon_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 30: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(30,'shop merchandising and stock ledger')"); err != nil {
			return fmt.Errorf("record migration version 30: %w", err)
		}
	}
	if version < 31 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_external_reward_events (
			 provider VARCHAR(30) NOT NULL, provider_event_id VARCHAR(128) NOT NULL, realm_key VARCHAR(64) NOT NULL,
			 provider_user_id VARCHAR(128) NOT NULL, identity_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL,
			 credits INT UNSIGNED NOT NULL, reason VARCHAR(255) NOT NULL, metadata_json JSON NULL,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(provider,provider_event_id), INDEX idx_portal_external_rewards_account(account_id,created_at),
			 INDEX idx_portal_external_rewards_realm(realm_key,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_referral_milestones (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, name VARCHAR(100) NOT NULL,
			 referral_count INT UNSIGNED NOT NULL, reward_credits INT UNSIGNED NOT NULL, active TINYINT(1) NOT NULL DEFAULT 1,
			 sort_order INT NOT NULL DEFAULT 0, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_referral_milestone(realm_key,referral_count), INDEX idx_portal_referral_milestones(realm_key,active,sort_order)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_referral_milestone_claims (
			 milestone_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, credits INT UNSIGNED NOT NULL,
			 claimed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, PRIMARY KEY(milestone_id,account_id),
			 INDEX idx_portal_referral_claims_account(account_id,claimed_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_vote_campaigns (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, name VARCHAR(120) NOT NULL,
			 description VARCHAR(500) NOT NULL DEFAULT '', starts_at DATETIME NOT NULL, ends_at DATETIME NOT NULL,
			 minimum_votes INT UNSIGNED NOT NULL DEFAULT 1, winner_count INT UNSIGNED NOT NULL DEFAULT 1,
			 prize_description VARCHAR(255) NOT NULL, target_entries INT UNSIGNED NOT NULL DEFAULT 0,
			 community_reward_description VARCHAR(255) NOT NULL DEFAULT '', draw_seed VARCHAR(128) NOT NULL, seed_commitment CHAR(64) NOT NULL,
			 status VARCHAR(20) NOT NULL DEFAULT 'scheduled', created_by INT UNSIGNED NOT NULL DEFAULT 0,
			 drawn_at DATETIME NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_vote_campaigns(realm_key,status,starts_at,ends_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_vote_campaign_winners (
			 campaign_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, rank_no INT UNSIGNED NOT NULL,
			 vote_count INT UNSIGNED NOT NULL, draw_hash CHAR(64) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(campaign_id,account_id), UNIQUE KEY uq_portal_vote_campaign_rank(campaign_id,rank_no)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 31: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(31,'retention rewards and transparent vote campaigns')"); err != nil {
			return fmt.Errorf("record migration version 31: %w", err)
		}
	}
	if version < 32 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_sanctions (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, moderation_log_id BIGINT UNSIGNED NOT NULL DEFAULT 0,
			 account_id INT UNSIGNED NOT NULL, character_name VARCHAR(24) NOT NULL DEFAULT '', sanction_type VARCHAR(30) NOT NULL,
			 reason VARCHAR(255) NOT NULL, status VARCHAR(20) NOT NULL DEFAULT 'active', starts_at DATETIME NOT NULL,
			 expires_at DATETIME NULL, lifted_at DATETIME NULL, created_by INT UNSIGNED NOT NULL,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 INDEX idx_portal_sanctions_account(account_id,status,created_at), INDEX idx_portal_sanctions_realm(realm_key,status,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_sanction_notes (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, sanction_id BIGINT UNSIGNED NOT NULL, actor_account_id INT UNSIGNED NOT NULL,
			 body TEXT NOT NULL, evidence_url VARCHAR(500) NOT NULL DEFAULT '', created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_sanction_notes(sanction_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_sanction_appeals (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, sanction_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL,
			 message TEXT NOT NULL, status VARCHAR(20) NOT NULL DEFAULT 'submitted', staff_response TEXT NULL,
			 reviewed_by INT UNSIGNED NOT NULL DEFAULT 0, reviewed_at DATETIME NULL,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_sanction_appeal(sanction_id,account_id), INDEX idx_portal_appeals_status(status,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 32: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(32,'sanction case history and appeals')"); err != nil {
			return fmt.Errorf("record migration version 32: %w", err)
		}
	}
	if version < 33 {
		columns := []struct{ table, column, definition string }{
			{"portal_pvp_matches", "season_slug", "VARCHAR(80) NOT NULL DEFAULT 'current'"},
			{"portal_pvp_matches", "team_id", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_pvp_matches", "opponent_id", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_pvp_matches", "rating_before", "SMALLINT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_pvp_matches", "rating_after", "SMALLINT UNSIGNED NOT NULL DEFAULT 0"},
			{"portal_pvp_matches", "duration_seconds", "INT UNSIGNED NOT NULL DEFAULT 0"},
		}
		for _, column := range columns {
			if err := s.ensureColumn(ctx, column.table, column.column, column.definition); err != nil {
				return fmt.Errorf("portal migration 33 %s.%s: %w", column.table, column.column, err)
			}
		}
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_raid_attempts (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, source_event_id VARCHAR(128) NOT NULL,
			 guild_id INT UNSIGNED NOT NULL, guild_name VARCHAR(80) NOT NULL, raid VARCHAR(80) NOT NULL, boss VARCHAR(80) NOT NULL,
			 difficulty VARCHAR(30) NOT NULL, result VARCHAR(10) NOT NULL, attempt_number INT UNSIGNED NOT NULL DEFAULT 0,
			 duration_seconds INT UNSIGNED NOT NULL DEFAULT 0, boss_health_pct DECIMAL(5,2) NOT NULL DEFAULT 0,
			 occurred_at DATETIME NOT NULL, verified_members SMALLINT UNSIGNED NOT NULL DEFAULT 0, source_kind VARCHAR(30) NOT NULL DEFAULT 'signed_ingest',
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_raid_attempt_event(realm_key,source_event_id),
			 INDEX idx_portal_raid_attempt_recent(realm_key,raid,boss,difficulty,occurred_at),
			 INDEX idx_portal_raid_attempt_guild(realm_key,guild_id,occurred_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_raid_attempt_members (
			 attempt_id BIGINT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL DEFAULT 0, character_name VARCHAR(32) NOT NULL,
			 class_id TINYINT UNSIGNED NOT NULL DEFAULT 0, role_name VARCHAR(20) NOT NULL DEFAULT '',
			 PRIMARY KEY(attempt_id,character_name), INDEX idx_portal_raid_attempt_character(character_name,attempt_id)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 33: %w", err)
			}
		}
		if err := s.ensureIndex(ctx, "portal_pvp_matches", "idx_portal_pvp_season", "realm_key,season_slug,played_at"); err != nil {
			return fmt.Errorf("portal migration 33 pvp season index: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(33,'competitive attempt and rating history')"); err != nil {
			return fmt.Errorf("record migration version 33: %w", err)
		}
	}
	if version < 34 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_moderation_evidence (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL,
			 target_account_id INT UNSIGNED NOT NULL, case_reference VARCHAR(80) NOT NULL DEFAULT '',
			 note TEXT NOT NULL, evidence_url VARCHAR(500) NOT NULL DEFAULT '', actor_account_id INT UNSIGNED NOT NULL,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 INDEX idx_portal_moderation_evidence_target(realm_key,target_account_id,created_at),
			 INDEX idx_portal_moderation_evidence_actor(actor_account_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 34: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(34,'privacy-aware moderation investigations')"); err != nil {
			return fmt.Errorf("record migration version 34: %w", err)
		}
	}
	if version < 35 {
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_missions (
			 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL, slug VARCHAR(80) NOT NULL,
			 name VARCHAR(120) NOT NULL, description VARCHAR(500) NOT NULL DEFAULT '', category VARCHAR(20) NOT NULL,
			 metric VARCHAR(40) NOT NULL, target_value INT UNSIGNED NOT NULL, reward_credits INT UNSIGNED NOT NULL,
			 active TINYINT(1) NOT NULL DEFAULT 1, sort_order INT NOT NULL DEFAULT 0,
			 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 UNIQUE KEY uq_portal_mission_slug(realm_key,slug), INDEX idx_portal_missions(realm_key,active,sort_order)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_mission_claims (
			 mission_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, period_key CHAR(7) NOT NULL,
			 credits INT UNSIGNED NOT NULL, claimed_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(mission_id,account_id,period_key), INDEX idx_portal_mission_claim_account(account_id,period_key)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 35: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(35,'loyalty and monthly player missions')"); err != nil {
			return fmt.Errorf("record migration version 35: %w", err)
		}
	}
	if version < 36 {
		if err := s.ensureColumn(ctx, "portal_guild_recruitment", "discord_url", "VARCHAR(500) NOT NULL DEFAULT ''"); err != nil {
			return fmt.Errorf("portal migration 36 guild Discord URL: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(36,'guild Discord recruitment links')"); err != nil {
			return fmt.Errorf("record migration version 36: %w", err)
		}
	}
	if version < 37 {
		columns := []struct {
			name, definition string
		}{
			{"virus_total_url", "VARCHAR(500) NOT NULL DEFAULT ''"},
			{"changelog_url", "VARCHAR(500) NOT NULL DEFAULT ''"},
			{"released_at", "DATETIME NULL"},
			{"requirements", "VARCHAR(1000) NOT NULL DEFAULT ''"},
		}
		for _, column := range columns {
			if err := s.ensureColumn(ctx, "portal_downloads", column.name, column.definition); err != nil {
				return fmt.Errorf("portal migration 37 download %s: %w", column.name, err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(37,'download trust metadata')"); err != nil {
			return fmt.Errorf("record migration version 37: %w", err)
		}
	}
	if version < 38 {
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_forced_password_resets (
		 account_id INT UNSIGNED PRIMARY KEY, actor_account_id INT UNSIGNED NOT NULL, reason VARCHAR(255) NOT NULL,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_forced_password_reset_actor(actor_account_id,created_at)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 38: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(38,'staff-required password resets')"); err != nil {
			return fmt.Errorf("record migration version 38: %w", err)
		}
	}
	if version < 39 {
		if err := s.ensureColumn(ctx, "portal_downloads", "mirrors_json", "TEXT NULL"); err != nil {
			return fmt.Errorf("portal migration 39 download mirrors: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS portal_launcher_patches (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, realm_key VARCHAR(64) NOT NULL,
		 platform VARCHAR(30) NOT NULL, from_version VARCHAR(40) NOT NULL, to_version VARCHAR(40) NOT NULL,
		 url VARCHAR(500) NOT NULL, mirrors_json TEXT NULL, file_size VARCHAR(40) NOT NULL DEFAULT '',
		 sha256 CHAR(64) NOT NULL, signature_url VARCHAR(500) NOT NULL DEFAULT '', notes VARCHAR(500) NOT NULL DEFAULT '',
		 released_at DATETIME NULL, active TINYINT(1) NOT NULL DEFAULT 1, sort_order INT NOT NULL DEFAULT 0,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
		 INDEX idx_portal_launcher_patches(realm_key,active,platform,sort_order),
		 UNIQUE KEY uq_portal_launcher_patch(realm_key,platform,from_version,to_version)
		) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`); err != nil {
			return fmt.Errorf("portal migration 39 launcher patches: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(39,'download mirrors and launcher patch graph')"); err != nil {
			return fmt.Errorf("record migration version 39: %w", err)
		}
	}
	if version < 40 {
		columns := []struct {
			name, definition string
		}{
			{"signup_enabled", "TINYINT(1) NOT NULL DEFAULT 0"},
			{"registration_deadline", "DATETIME NULL"},
			{"reward_credits", "INT UNSIGNED NOT NULL DEFAULT 0"},
		}
		for _, column := range columns {
			if err := s.ensureColumn(ctx, "portal_events", column.name, column.definition); err != nil {
				return fmt.Errorf("portal migration 40 event %s: %w", column.name, err)
			}
		}
		statements := []string{
			`CREATE TABLE IF NOT EXISTS portal_event_registrations (
			 event_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL,
			 status VARCHAR(20) NOT NULL DEFAULT 'registered', registered_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
			 PRIMARY KEY(event_id,account_id), INDEX idx_portal_event_registration_character(character_guid,event_id),
			 INDEX idx_portal_event_registration_status(event_id,status)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
			`CREATE TABLE IF NOT EXISTS portal_event_reward_grants (
			 event_id BIGINT UNSIGNED NOT NULL, account_id INT UNSIGNED NOT NULL, credits INT UNSIGNED NOT NULL,
			 granted_by INT UNSIGNED NOT NULL, reason VARCHAR(255) NOT NULL, created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
			 PRIMARY KEY(event_id,account_id), INDEX idx_portal_event_reward_account(account_id,created_at)
			) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		}
		for _, statement := range statements {
			if _, err := s.Auth.ExecContext(ctx, statement); err != nil {
				return fmt.Errorf("portal migration 40: %w", err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(40,'event registration attendance and rewards')"); err != nil {
			return fmt.Errorf("record migration version 40: %w", err)
		}
	}
	if version < 41 {
		if err := s.ensureIndex(ctx, "portal_vote_events", "idx_portal_vote_ip_cooldown", "site_id,ip_hash,voted_at"); err != nil {
			return fmt.Errorf("portal migration 41 vote IP cooldown index: %w", err)
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(41,'vote IP cooldown enforcement')"); err != nil {
			return fmt.Errorf("record migration version 41: %w", err)
		}
	}
	if version < 42 {
		for _, column := range []struct{ name, definition string }{
			{"target_entries", "INT UNSIGNED NOT NULL DEFAULT 0"},
			{"community_reward_description", "VARCHAR(255) NOT NULL DEFAULT ''"},
		} {
			if err := s.ensureColumn(ctx, "portal_vote_campaigns", column.name, column.definition); err != nil {
				return fmt.Errorf("portal migration 42 vote campaign %s: %w", column.name, err)
			}
		}
		if _, err := s.Auth.ExecContext(ctx, "INSERT INTO portal_schema_migrations(version,name) VALUES(42,'community voting goals')"); err != nil {
			return fmt.Errorf("record migration version 42: %w", err)
		}
	}
	if s.C.AuditIPRetentionDays > 0 {
		_, _ = s.Auth.ExecContext(ctx, `UPDATE portal_admin_audit SET ip_address='',user_agent='' WHERE created_at < TIMESTAMPADD(DAY,-?,NOW()) AND (ip_address<>'' OR user_agent<>'')`, s.C.AuditIPRetentionDays)
		_, _ = s.Auth.ExecContext(ctx, `UPDATE portal_command_log SET ip_address='' WHERE created_at < TIMESTAMPADD(DAY,-?,NOW()) AND ip_address<>''`, s.C.AuditIPRetentionDays)
	}
	if s.C.AuditRetentionDays > 0 {
		cutoff := s.C.AuditRetentionDays
		for _, table := range []string{"portal_admin_audit", "portal_moderation_log", "portal_command_log", "portal_moderation_evidence"} {
			_, _ = s.Auth.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s` WHERE created_at < TIMESTAMPADD(DAY,-?,NOW())", table), cutoff)
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

func (s *Store) ensureUniqueIndex(ctx context.Context, table, index, columns string) error {
	var exists int
	if err := s.Auth.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema=? AND table_name=? AND index_name=?`, s.C.AuthDB, table, index).Scan(&exists); err != nil {
		return err
	}
	if exists > 0 {
		return nil
	}
	_, err := s.Auth.ExecContext(ctx, fmt.Sprintf("CREATE UNIQUE INDEX `%s` ON `%s` (%s)", index, table, columns))
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
		key, name, description, tier, action string
		price, gold                          uint32
		level                                uint8
		items                                []catalogItem
	}{
		{"service-level-80", "Complete Level 80 Boost", "Reach level 80 with all class spell ranks, every supported weapon skill at 400, Artisan Riding, Cold Weather Flying, four bags, faction mounts, and 10,000 gold.", "Level 80", "", 40, level80StarterGold, 80, level80StarterItems},
		{"service-race-change", "Race Change", "Choose a new race from your current faction on your next login.", "Character", "race_change", 35, 0, 0, nil},
		{"service-faction-change", "Faction Change", "Choose a compatible race from the opposite faction on your next login.", "Character", "faction_change", 50, 0, 0, nil},
	}
	for _, service := range services {
		seedKey := s.C.RealmKey + ":" + service.key
		_, err := s.Auth.ExecContext(ctx, `INSERT INTO portal_products(seed_key,name,description,item_id,quantity,price,category,tier_label,service_level,gold_amount,service_action,active,realm_key)
			VALUES(?,?,?,0,0,?,'Services',?,?,?,?,1,?)
			ON DUPLICATE KEY UPDATE name=VALUES(name),description=VALUES(description),price=VALUES(price),category=VALUES(category),tier_label=VALUES(tier_label),service_level=VALUES(service_level),gold_amount=VALUES(gold_amount),service_action=VALUES(service_action),active=1`,
			seedKey, service.name, service.description, service.price, service.tier, service.level, service.gold, service.action, s.C.RealmKey)
		if err != nil {
			return fmt.Errorf("seed %s: %w", service.key, err)
		}
		if len(service.items) == 0 {
			continue
		}
		var productID uint32
		if err = s.Auth.QueryRowContext(ctx, "SELECT id FROM portal_products WHERE seed_key=? AND realm_key=?", seedKey, s.C.RealmKey).Scan(&productID); err != nil {
			return fmt.Errorf("resolve seeded service %s: %w", service.key, err)
		}
		tx, beginErr := s.Auth.BeginTx(ctx, nil)
		if beginErr != nil {
			return beginErr
		}
		if _, err = tx.ExecContext(ctx, "DELETE FROM portal_product_items WHERE product_id=?", productID); err == nil {
			for _, item := range service.items {
				if _, err = tx.ExecContext(ctx, "INSERT INTO portal_product_items(product_id,item_id,quantity) VALUES(?,?,?)", productID, item.id, item.quantity); err != nil {
					break
				}
			}
		}
		if err != nil {
			tx.Rollback()
			return fmt.Errorf("seed %s contents: %w", service.key, err)
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

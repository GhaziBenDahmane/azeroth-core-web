package store

import (
	"context"
	"database/sql"
	"fmt"
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
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, INDEX idx_portal_session_account (account_id), INDEX idx_portal_session_expiry (expires_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_wallets (
		 account_id INT UNSIGNED PRIMARY KEY, balance INT UNSIGNED NOT NULL DEFAULT 0,
		 updated_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP) ENGINE=InnoDB`,
		`CREATE TABLE IF NOT EXISTS portal_products (
		 id INT UNSIGNED AUTO_INCREMENT PRIMARY KEY, name VARCHAR(100) NOT NULL, description VARCHAR(500) NOT NULL DEFAULT '',
		 item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL DEFAULT 1, price INT UNSIGNED NOT NULL,
		 category VARCHAR(40) NOT NULL DEFAULT 'Items', image_url VARCHAR(500) NOT NULL DEFAULT '', active TINYINT(1) NOT NULL DEFAULT 1,
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, INDEX idx_portal_products_active (active)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
		`CREATE TABLE IF NOT EXISTS portal_orders (
		 id BIGINT UNSIGNED AUTO_INCREMENT PRIMARY KEY, account_id INT UNSIGNED NOT NULL, character_guid INT UNSIGNED NOT NULL,
		 product_id INT UNSIGNED NOT NULL, item_id INT UNSIGNED NOT NULL, quantity INT UNSIGNED NOT NULL, total INT UNSIGNED NOT NULL,
		 status ENUM('processing','delivered','failed') NOT NULL DEFAULT 'processing', error_message VARCHAR(500) NOT NULL DEFAULT '',
		 created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP, delivered_at TIMESTAMP NULL,
		 INDEX idx_portal_orders_account (account_id, created_at)) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4`,
	}
	for _, q := range statements {
		if _, err := s.Auth.ExecContext(ctx, q); err != nil {
			return fmt.Errorf("portal migration: %w", err)
		}
	}
	_, _ = s.Auth.ExecContext(ctx, `DELETE FROM portal_sessions WHERE expires_at < NOW()`)
	return nil
}

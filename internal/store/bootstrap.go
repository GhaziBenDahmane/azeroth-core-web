package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/srp"
	_ "github.com/go-sql-driver/mysql"
)

// BootstrapAccount idempotently creates an AzerothCore account for deployment
// automation. Existing accounts are accepted only when the supplied password
// matches, so a redeploy can never silently replace credentials.
func BootstrapAccount(c config.Config, username, password, email string, gmLevel, realmID int) error {
	username = strings.ToUpper(strings.TrimSpace(username))
	email = strings.ToUpper(strings.TrimSpace(email))
	if err := srp.Validate(username, password); err != nil {
		return err
	}
	if len(email) > 255 || !strings.Contains(email, "@") {
		return fmt.Errorf("BOOTSTRAP_EMAIL must be a valid email address")
	}
	if gmLevel < 0 || gmLevel > 3 || realmID < -1 {
		return fmt.Errorf("invalid bootstrap GM level or realm ID")
	}

	db, err := sql.Open("mysql", c.AuthDSN+dsnOptions(c.AuthDSN))
	if err != nil {
		return err
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err = db.PingContext(ctx); err != nil {
		return fmt.Errorf("auth database: %w", err)
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var accountID uint32
	var salt, verifier []byte
	query := fmt.Sprintf("SELECT id,salt,verifier FROM `%s`.account WHERE username=? FOR UPDATE", c.AuthDB)
	err = tx.QueryRowContext(ctx, query, username).Scan(&accountID, &salt, &verifier)
	switch {
	case err == nil:
		if !srp.Verify(username, password, salt, verifier) {
			return fmt.Errorf("bootstrap account %s already exists with a different password", username)
		}
	case errors.Is(err, sql.ErrNoRows):
		salt, verifier, err = srp.Registration(username, password)
		if err != nil {
			return err
		}
		insert := fmt.Sprintf("INSERT INTO `%s`.account(username,salt,verifier,email,reg_mail,expansion) VALUES(?,?,?,?,?,?)", c.AuthDB)
		result, insertErr := tx.ExecContext(ctx, insert, username, salt, verifier, email, email, c.Expansion)
		if insertErr != nil {
			return fmt.Errorf("create bootstrap account: %w", insertErr)
		}
		id, idErr := result.LastInsertId()
		if idErr != nil {
			return idErr
		}
		accountID = uint32(id)
	case err != nil:
		return fmt.Errorf("inspect bootstrap account: %w", err)
	}

	worlds := fmt.Sprintf("INSERT IGNORE INTO `%s`.realmcharacters(realmid,acctid,numchars) SELECT id,?,0 FROM `%s`.realmlist", c.AuthDB, c.AuthDB)
	if _, err = tx.ExecContext(ctx, worlds, accountID); err != nil {
		return fmt.Errorf("initialize bootstrap realms: %w", err)
	}
	if gmLevel > 0 {
		access := fmt.Sprintf(`INSERT INTO %s.account_access(id,gmlevel,RealmID) VALUES(?,?,?)
			ON DUPLICATE KEY UPDATE gmlevel=GREATEST(gmlevel,VALUES(gmlevel))`, c.AuthDB)
		if _, err = tx.ExecContext(ctx, access, accountID, gmLevel, realmID); err != nil {
			return fmt.Errorf("grant bootstrap account access: %w", err)
		}
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	return nil
}

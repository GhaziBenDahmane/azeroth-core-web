package web

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type creditPackage struct {
	ID         uint32 `json:"id"`
	Slug       string `json:"slug"`
	Name       string `json:"name"`
	Price      string `json:"stripePriceId,omitempty"`
	Credits    uint32 `json:"credits"`
	BonusLabel string `json:"bonusLabel"`
	Active     bool   `json:"active"`
	SortOrder  int    `json:"sortOrder"`
}

func (s *Server) creditPackages() map[string]creditPackage {
	return map[string]creditPackage{"small": {Slug: "small", Name: "100 credits", Price: s.c.StripePriceSmall, Credits: 100, Active: true}, "medium": {Slug: "medium", Name: "550 credits", Price: s.c.StripePriceMedium, Credits: 550, Active: true}, "large": {Slug: "large", Name: "1,200 credits", Price: s.c.StripePriceLarge, Credits: 1200, Active: true}}
}

func (s *Server) availableCreditPackages(r *http.Request, includeInactive bool) []creditPackage {
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		if len(s.mock.creditPackages) > 0 {
			out := []creditPackage{}
			for _, pack := range s.mock.creditPackages {
				if includeInactive || pack.Active {
					out = append(out, pack)
				}
			}
			return out
		}
	}
	if !s.c.MockMode {
		query := "SELECT id,slug,name,stripe_price_id,credits,bonus_label,active,sort_order FROM portal_credit_packages WHERE realm_key=?"
		if !includeInactive {
			query += " AND active=1"
		}
		query += " ORDER BY sort_order,id"
		if rows, err := s.s.Auth.QueryContext(r.Context(), query, s.c.RealmKey); err == nil {
			defer rows.Close()
			packages := []creditPackage{}
			for rows.Next() {
				var pack creditPackage
				if rows.Scan(&pack.ID, &pack.Slug, &pack.Name, &pack.Price, &pack.Credits, &pack.BonusLabel, &pack.Active, &pack.SortOrder) == nil {
					packages = append(packages, pack)
				}
			}
			if len(packages) > 0 {
				return packages
			}
		}
	}
	packages := []creditPackage{}
	legacy := s.creditPackages()
	for _, key := range []string{"small", "medium", "large"} {
		pack := legacy[key]
		if pack.Price != "" || s.c.MockMode {
			packages = append(packages, pack)
		}
	}
	return packages
}

func (s *Server) billingPackages(w http.ResponseWriter, r *http.Request) {
	packages := s.availableCreditPackages(r, false)
	for i := range packages {
		packages[i].Price = ""
	}
	jsonOut(w, 200, map[string]any{"packages": packages})
}

func (s *Server) adminCreditPackages(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	if r.Method == http.MethodGet {
		jsonOut(w, 200, map[string]any{"packages": s.availableCreditPackages(r, true)})
		return
	}
	var pack creditPackage
	if !decode(w, r, &pack) {
		return
	}
	pack.Slug = articleSlug(pack.Slug)
	pack.Name = strings.TrimSpace(pack.Name)
	pack.Price = strings.TrimSpace(pack.Price)
	if pack.Slug == "" || len(pack.Slug) > 50 || len(pack.Name) < 2 || len(pack.Name) > 100 || pack.Credits == 0 || pack.Credits > 1000000 || len(pack.Price) > 255 || len(pack.BonusLabel) > 100 {
		problem(w, 422, "Invalid credit package")
		return
	}
	pack.Active = true
	if s.c.MockMode {
		s.mock.mu.Lock()
		pack.ID = uint32(len(s.mock.creditPackages) + 1)
		s.mock.creditPackages = append(s.mock.creditPackages, pack)
		s.mock.mu.Unlock()
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_credit_packages(realm_key,slug,name,credits,stripe_price_id,bonus_label,active,sort_order) VALUES(?,?,?,?,?,?,?,?)`, s.c.RealmKey, pack.Slug, pack.Name, pack.Credits, pack.Price, pack.BonusLabel, pack.Active, pack.SortOrder)
		if err != nil {
			problem(w, 409, "Package slug already exists or could not be saved")
			return
		}
		id, _ := res.LastInsertId()
		pack.ID = uint32(id)
		_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'credit_package.create',?,?)", a.ID, pack.Slug, pack.Name)
	}
	jsonOut(w, 201, pack)
}

func (s *Server) adminCreditPackageDelete(w http.ResponseWriter, r *http.Request) {
	a, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "Commerce permission required")
		return
	}
	id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
	if err != nil {
		problem(w, 400, "Invalid credit package")
		return
	}
	if s.c.MockMode {
		s.mock.mu.Lock()
		defer s.mock.mu.Unlock()
		for i := range s.mock.creditPackages {
			if s.mock.creditPackages[i].ID == uint32(id) {
				s.mock.creditPackages[i].Active = false
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
	} else {
		res, err := s.s.Auth.ExecContext(r.Context(), "UPDATE portal_credit_packages SET active=0 WHERE id=? AND realm_key=?", id, s.c.RealmKey)
		if err == nil {
			if changed, _ := res.RowsAffected(); changed > 0 {
				_, _ = s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target) VALUES(?,'credit_package.archive',?)", a.ID, strconv.FormatUint(id, 10))
				jsonOut(w, 200, map[string]bool{"ok": true})
				return
			}
		}
	}
	problem(w, 404, "Credit package not found")
}

func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Package, RecipientUsername, Message string
	}
	if !decode(w, r, &in) {
		return
	}
	var pack creditPackage
	found := false
	for _, candidate := range s.availableCreditPackages(r, false) {
		if candidate.Slug == in.Package {
			pack = candidate
			found = true
			break
		}
	}
	if !found || pack.Price == "" || s.c.StripeSecret == "" {
		problem(w, 503, "Credit purchases are not configured")
		return
	}
	targetID := a.ID
	recipient := strings.ToUpper(strings.TrimSpace(in.RecipientUsername))
	if len(in.Message) > 500 {
		problem(w, 422, "Gift message is too long")
		return
	}
	if recipient != "" && recipient != strings.ToUpper(a.Username) {
		if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT id FROM account WHERE username=?", recipient).Scan(&targetID); err != nil {
			problem(w, 422, "Gift recipient was not found")
			return
		}
	}
	values := url.Values{"mode": {"payment"}, "success_url": {s.c.PublicURL + "/account/orders?payment=success"}, "cancel_url": {s.c.PublicURL + "/shop?payment=cancelled"}, "client_reference_id": {strconv.FormatUint(uint64(a.ID), 10)}, "metadata[account_id]": {strconv.FormatUint(uint64(targetID), 10)}, "metadata[purchaser_account_id]": {strconv.FormatUint(uint64(a.ID), 10)}, "metadata[credits]": {strconv.FormatUint(uint64(pack.Credits), 10)}, "metadata[realm_key]": {s.c.RealmKey}, "metadata[gift_message]": {strings.TrimSpace(in.Message)}, "line_items[0][price]": {pack.Price}, "line_items[0][quantity]": {"1"}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.stripe.com/v1/checkout/sessions", strings.NewReader(values.Encode()))
	if err != nil {
		problem(w, 500, "Could not create checkout")
		return
	}
	req.SetBasicAuth(s.c.StripeSecret, "")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := s.soap.HTTP.Do(req)
	if err != nil {
		problem(w, 502, "Payment provider unavailable")
		return
	}
	defer resp.Body.Close()
	var out struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out) != nil || resp.StatusCode/100 != 2 || out.URL == "" {
		problem(w, 502, "Payment provider rejected checkout")
		return
	}
	jsonOut(w, 200, map[string]string{"url": out.URL})
}

func (s *Server) billingWebhook(w http.ResponseWriter, r *http.Request) {
	if s.c.StripeWebhookSecret == "" {
		problem(w, 503, "Payment webhook is not configured")
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if err != nil {
		problem(w, 400, "Invalid webhook")
		return
	}
	if !verifyStripeSignature(body, r.Header.Get("Stripe-Signature"), s.c.StripeWebhookSecret, time.Now()) {
		problem(w, 401, "Invalid webhook signature")
		return
	}
	var event struct {
		ID   string `json:"id"`
		Type string `json:"type"`
		Data struct {
			Object struct {
				ID             string            `json:"id"`
				PaymentStatus  string            `json:"payment_status"`
				PaymentIntent  string            `json:"payment_intent"`
				Status         string            `json:"status"`
				Currency       string            `json:"currency"`
				ReceiptURL     string            `json:"receipt_url"`
				AmountTotal    uint64            `json:"amount_total"`
				AmountRefunded uint64            `json:"amount_refunded"`
				Metadata       map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &event) != nil || event.ID == "" {
		problem(w, 400, "Invalid webhook payload")
		return
	}
	payloadHash := sha256.Sum256(body)
	webhookResult, err := s.s.Auth.ExecContext(r.Context(), `INSERT IGNORE INTO portal_payment_webhooks(event_id,event_type,object_id,payload_sha256) VALUES(?,?,?,?)`, event.ID, event.Type, event.Data.Object.ID, payloadHash[:])
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not record payment event")
		return
	}
	insertedWebhook, _ := webhookResult.RowsAffected()
	if insertedWebhook == 0 {
		var processed bool
		if s.s.Auth.QueryRowContext(r.Context(), `SELECT processed FROM portal_payment_webhooks WHERE event_id=?`, event.ID).Scan(&processed) == nil && processed {
			jsonOut(w, http.StatusOK, map[string]bool{"received": true})
			return
		}
	}
	if event.Type == "charge.succeeded" {
		_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_payment_transactions SET receipt_url=?,currency=CASE WHEN currency='' THEN ? ELSE currency END WHERE payment_intent=?`, event.Data.Object.ReceiptURL, event.Data.Object.Currency, event.Data.Object.PaymentIntent)
		_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_payment_webhooks SET processed=1,processed_at=NOW() WHERE event_id=?`, event.ID)
		jsonOut(w, http.StatusOK, map[string]bool{"received": true})
		return
	}
	if event.Type == "charge.refunded" || event.Type == "charge.dispute.created" || event.Type == "charge.dispute.closed" {
		if err := s.applyStripeReversal(r, event.ID, event.Type, event.Data.Object.ID, event.Data.Object.PaymentIntent, event.Data.Object.Status, event.Data.Object.AmountRefunded); err != nil {
			_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_payment_webhooks SET error_message=? WHERE event_id=?`, truncate(err.Error(), 500), event.ID)
			problem(w, http.StatusConflict, err.Error())
			return
		}
		jsonOut(w, http.StatusOK, map[string]bool{"received": true})
		return
	}
	if event.Type != "checkout.session.completed" || event.Data.Object.PaymentStatus != "paid" {
		_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_payment_webhooks SET processed=1,processed_at=NOW() WHERE event_id=?`, event.ID)
		jsonOut(w, 200, map[string]bool{"received": true})
		return
	}
	accountID, e1 := strconv.ParseUint(event.Data.Object.Metadata["account_id"], 10, 32)
	credits, e2 := strconv.ParseUint(event.Data.Object.Metadata["credits"], 10, 32)
	purchaserID, e3 := strconv.ParseUint(event.Data.Object.Metadata["purchaser_account_id"], 10, 32)
	if e3 != nil {
		purchaserID = accountID
	}
	if e1 != nil || e2 != nil || credits == 0 || credits > 1000000 {
		problem(w, 422, "Invalid checkout metadata")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_payment_events(event_id,checkout_id,account_id,credits) VALUES(?,?,?,?)", event.ID, event.Data.Object.ID, accountID, credits)
	if err != nil {
		problem(w, 500, "Could not record payment")
		return
	}
	inserted, _ := res.RowsAffected()
	if inserted == 1 {
		if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)", accountID, credits); err == nil {
			_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)", accountID, credits, "Stripe checkout "+event.Data.Object.ID)
		}
		if err == nil && purchaserID != accountID {
			_, err = tx.ExecContext(r.Context(), "INSERT IGNORE INTO portal_gifts(purchaser_account_id,recipient_account_id,realm_key,checkout_id,credits,message) VALUES(?,?,?,?,?,?)", purchaserID, accountID, event.Data.Object.Metadata["realm_key"], event.Data.Object.ID, credits, event.Data.Object.Metadata["gift_message"])
		}
		if err == nil {
			_, err = tx.ExecContext(r.Context(), `INSERT INTO portal_payment_transactions(checkout_id,payment_intent,realm_key,purchaser_account_id,recipient_account_id,credits,amount_total,currency,status) VALUES(?,?,?,?,?,?,?,?, 'paid') ON DUPLICATE KEY UPDATE payment_intent=VALUES(payment_intent),amount_total=VALUES(amount_total),currency=VALUES(currency),status='paid'`, event.Data.Object.ID, event.Data.Object.PaymentIntent, event.Data.Object.Metadata["realm_key"], purchaserID, accountID, credits, event.Data.Object.AmountTotal, event.Data.Object.Currency)
		}
	}
	if err != nil {
		problem(w, 500, "Could not apply payment")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not commit payment")
		return
	}
	if inserted == 1 && purchaserID != accountID {
		s.notifyAccount(r.Context(), uint32(accountID), "gift", "You received portal credits", fmt.Sprintf("%d credits were gifted to your account.", credits), "/account/orders")
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_payment_webhooks SET processed=1,processed_at=NOW() WHERE event_id=?`, event.ID)
	jsonOut(w, 200, map[string]bool{"received": true})
}

func truncate(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}
	return value[:maximum]
}

func (s *Server) applyStripeReversal(r *http.Request, eventID, eventType, objectID, paymentIntent, disputeStatus string, amountRefunded uint64) error {
	if paymentIntent == "" {
		return fmt.Errorf("payment event has no payment intent")
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var accountID, credits, refunded uint32
	var amountTotal uint64
	var status string
	if err = tx.QueryRowContext(r.Context(), `SELECT recipient_account_id,credits,refunded_credits,amount_total,status FROM portal_payment_transactions WHERE payment_intent=? FOR UPDATE`, paymentIntent).Scan(&accountID, &credits, &refunded, &amountTotal, &status); err != nil {
		return fmt.Errorf("payment transaction not found")
	}
	nextStatus := status
	desiredReversal := uint32(0)
	if eventType == "charge.refunded" {
		desiredReversal = credits
		if amountRefunded > 0 && amountTotal > 0 && amountRefunded < amountTotal {
			desiredReversal = uint32((uint64(credits)*amountRefunded + amountTotal - 1) / amountTotal)
		}
	} else if eventType == "charge.dispute.created" || eventType == "charge.dispute.closed" && disputeStatus == "lost" {
		desiredReversal = credits
	}
	if eventType == "charge.dispute.created" {
		nextStatus = "disputed"
	} else if eventType == "charge.dispute.closed" && disputeStatus == "won" {
		nextStatus = "paid"
	} else if eventType == "charge.refunded" {
		nextStatus = "partially_refunded"
		if desiredReversal == credits {
			nextStatus = "refunded"
		}
	} else if eventType == "charge.dispute.closed" {
		nextStatus = "chargeback"
	}
	if eventType == "charge.dispute.closed" && disputeStatus == "won" && refunded > 0 && status == "disputed" {
		if _, err = tx.ExecContext(r.Context(), `UPDATE portal_wallets SET balance=balance+? WHERE account_id=?`, refunded, accountID); err != nil {
			return err
		}
		if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)`, accountID, refunded, "Stripe dispute won "+objectID); err != nil {
			return err
		}
		refunded = 0
	} else if desiredReversal > refunded {
		delta := desiredReversal - refunded
		result, updateErr := tx.ExecContext(r.Context(), `UPDATE portal_wallets SET balance=balance-? WHERE account_id=? AND balance>=?`, delta, accountID, delta)
		if updateErr != nil {
			return updateErr
		}
		changed, _ := result.RowsAffected()
		if changed == 0 {
			nextStatus = "reversal_review"
		} else {
			if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,?,?)`, accountID, -int32(delta), "Stripe reversal "+objectID); err != nil {
				return err
			}
			refunded = desiredReversal
		}
	}
	_, err = tx.ExecContext(r.Context(), `UPDATE portal_payment_transactions SET status=?,refunded_credits=?,dispute_id=CASE WHEN ? LIKE 'charge.dispute.%' THEN ? ELSE dispute_id END WHERE payment_intent=?`, nextStatus, refunded, eventType, objectID, paymentIntent)
	if err != nil {
		return err
	}
	if _, err = tx.ExecContext(r.Context(), `UPDATE portal_payment_webhooks SET processed=1,processed_at=NOW() WHERE event_id=?`, eventID); err != nil {
		return err
	}
	return tx.Commit()
}

func verifyStripeSignature(payload []byte, header, secret string, now time.Time) bool {
	var timestamp int64
	signatures := []string{}
	for _, part := range strings.Split(header, ",") {
		key, value, ok := strings.Cut(strings.TrimSpace(part), "=")
		if !ok {
			continue
		}
		if key == "t" {
			timestamp, _ = strconv.ParseInt(value, 10, 64)
		} else if key == "v1" {
			signatures = append(signatures, value)
		}
	}
	if timestamp == 0 || now.Sub(time.Unix(timestamp, 0)) > 5*time.Minute || time.Unix(timestamp, 0).Sub(now) > 5*time.Minute {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "%d.", timestamp)
	_, _ = mac.Write(payload)
	expected := mac.Sum(nil)
	for _, signature := range signatures {
		got, err := hex.DecodeString(signature)
		if err == nil && hmac.Equal(got, expected) {
			return true
		}
	}
	return false
}

type paymentTransaction struct {
	CheckoutID      string    `json:"checkoutId"`
	PaymentIntent   string    `json:"paymentIntent,omitempty"`
	RealmKey        string    `json:"realmKey"`
	Purchaser       string    `json:"purchaser"`
	Recipient       string    `json:"recipient"`
	Credits         uint32    `json:"credits"`
	RefundedCredits uint32    `json:"refundedCredits"`
	AmountTotal     uint64    `json:"amountTotal"`
	Currency        string    `json:"currency"`
	Status          string    `json:"status"`
	ReceiptURL      string    `json:"receiptUrl,omitempty"`
	DisputeID       string    `json:"disputeId,omitempty"`
	CreatedAt       time.Time `json:"createdAt"`
	UpdatedAt       time.Time `json:"updatedAt"`
}

func (s *Server) paymentTransactions(w http.ResponseWriter, r *http.Request) {
	a, err := s.trackerAccount(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusOK, map[string]any{"payments": []map[string]any{{"checkoutId": "cs_demo", "credits": 550, "amountTotal": 1999, "currency": "eur", "status": "paid", "receiptUrl": "https://example.com/receipt", "createdAt": time.Now().Add(-24 * time.Hour)}}})
		return
	}
	query := fmt.Sprintf(`SELECT p.checkout_id,p.payment_intent,p.realm_key,COALESCE(b.username,''),COALESCE(recipient.username,''),p.credits,p.refunded_credits,p.amount_total,p.currency,p.status,p.receipt_url,p.dispute_id,p.created_at,p.updated_at FROM portal_payment_transactions p LEFT JOIN %s.account b ON b.id=p.purchaser_account_id LEFT JOIN %s.account recipient ON recipient.id=p.recipient_account_id WHERE p.purchaser_account_id=? OR p.recipient_account_id=? ORDER BY p.created_at DESC LIMIT 200`, s.c.AuthDB, s.c.AuthDB)
	rows, err := s.s.Auth.QueryContext(r.Context(), query, a.ID, a.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load payment history")
		return
	}
	defer rows.Close()
	items := []paymentTransaction{}
	for rows.Next() {
		var item paymentTransaction
		if rows.Scan(&item.CheckoutID, &item.PaymentIntent, &item.RealmKey, &item.Purchaser, &item.Recipient, &item.Credits, &item.RefundedCredits, &item.AmountTotal, &item.Currency, &item.Status, &item.ReceiptURL, &item.DisputeID, &item.CreatedAt, &item.UpdatedAt) == nil {
			item.PaymentIntent = ""
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"payments": items})
}

func (s *Server) adminPayments(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "commerce"); !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	page, perPage, offset := requestPage(r, 25, 100)
	search, status := strings.TrimSpace(r.URL.Query().Get("q")), strings.TrimSpace(r.URL.Query().Get("status"))
	if len(search) > 100 || status != "" && !map[string]bool{"pending": true, "paid": true, "partially_refunded": true, "refunded": true, "disputed": true, "reversal_review": true}[status] {
		problem(w, http.StatusUnprocessableEntity, "Invalid payment filters")
		return
	}
	if s.c.MockMode {
		items := []map[string]any{{"checkoutId": "cs_demo", "paymentIntent": "pi_demo", "purchaser": "DEMO", "recipient": "DEMO", "credits": 550, "amountTotal": 1999, "currency": "eur", "status": "paid", "createdAt": time.Now().Add(-24 * time.Hour)}}
		if status != "" && status != "paid" || search != "" && !strings.Contains(strings.ToLower("cs_demo pi_demo demo"), strings.ToLower(search)) {
			items = nil
		}
		items, meta := slicePage(items, page, perPage)
		jsonOut(w, http.StatusOK, map[string]any{"payments": items, "pagination": meta})
		return
	}
	where, args := " WHERE p.realm_key=?", []any{s.c.RealmKey}
	if status != "" {
		where += " AND p.status=?"
		args = append(args, status)
	}
	if search != "" {
		where += " AND (p.checkout_id LIKE ? OR p.payment_intent LIKE ? OR b.username LIKE ? OR recipient.username LIKE ?)"
		pattern := likePattern(search)
		args = append(args, pattern, pattern, pattern, pattern)
	}
	var total int
	countQuery := fmt.Sprintf(`SELECT COUNT(*) FROM portal_payment_transactions p LEFT JOIN %s.account b ON b.id=p.purchaser_account_id LEFT JOIN %s.account recipient ON recipient.id=p.recipient_account_id`, s.c.AuthDB, s.c.AuthDB) + where
	if err := s.s.Auth.QueryRowContext(r.Context(), countQuery, args...).Scan(&total); err != nil {
		problem(w, http.StatusInternalServerError, "Could not count payments")
		return
	}
	meta := paginationMeta(page, perPage, total)
	offset = (meta.Page - 1) * perPage
	query := fmt.Sprintf(`SELECT p.checkout_id,p.payment_intent,p.realm_key,COALESCE(b.username,''),COALESCE(recipient.username,''),p.credits,p.refunded_credits,p.amount_total,p.currency,p.status,p.receipt_url,p.dispute_id,p.created_at,p.updated_at FROM portal_payment_transactions p LEFT JOIN %s.account b ON b.id=p.purchaser_account_id LEFT JOIN %s.account recipient ON recipient.id=p.recipient_account_id`, s.c.AuthDB, s.c.AuthDB) + where + " ORDER BY p.created_at DESC LIMIT ? OFFSET ?"
	rows, err := s.s.Auth.QueryContext(r.Context(), query, append(args, perPage, offset)...)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load payments")
		return
	}
	defer rows.Close()
	items := []paymentTransaction{}
	for rows.Next() {
		var item paymentTransaction
		if rows.Scan(&item.CheckoutID, &item.PaymentIntent, &item.RealmKey, &item.Purchaser, &item.Recipient, &item.Credits, &item.RefundedCredits, &item.AmountTotal, &item.Currency, &item.Status, &item.ReceiptURL, &item.DisputeID, &item.CreatedAt, &item.UpdatedAt) == nil {
			items = append(items, item)
		}
	}
	jsonOut(w, http.StatusOK, map[string]any{"payments": items, "pagination": meta})
}

func (s *Server) adminPaymentRefund(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, http.StatusForbidden, "Commerce permission required")
		return
	}
	checkoutID := strings.TrimSpace(r.PathValue("id"))
	if checkoutID == "" || len(checkoutID) > 255 {
		problem(w, http.StatusBadRequest, "Invalid payment")
		return
	}
	if s.c.MockMode {
		jsonOut(w, http.StatusAccepted, map[string]bool{"ok": true})
		return
	}
	if strings.TrimSpace(s.c.StripeSecret) == "" {
		problem(w, http.StatusServiceUnavailable, "Payment refunds are not configured")
		return
	}
	var paymentIntent, status string
	if s.s.Auth.QueryRowContext(r.Context(), `SELECT payment_intent,status FROM portal_payment_transactions WHERE checkout_id=? AND realm_key=?`, checkoutID, s.c.RealmKey).Scan(&paymentIntent, &status) != nil {
		problem(w, http.StatusNotFound, "Payment not found")
		return
	}
	if status != "paid" || paymentIntent == "" {
		problem(w, http.StatusConflict, "Only a settled, unreversed payment can be refunded")
		return
	}
	request, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://api.stripe.com/v1/refunds", strings.NewReader(url.Values{"payment_intent": {paymentIntent}}.Encode()))
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not prepare refund")
		return
	}
	request.SetBasicAuth(s.c.StripeSecret, "")
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response, err := s.soap.HTTP.Do(request)
	if err != nil {
		problem(w, http.StatusBadGateway, "Payment provider unavailable")
		return
	}
	defer response.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if response.StatusCode/100 != 2 {
		problem(w, http.StatusBadGateway, "Payment provider rejected refund")
		return
	}
	var refund struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(body, &refund) != nil || refund.ID == "" {
		problem(w, http.StatusBadGateway, "Invalid refund response")
		return
	}
	_, _ = s.s.Auth.ExecContext(r.Context(), `UPDATE portal_payment_transactions SET status='refund_pending' WHERE checkout_id=? AND status='paid'`, checkoutID)
	_, _ = s.s.Auth.ExecContext(r.Context(), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'payment.refund_requested',?,?)`, actor.ID, checkoutID, "refund="+refund.ID)
	jsonOut(w, http.StatusAccepted, map[string]any{"ok": true, "refundId": refund.ID})
}

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
	Price   string
	Credits uint32
}

func (s *Server) creditPackages() map[string]creditPackage {
	return map[string]creditPackage{"small": {s.c.StripePriceSmall, 100}, "medium": {s.c.StripePriceMedium, 550}, "large": {s.c.StripePriceLarge, 1200}}
}

func (s *Server) billingCheckout(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		Package string `json:"package"`
	}
	if !decode(w, r, &in) {
		return
	}
	pack, ok := s.creditPackages()[in.Package]
	if !ok || pack.Price == "" || s.c.StripeSecret == "" {
		problem(w, 503, "Credit purchases are not configured")
		return
	}
	values := url.Values{"mode": {"payment"}, "success_url": {s.c.PublicURL + "/account?payment=success"}, "cancel_url": {s.c.PublicURL + "/account?payment=cancelled"}, "client_reference_id": {strconv.FormatUint(uint64(a.ID), 10)}, "metadata[account_id]": {strconv.FormatUint(uint64(a.ID), 10)}, "metadata[credits]": {strconv.FormatUint(uint64(pack.Credits), 10)}, "line_items[0][price]": {pack.Price}, "line_items[0][quantity]": {"1"}}
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
				ID            string            `json:"id"`
				PaymentStatus string            `json:"payment_status"`
				Metadata      map[string]string `json:"metadata"`
			} `json:"object"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &event) != nil || event.ID == "" {
		problem(w, 400, "Invalid webhook payload")
		return
	}
	if event.Type != "checkout.session.completed" || event.Data.Object.PaymentStatus != "paid" {
		jsonOut(w, 200, map[string]bool{"received": true})
		return
	}
	accountID, e1 := strconv.ParseUint(event.Data.Object.Metadata["account_id"], 10, 32)
	credits, e2 := strconv.ParseUint(event.Data.Object.Metadata["credits"], 10, 32)
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
	}
	if err != nil {
		problem(w, 500, "Could not apply payment")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not commit payment")
		return
	}
	jsonOut(w, 200, map[string]bool{"received": true})
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

package web

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/store"
)

func TestStripeWebhookLedgerMatrix(t *testing.T) {
	base := os.Getenv("PORTAL_TEST_MYSQL_DSN")
	if base == "" {
		t.Skip("PORTAL_TEST_MYSQL_DSN is not set")
	}
	admin, err := sql.Open("mysql", base+"?parseTime=true")
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	suffix := fmt.Sprintf("_%d", time.Now().UnixNano())
	names := []string{"portal_billing_auth" + suffix, "portal_billing_characters" + suffix, "portal_billing_world" + suffix}
	for _, name := range names {
		if _, err = admin.ExecContext(ctx, "CREATE DATABASE `"+name+"` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci"); err != nil {
			t.Fatal(err)
		}
		name := name
		t.Cleanup(func() { _, _ = admin.Exec("DROP DATABASE `" + name + "`") })
	}
	c := config.Config{
		AuthDSN: base + names[0], CharactersDSN: base + names[1], WorldDSN: base + names[2],
		AuthDB: names[0], CharactersDB: names[1], WorldDB: names[2], RealmKey: "integration", DefaultRealmKey: "integration",
		StripeWebhookSecret: "whsec_integration", CompetitiveIngestSecret: "competitive-integration-secret",
	}
	database, err := store.ConnectForMigration(c)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err = database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	s := &Server{s: database, c: c}

	post := func(payload string) *httptest.ResponseRecorder {
		t.Helper()
		timestamp := time.Now().Unix()
		mac := hmac.New(sha256.New, []byte(c.StripeWebhookSecret))
		_, _ = fmt.Fprintf(mac, "%d.%s", timestamp, payload)
		r := httptest.NewRequest(http.MethodPost, "/api/billing/webhook", strings.NewReader(payload))
		r.Header.Set("Stripe-Signature", fmt.Sprintf("t=%d,v1=%s", timestamp, hex.EncodeToString(mac.Sum(nil))))
		w := httptest.NewRecorder()
		s.billingWebhook(w, r)
		return w
	}
	checkout := `{"id":"evt_checkout","type":"checkout.session.completed","data":{"object":{"id":"cs_test","payment_status":"paid","payment_intent":"pi_checkout","amount_total":1999,"currency":"eur","metadata":{"account_id":"3","purchaser_account_id":"3","credits":"100","realm_key":"integration"}}}}`
	for i := 0; i < 2; i++ {
		if w := post(checkout); w.Code != http.StatusOK {
			t.Fatalf("checkout %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	assertWalletBalance(t, database.Auth, 3, 100)

	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO portal_wallets(account_id,balance) VALUES(1,100),(2,100)`); err != nil {
		t.Fatal(err)
	}
	if _, err = database.Auth.ExecContext(ctx, `INSERT INTO portal_payment_transactions(checkout_id,payment_intent,realm_key,purchaser_account_id,recipient_account_id,credits,amount_total,currency,status) VALUES
		('cs_refund','pi_refund','integration',1,1,100,1000,'eur','paid'),
		('cs_dispute','pi_dispute','integration',1,1,50,1000,'eur','paid'),
		('cs_review','pi_review','integration',2,2,200,1000,'eur','paid')`); err != nil {
		t.Fatal(err)
	}
	partial := `{"id":"evt_partial","type":"charge.refunded","data":{"object":{"id":"ch_refund","payment_intent":"pi_refund","amount_refunded":250}}}`
	if w := post(partial); w.Code != http.StatusOK {
		t.Fatalf("partial refund: %d %s", w.Code, w.Body.String())
	}
	if w := post(partial); w.Code != http.StatusOK {
		t.Fatalf("duplicate partial refund: %d %s", w.Code, w.Body.String())
	}
	assertPaymentState(t, database.Auth, "pi_refund", "partially_refunded", 25)
	assertWalletBalance(t, database.Auth, 1, 75)
	full := `{"id":"evt_full","type":"charge.refunded","data":{"object":{"id":"ch_refund","payment_intent":"pi_refund","amount_refunded":1000}}}`
	if w := post(full); w.Code != http.StatusOK {
		t.Fatalf("full refund: %d %s", w.Code, w.Body.String())
	}
	assertPaymentState(t, database.Auth, "pi_refund", "refunded", 100)
	assertWalletBalance(t, database.Auth, 1, 0)

	if _, err = database.Auth.ExecContext(ctx, `UPDATE portal_wallets SET balance=100 WHERE account_id=1`); err != nil {
		t.Fatal(err)
	}
	dispute := `{"id":"evt_dispute","type":"charge.dispute.created","data":{"object":{"id":"dp_test","payment_intent":"pi_dispute"}}}`
	if w := post(dispute); w.Code != http.StatusOK {
		t.Fatalf("dispute: %d %s", w.Code, w.Body.String())
	}
	assertPaymentState(t, database.Auth, "pi_dispute", "disputed", 50)
	assertWalletBalance(t, database.Auth, 1, 50)
	won := `{"id":"evt_dispute_won","type":"charge.dispute.closed","data":{"object":{"id":"dp_test","payment_intent":"pi_dispute","status":"won"}}}`
	for i := 0; i < 2; i++ {
		if w := post(won); w.Code != http.StatusOK {
			t.Fatalf("won dispute %d: %d %s", i, w.Code, w.Body.String())
		}
	}
	assertPaymentState(t, database.Auth, "pi_dispute", "paid", 0)
	assertWalletBalance(t, database.Auth, 1, 100)

	review := `{"id":"evt_review","type":"charge.dispute.created","data":{"object":{"id":"dp_review","payment_intent":"pi_review"}}}`
	if w := post(review); w.Code != http.StatusOK {
		t.Fatalf("reversal review: %d %s", w.Code, w.Body.String())
	}
	assertPaymentState(t, database.Auth, "pi_review", "reversal_review", 0)
	assertWalletBalance(t, database.Auth, 2, 100)

	battleground := `{"eventId":"bg-event-1","battleground":"Warsong Gulch","winningTeam":"alliance","durationSeconds":912,"playedAt":"2026-08-29T20:00:00Z","members":[{"guid":1,"name":"Arthoria","team":"alliance","class":2,"killingBlows":5,"honorableKills":18,"deaths":1,"damageDone":250000,"healingDone":42000}]}`
	for index := 0; index < 2; index++ {
		r := httptest.NewRequest(http.MethodPost, "/api/integrations/battlegrounds", strings.NewReader(battleground))
		r.Header.Set("Authorization", "Bearer "+c.CompetitiveIngestSecret)
		w := httptest.NewRecorder()
		s.ingestBattlegroundMatch(w, r)
		if index == 0 && w.Code != http.StatusCreated || index == 1 && (w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"duplicate":true`)) {
			t.Fatalf("battleground ingest %d: %d %s", index, w.Code, w.Body.String())
		}
	}
	var battlegroundCount, memberCount int
	if err = database.Auth.QueryRow(`SELECT COUNT(*) FROM portal_battleground_matches WHERE source_event_id='bg-event-1'`).Scan(&battlegroundCount); err != nil {
		t.Fatal(err)
	}
	if err = database.Auth.QueryRow(`SELECT COUNT(*) FROM portal_battleground_members`).Scan(&memberCount); err != nil || battlegroundCount != 1 || memberCount != 1 {
		t.Fatalf("battleground ledger counts = %d/%d, err=%v", battlegroundCount, memberCount, err)
	}

	pvp := fmt.Sprintf(`{"eventId":"arena-event-1","season":"season-8","bracket":2,"teamId":18,"teamName":"Dawnbringers","opponentId":27,"opponentName":"Frozen Resolve","result":"win","ratingBefore":2148,"ratingAfter":2162,"ratingChange":14,"durationSeconds":184,"playedAt":%q,"members":[{"guid":1,"name":"Arthoria"}]}`, time.Now().Add(-time.Hour).UTC().Format(time.RFC3339))
	r := httptest.NewRequest(http.MethodPost, "/api/integrations/pvp", strings.NewReader(pvp))
	r.Header.Set("Authorization", "Bearer "+c.CompetitiveIngestSecret)
	w := httptest.NewRecorder()
	s.ingestPvPMatch(w, r)
	if w.Code != http.StatusCreated {
		t.Fatalf("PvP ingest: %d %s", w.Code, w.Body.String())
	}
	var season string
	var before, after uint16
	var change int16
	if err = database.Auth.QueryRow(`SELECT season_slug,rating_before,rating_after,rating_change FROM portal_pvp_matches WHERE source_event_id='arena-event-1'`).Scan(&season, &before, &after, &change); err != nil || season != "season-8" || before != 2148 || after != 2162 || change != 14 {
		t.Fatalf("PvP rating history = %s/%d/%d/%d, err=%v", season, before, after, change, err)
	}

	roster := `[{"guid":1,"name":"Arthoria","class":2,"role":"tank"},{"guid":2,"name":"Frostveil","class":8,"role":"damage"},{"guid":3,"name":"Dawnmend","class":5,"role":"healer"},{"guid":4,"name":"Oakshield","class":11,"role":"healer"},{"guid":5,"name":"Ironward","class":1,"role":"tank"},{"guid":6,"name":"Nightarrow","class":3,"role":"damage"},{"guid":7,"name":"Dusksong","class":4,"role":"damage"},{"guid":8,"name":"Stormcall","class":7,"role":"damage"}]`
	postRaid := func(eventID, result string, attempt int, health float64) *httptest.ResponseRecorder {
		payload := fmt.Sprintf(`{"eventId":%q,"guildId":1,"guildName":"Keepers of Dawn","raid":"Icecrown Citadel","boss":"The Lich King","difficulty":"10 player","result":%q,"attemptNumber":%d,"bossHealthPercent":%.2f,"durationSeconds":720,"occurredAt":%q,"members":%s}`, eventID, result, attempt, health, time.Now().Add(-30*time.Minute).UTC().Format(time.RFC3339), roster)
		req := httptest.NewRequest(http.MethodPost, "/api/integrations/raids", strings.NewReader(payload))
		req.Header.Set("Authorization", "Bearer "+c.CompetitiveIngestSecret)
		out := httptest.NewRecorder()
		s.ingestRaidKill(out, req)
		return out
	}
	if w = postRaid("raid-wipe-1", "wipe", 17, 8.4); w.Code != http.StatusCreated {
		t.Fatalf("raid wipe ingest: %d %s", w.Code, w.Body.String())
	}
	if w = postRaid("raid-wipe-1", "wipe", 17, 8.4); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"duplicate":true`) {
		t.Fatalf("duplicate raid wipe: %d %s", w.Code, w.Body.String())
	}
	if w = postRaid("raid-kill-1", "kill", 18, 0); w.Code != http.StatusCreated {
		t.Fatalf("raid kill ingest: %d %s", w.Code, w.Body.String())
	}
	var attemptCount, killCount, attemptMemberCount int
	_ = database.Auth.QueryRow(`SELECT COUNT(*) FROM portal_raid_attempts`).Scan(&attemptCount)
	_ = database.Auth.QueryRow(`SELECT COUNT(*) FROM portal_raid_kills`).Scan(&killCount)
	_ = database.Auth.QueryRow(`SELECT COUNT(*) FROM portal_raid_attempt_members`).Scan(&attemptMemberCount)
	if attemptCount != 2 || killCount != 1 || attemptMemberCount != 16 {
		t.Fatalf("raid history counts = attempts %d, kills %d, members %d", attemptCount, killCount, attemptMemberCount)
	}
}

func assertWalletBalance(t *testing.T, database *sql.DB, accountID, want uint32) {
	t.Helper()
	var got uint32
	if err := database.QueryRow(`SELECT balance FROM portal_wallets WHERE account_id=?`, accountID).Scan(&got); err != nil || got != want {
		t.Fatalf("account %d balance = %d, err=%v; want %d", accountID, got, err, want)
	}
}

func assertPaymentState(t *testing.T, database *sql.DB, paymentIntent, wantStatus string, wantRefunded uint32) {
	t.Helper()
	var status string
	var refunded uint32
	if err := database.QueryRow(`SELECT status,refunded_credits FROM portal_payment_transactions WHERE payment_intent=?`, paymentIntent).Scan(&status, &refunded); err != nil || status != wantStatus || refunded != wantRefunded {
		t.Fatalf("payment %s = %s/%d, err=%v; want %s/%d", paymentIntent, status, refunded, err, wantStatus, wantRefunded)
	}
}

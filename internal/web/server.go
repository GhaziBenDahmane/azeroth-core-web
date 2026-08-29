package web

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/example/azeroth-portal/internal/config"
	"github.com/example/azeroth-portal/internal/soap"
	"github.com/example/azeroth-portal/internal/srp"
	"github.com/example/azeroth-portal/internal/store"
)

type Server struct {
	s       *store.Store
	c       config.Config
	soap    *soap.Client
	static  fs.FS
	limiter *limiter
	mock    *mockState
	metrics portalMetrics
}
type portalMetrics struct {
	requests atomic.Uint64
	errors   atomic.Uint64
	orders   atomic.Uint64
}
type limiter struct {
	mu        sync.Mutex
	hits      map[string][]time.Time
	lastSweep time.Time
}

func New(s *store.Store, c config.Config, static fs.FS) *Server {
	server := &Server{s: s, c: c, soap: soap.New(c.SOAPURL, c.SOAPUser, c.SOAPPassword), static: static, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState()}
	if !c.MockMode && server.soap.Enabled() {
		go server.deliveryLoop()
	}
	return server
}

func (s *Server) Handler() http.Handler {
	if s.c.MockMode {
		return s.middleware(s.mockHandler())
	}
	m := http.NewServeMux()
	m.HandleFunc("GET /api/setup/status", s.setupStatus)
	m.HandleFunc("POST /api/setup", s.rate(5, time.Hour, s.setup))
	m.HandleFunc("GET /api/status", s.status)
	m.HandleFunc("POST /api/auth/register", s.feature(s.c.EnableRegistration, "Registration", s.rate(5, time.Hour, s.register)))
	m.HandleFunc("POST /api/auth/login", s.rate(10, 10*time.Minute, s.login))
	m.HandleFunc("POST /api/auth/logout", s.logout)
	m.HandleFunc("POST /api/auth/password/request", s.rate(5, time.Hour, s.passwordResetRequest))
	m.HandleFunc("POST /api/auth/password/reset", s.rate(10, time.Hour, s.passwordResetConfirm))
	m.HandleFunc("POST /api/auth/email/verify", s.rate(10, time.Hour, s.emailVerificationConfirm))
	m.HandleFunc("POST /api/auth/email/resend", s.rate(5, time.Hour, s.emailVerificationResend))
	m.HandleFunc("GET /api/public-config", s.publicConfig)
	m.HandleFunc("GET /api/me", s.me)
	m.HandleFunc("GET /api/characters", s.characters)
	m.HandleFunc("GET /api/armory", s.feature(s.c.EnableArmory, "Armory", s.armorySearch))
	m.HandleFunc("GET /api/armory/{name}", s.feature(s.c.EnableArmory, "Armory", s.armoryCharacter))
	m.HandleFunc("GET /api/armory/{name}/insights", s.feature(s.c.EnableArmory, "Armory", s.armoryInsights))
	m.HandleFunc("GET /api/arena", s.feature(s.c.EnableRankings, "Rankings", s.arenaRankings))
	m.HandleFunc("GET /api/rankings", s.feature(s.c.EnableRankings, "Rankings", s.expandedRankings))
	m.HandleFunc("GET /api/rankings/raids", s.feature(s.c.EnableRankings, "Rankings", s.raidRankings))
	m.HandleFunc("GET /api/progression/{name}", s.feature(s.c.EnableArmory, "Armory", s.raidProgression))
	m.HandleFunc("GET /api/realm", s.feature(s.c.EnableRealmStatus, "Realm status", s.realmOverview))
	m.HandleFunc("GET /api/guilds", s.feature(s.c.EnableGuilds, "Guilds", s.guildList))
	m.HandleFunc("GET /api/guilds/{id}", s.feature(s.c.EnableGuilds, "Guilds", s.guildDetail))
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /readyz", s.ready)
	m.HandleFunc("GET /metrics", s.prometheusMetrics)
	m.HandleFunc("GET /api/shop", s.feature(s.c.EnableShop, "Shop", s.shop))
	m.HandleFunc("POST /api/shop/purchase", s.feature(s.c.EnableShop, "Shop", s.rate(10, time.Minute, s.purchase)))
	m.HandleFunc("GET /api/characters/deleted", s.deletedCharacters)
	m.HandleFunc("POST /api/characters/{guid}/service", s.rate(8, time.Hour, s.characterService))
	m.HandleFunc("GET /api/orders", s.orders)
	m.HandleFunc("GET /api/dashboard", s.dashboard)
	m.HandleFunc("POST /api/rewards/daily", s.rate(3, time.Hour, s.claimDailyReward))
	m.HandleFunc("POST /api/rewards/vote/callback", s.rate(60, time.Minute, s.voteRewardCallback))
	m.HandleFunc("GET /api/tickets", s.feature(s.c.EnableSupport, "Support", s.tickets))
	m.HandleFunc("POST /api/tickets", s.feature(s.c.EnableSupport, "Support", s.rate(5, time.Hour, s.createTicket)))
	m.HandleFunc("POST /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProduct))
	m.HandleFunc("POST /api/admin/credits", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.adminCredits)))
	m.HandleFunc("POST /api/admin/orders/{id}/retry", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRetryOrder))
	m.HandleFunc("POST /api/admin/orders/{id}/refund", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRefundOrder))
	m.HandleFunc("GET /api/admin/orders", s.feature(s.c.EnableAdminPanel, "Administration", s.adminOrders))
	m.HandleFunc("GET /api/admin/status", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStatus))
	m.HandleFunc("GET /api/admin/analytics", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAnalytics))
	m.HandleFunc("GET /api/admin/ledger", s.feature(s.c.EnableAdminPanel, "Administration", s.adminLedger))
	m.HandleFunc("GET /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProducts))
	m.HandleFunc("GET /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductDetail))
	m.HandleFunc("GET /api/admin/items", s.feature(s.c.EnableAdminPanel, "Administration", s.adminItemSearch))
	m.HandleFunc("PUT /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductUpdate))
	m.HandleFunc("DELETE /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductDelete))
	m.HandleFunc("GET /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("POST /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("DELETE /api/admin/coupons/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCouponDelete))
	m.HandleFunc("GET /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("POST /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("PUT /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("DELETE /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("GET /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("PUT /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("GET /api/admin/accounts", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAccounts))
	m.HandleFunc("POST /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.adminModeration)))
	m.HandleFunc("GET /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.adminModerationLog))
	m.HandleFunc("GET /api/admin/audit", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAudit))
	m.HandleFunc("GET /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.adminConsoleHistory))
	m.HandleFunc("POST /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.rate(20, time.Minute, s.adminConsoleExecute)))
	m.HandleFunc("GET /api/admin/tickets", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminTickets))
	m.HandleFunc("POST /api/admin/tickets/{id}", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminTicketUpdate))
	m.HandleFunc("POST /api/billing/checkout", s.feature(s.c.EnableShop, "Shop", s.billingCheckout))
	m.HandleFunc("POST /api/billing/webhook", s.billingWebhook)
	m.HandleFunc("GET /api/security/sessions", s.securitySessions)
	m.HandleFunc("DELETE /api/security/sessions/{id}", s.securityRevokeSession)
	m.HandleFunc("POST /api/security/password", s.securityPassword)
	m.HandleFunc("POST /api/security/totp/setup", s.securityTOTPSetup)
	m.HandleFunc("POST /api/security/totp/enable", s.securityTOTPEnable)
	m.HandleFunc("POST /api/security/totp/disable", s.securityTOTPDisable)
	m.Handle("/", spaHandler(s.static))
	return s.middleware(m)
}

func (s *Server) feature(enabled bool, name string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.featureEnabled(r, name, enabled) {
			problem(w, http.StatusNotFound, name+" is disabled")
			return
		}
		next(w, r)
	}
}

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self' blob:; img-src 'self' https: data: blob:; style-src 'self' 'unsafe-inline' https://wow.zamimg.com; script-src 'self' https://code.jquery.com https://wow.zamimg.com https://challenges.cloudflare.com; connect-src 'self' https://nether.wowhead.com https://wow.zamimg.com https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; worker-src 'self' blob:")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if s.c.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			problem(w, http.StatusForbidden, "Invalid request origin")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !strings.HasPrefix(r.URL.Path, "/api/admin/") && r.URL.Path != "/api/auth/login" && r.URL.Path != "/api/auth/logout" && r.URL.Path != "/api/billing/webhook" {
			if active, message := s.maintenanceActive(r); active {
				if _, gm := s.requireGM(r); !gm {
					if strings.TrimSpace(message) == "" {
						message = "Scheduled maintenance is in progress"
					}
					problem(w, http.StatusServiceUnavailable, message)
					return
				}
			}
		}
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		s.metrics.requests.Add(1)
		if rw.status >= 500 {
			s.metrics.errors.Add(1)
		}
		slog.Info("request", "method", r.Method, "path", r.URL.Path, "duration", time.Since(start))
	})
}
func (s *Server) sameOrigin(r *http.Request) bool {
	o := r.Header.Get("Origin")
	if o == "" {
		return true
	}
	a, e1 := url.Parse(o)
	b, e2 := url.Parse(s.c.PublicURL)
	return e1 == nil && e2 == nil && strings.EqualFold(a.Host, b.Host) && a.Scheme == b.Scheme
}

func (s *Server) status(w http.ResponseWriter, r *http.Request) {
	status := s.serviceStatus(r)
	jsonOut(w, 200, map[string]any{"online": status["online"], "realm": status["realm"], "address": status["address"], "maintenance": status["maintenance"], "maintenanceMessage": status["maintenanceMessage"], "checkedAt": status["checkedAt"]})
}

func (s *Server) adminStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.requireStaffPermission(r, "monitoring"); !ok {
		problem(w, http.StatusForbidden, "GM access required")
		return
	}
	jsonOut(w, 200, s.serviceStatus(r))
}

func (s *Server) serviceStatus(r *http.Request) map[string]any {
	cfg := s.runtimeSettings(r)
	dbOK := s.s.Auth.PingContext(r.Context()) == nil
	realmOnline := false
	if dbOK {
		q := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s`.uptime WHERE realmid=? AND starttime+uptime>=UNIX_TIMESTAMP()-300)", s.c.AuthDB)
		_ = s.s.Auth.QueryRowContext(r.Context(), q, s.c.RealmID).Scan(&realmOnline)
	}
	now := time.Now()
	maintenance := cfg.MaintenanceEnabled && (cfg.MaintenanceStarts == nil || !now.Before(*cfg.MaintenanceStarts)) && (cfg.MaintenanceEnds == nil || now.Before(*cfg.MaintenanceEnds))
	return map[string]any{"online": realmOnline, "realm": cfg.RealmName, "address": cfg.RealmAddress, "shopDelivery": s.soap.Enabled(), "portal": true, "database": dbOK, "soapConfigured": s.soap.Enabled(), "maintenance": maintenance, "maintenanceMessage": cfg.MaintenanceMessage, "checkedAt": now}
}

func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	if s.c.EnableSetup {
		complete, err := s.isSetupComplete(r)
		if err != nil || !complete {
			problem(w, http.StatusServiceUnavailable, "Complete first-time setup before registering accounts")
			return
		}
	}
	var in struct {
		Username, Password, Email, TurnstileToken string
		ReferralCode                              string `json:"referralCode"`
	}
	if !decode(w, r, &in) {
		return
	}
	if !s.verifyTurnstile(r.Context(), in.TurnstileToken, s.clientIP(r)) {
		problem(w, 422, "Human verification failed")
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Email = strings.ToUpper(strings.TrimSpace(in.Email))
	if err := srp.Validate(in.Username, in.Password); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if !validEmailAddress(in.Email) {
		problem(w, 422, "Enter a valid email address")
		return
	}
	salt, verifier, err := srp.Registration(in.Username, in.Password)
	if err != nil {
		problem(w, 500, "Could not secure account")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	q := fmt.Sprintf("INSERT INTO `%s`.account (username,salt,verifier,email,reg_mail,expansion,locked) VALUES (?,?,?,?,?,?,?)", s.c.AuthDB)
	res, err := tx.ExecContext(r.Context(), q, in.Username, salt, verifier, in.Email, in.Email, s.c.Expansion, s.c.RequireEmailVerification)
	if err != nil {
		if strings.Contains(err.Error(), "Duplicate") {
			problem(w, 409, "That username is already taken")
		} else {
			problem(w, 500, "Could not create account")
		}
		return
	}
	id, _ := res.LastInsertId()
	var verificationToken string
	if s.c.RequireEmailVerification {
		verificationToken, err = createEmailVerification(r.Context(), tx, uint32(id))
		if err != nil {
			problem(w, 500, "Could not initialize email verification")
			return
		}
	}
	var referredBy uint32
	in.ReferralCode = strings.ToUpper(strings.TrimSpace(in.ReferralCode))
	if in.ReferralCode != "" {
		if !couponCodePattern.MatchString(in.ReferralCode) || tx.QueryRowContext(r.Context(), "SELECT account_id FROM portal_referrals WHERE code=?", in.ReferralCode).Scan(&referredBy) != nil || referredBy == uint32(id) {
			problem(w, 422, "Referral code is invalid")
			return
		}
	}
	startingCredits := s.c.StartingCredits
	if referredBy > 0 {
		startingCredits += 10
	}
	_, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets (account_id,balance) VALUES (?,?)", id, startingCredits)
	if err != nil {
		problem(w, 500, "Could not initialize account")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_referrals(account_id,code,referred_by) VALUES(?,?,?)", id, referralCode(in.Username, uint32(id)), referredBy); err != nil {
		problem(w, 500, "Could not initialize referral account")
		return
	}
	if referredBy > 0 {
		if _, err = tx.ExecContext(r.Context(), "UPDATE portal_wallets SET balance=balance+25 WHERE account_id=?", referredBy); err != nil {
			problem(w, 500, "Could not credit referral")
			return
		}
		_, _ = tx.ExecContext(r.Context(), "UPDATE portal_referrals SET uses=uses+1,credits_earned=credits_earned+25 WHERE account_id=?", referredBy)
		_, _ = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(0,?,25,'Successful referral')", referredBy)
	}
	realms := fmt.Sprintf("INSERT IGNORE INTO `%s`.realmcharacters (realmid,acctid,numchars) SELECT id,?,0 FROM `%s`.realmlist", s.c.AuthDB, s.c.AuthDB)
	if _, err = tx.ExecContext(r.Context(), realms, id); err != nil {
		problem(w, 500, "Could not initialize realms")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not create account")
		return
	}
	s.notifyDiscordAsync("New account", "**%s** registered on **%s**.", in.Username, s.c.RealmName)
	if s.c.RequireEmailVerification {
		go func() {
			if err := s.sendVerificationEmail(in.Email, in.Username, verificationToken); err != nil {
				slog.Error("send registration verification", "error", err)
			}
		}()
		jsonOut(w, 201, map[string]any{"ok": true, "verificationRequired": true, "message": "Account created. Check your email to activate it."})
		return
	}
	jsonOut(w, 201, map[string]any{"ok": true, "verificationRequired": false, "message": "Account created. You can now sign in."})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Username, Password, OTP string }
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	var a account
	var salt, verifier, totpSecret []byte
	var totpEnabled bool
	q := fmt.Sprintf("SELECT a.id,a.username,a.email,a.salt,a.verifier,COALESCE(ps.totp_secret,''),COALESCE(ps.totp_enabled,0) FROM `%s`.account a LEFT JOIN portal_account_security ps ON ps.account_id=a.id WHERE username=? AND locked=0 AND NOT EXISTS (SELECT 1 FROM `%s`.account_banned b WHERE b.id=a.id AND b.active=1)", s.c.AuthDB, s.c.AuthDB)
	if err := s.s.Auth.QueryRowContext(r.Context(), q, in.Username).Scan(&a.ID, &a.Username, &a.Email, &salt, &verifier, &totpSecret, &totpEnabled); err != nil || !srp.Verify(a.Username, in.Password, salt, verifier) {
		problem(w, 401, "Invalid username or password")
		return
	}
	if totpEnabled && !validTOTP(string(totpSecret), in.OTP, time.Now()) {
		problem(w, 401, "A valid authenticator code is required")
		return
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		problem(w, 500, "Could not create session")
		return
	}
	token := hex.EncodeToString(raw)
	hash := sha256.Sum256([]byte(token))
	expires := time.Now().Add(7 * 24 * time.Hour)
	ua := r.UserAgent()
	if len(ua) > 255 {
		ua = ua[:255]
	}
	ip := s.clientIP(r)
	if _, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_sessions (token_hash,account_id,expires_at,ip_address,user_agent) VALUES (?,?,?,?,?)", hash[:], a.ID, expires, ip, ua); err != nil {
		problem(w, 500, "Could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: token, Path: "/", Expires: expires, MaxAge: 604800, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]any{"account": a})
}
func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie("portal_session"); e == nil {
		h := sha256.Sum256([]byte(c.Value))
		_, _ = s.s.Auth.ExecContext(r.Context(), "DELETE FROM portal_sessions WHERE token_hash=?", h[:])
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]bool{"ok": true})
}

func (s *Server) auth(r *http.Request) (account, error) {
	var a account
	c, err := r.Cookie("portal_session")
	if err != nil {
		return a, err
	}
	h := sha256.Sum256([]byte(c.Value))
	q := fmt.Sprintf("SELECT a.id,a.username,a.email FROM portal_sessions s JOIN `%s`.account a ON a.id=s.account_id WHERE s.token_hash=? AND s.expires_at>NOW() AND a.locked=0 AND NOT EXISTS (SELECT 1 FROM `%s`.account_banned b WHERE b.id=a.id AND b.active=1)", s.c.AuthDB, s.c.AuthDB)
	err = s.s.Auth.QueryRowContext(r.Context(), q, h[:]).Scan(&a.ID, &a.Username, &a.Email)
	if err == nil {
		_, _ = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_sessions SET last_seen_at=NOW() WHERE token_hash=? AND last_seen_at < NOW()-INTERVAL 5 MINUTE", h[:])
	}
	return a, err
}
func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var balance uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", a.ID).Scan(&balance)
	a.GMLevel = s.gmLevel(r.Context(), a.ID)
	jsonOut(w, 200, map[string]any{"account": a, "balance": balance, "staffRole": staffRole(a, s.c), "permissions": s.staffPermissionsFor(a.GMLevel, a.Username)})
}

func staffRole(a account, c config.Config) string {
	if int(a.GMLevel) >= c.GMLevel {
		return "Administrator"
	}
	if c.StaffShopManagers[strings.ToUpper(a.Username)] {
		return "Shop manager"
	}
	if int(a.GMLevel) >= c.ModeratorGMLevel {
		return "Moderator"
	}
	if int(a.GMLevel) >= c.SupportGMLevel {
		return "Support"
	}
	return "Player"
}

func (s *Server) characterRows(ctx context.Context, accountID uint32) ([]character, error) {
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,c.money,COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.account=? AND c.deleteDate IS NULL ORDER BY c.level DESC,c.name`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(ctx, q, accountID)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := []character{}
	for rows.Next() {
		var c character
		if e = rows.Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Money, &c.Guild); e != nil {
			return nil, e
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
func (s *Server) characters(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	cs, e := s.characterRows(r.Context(), a.ID)
	if e != nil {
		problem(w, 500, "Could not load characters")
		return
	}
	jsonOut(w, 200, map[string]any{"characters": cs})
}

func (s *Server) armorySearch(w http.ResponseWriter, r *http.Request) {
	term := strings.TrimSpace(r.URL.Query().Get("q"))
	if len(term) > 12 {
		term = term[:12]
	}
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.deleteDate IS NULL AND c.name LIKE ? ORDER BY c.level DESC,c.name LIMIT 24`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), q, "%"+term+"%")
	if e != nil {
		problem(w, 500, "Could not search armory")
		return
	}
	defer rows.Close()
	out := []character{}
	for rows.Next() {
		var c character
		if rows.Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Guild) == nil {
			out = append(out, c)
		}
	}
	jsonOut(w, 200, map[string]any{"characters": out})
}

func (s *Server) armoryCharacter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var c character
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,COALESCE(g.name,'') FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.name=? AND c.deleteDate IS NULL`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	if e := s.s.Characters.QueryRowContext(r.Context(), q, name).Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Guild); e != nil {
		problem(w, 404, "Character not found")
		return
	}
	type item struct {
		Slot          uint8  `json:"slot"`
		Entry         uint32 `json:"entry"`
		Name          string `json:"name"`
		Quality       uint8  `json:"quality"`
		DisplayID     uint32 `json:"displayId"`
		ItemLevel     uint16 `json:"itemLevel"`
		RequiredLevel uint8  `json:"requiredLevel"`
		Armor         uint32 `json:"armor"`
		InventoryType uint8  `json:"inventoryType"`
		Icon          string `json:"icon"`
		Stats         []struct {
			Type  int16 `json:"type"`
			Value int16 `json:"value"`
		} `json:"stats"`
	}
	items := []item{}
	iq := fmt.Sprintf(`SELECT ci.slot,ii.itemEntry,it.name,it.Quality,it.displayid,it.ItemLevel,it.RequiredLevel,it.armor,it.InventoryType,it.stat_type1,it.stat_value1,it.stat_type2,it.stat_value2,it.stat_type3,it.stat_value3,it.stat_type4,it.stat_value4,it.stat_type5,it.stat_value5,it.stat_type6,it.stat_value6,it.stat_type7,it.stat_value7,it.stat_type8,it.stat_value8,it.stat_type9,it.stat_value9,it.stat_type10,it.stat_value10 FROM %s.character_inventory ci JOIN %s.item_instance ii ON ii.guid=ci.item JOIN %s.item_template it ON it.entry=ii.itemEntry WHERE ci.guid=? AND ci.bag=0 AND ci.slot<19 ORDER BY ci.slot`, s.c.CharactersDB, s.c.CharactersDB, s.c.WorldDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), iq, c.GUID)
	if e == nil {
		defer rows.Close()
		for rows.Next() {
			var i item
			var statTypes, statValues [10]int16
			if rows.Scan(&i.Slot, &i.Entry, &i.Name, &i.Quality, &i.DisplayID, &i.ItemLevel, &i.RequiredLevel, &i.Armor, &i.InventoryType,
				&statTypes[0], &statValues[0], &statTypes[1], &statValues[1], &statTypes[2], &statValues[2], &statTypes[3], &statValues[3], &statTypes[4], &statValues[4],
				&statTypes[5], &statValues[5], &statTypes[6], &statValues[6], &statTypes[7], &statValues[7], &statTypes[8], &statValues[8], &statTypes[9], &statValues[9]) == nil {
				for n := range statTypes {
					if statTypes[n] != 0 && statValues[n] != 0 {
						i.Stats = append(i.Stats, struct {
							Type  int16 `json:"type"`
							Value int16 `json:"value"`
						}{statTypes[n], statValues[n]})
					}
				}
				items = append(items, i)
			}
		}
	}
	jsonOut(w, 200, map[string]any{"character": c, "equipment": items, "profile": s.loadCharacterProfile(r.Context(), c.GUID), "arenaTeams": s.characterArenaTeams(r, c.GUID)})
}

func (s *Server) shop(w http.ResponseWriter, r *http.Request) {
	rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount,service_action,active,starts_at,ends_at,per_account_limit,featured,sale_price,stock_limit,sold_count,category_order FROM portal_products WHERE realm_key=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) ORDER BY featured DESC,category_order,category,class_id,price,name", s.c.RealmKey)
	if e != nil {
		problem(w, 500, "Could not load shop")
		return
	}
	defer rows.Close()
	out := []product{}
	for rows.Next() {
		var p product
		if rows.Scan(&p.ID, &p.Name, &p.Description, &p.ItemID, &p.Quantity, &p.Price, &p.Category, &p.ImageURL, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold, &p.ServiceAction, &p.Active, &p.StartsAt, &p.EndsAt, &p.PerAccountLimit, &p.Featured, &p.SalePrice, &p.StockLimit, &p.SoldCount, &p.CategoryOrder) == nil {
			out = append(out, p)
		}
	}
	rows.Close()
	byID := make(map[uint32]*product, len(out))
	for i := range out {
		byID[out[i].ID] = &out[i]
	}
	type productItemRef struct {
		productID uint32
		item      bundleItem
	}
	refs := []productItemRef{}
	itemIDs := []uint32{}
	seenIDs := map[uint32]bool{}
	itemRows, itemErr := s.s.Auth.QueryContext(r.Context(), "SELECT product_id,item_id,quantity FROM portal_product_items ORDER BY product_id,item_id")
	if itemErr == nil {
		for itemRows.Next() {
			var productID uint32
			var item bundleItem
			if itemRows.Scan(&productID, &item.ItemID, &item.Quantity) != nil {
				continue
			}
			p := byID[productID]
			if p == nil {
				continue
			}
			p.Items = append(p.Items, item)
			refs = append(refs, productItemRef{productID, item})
			if !seenIDs[item.ItemID] {
				seenIDs[item.ItemID] = true
				itemIDs = append(itemIDs, item.ItemID)
			}
		}
		itemRows.Close()
	}
	names := map[uint32]string{}
	if len(itemIDs) > 0 {
		args := make([]any, len(itemIDs))
		for i, id := range itemIDs {
			args[i] = id
		}
		q := fmt.Sprintf("SELECT entry,name FROM `%s`.item_template WHERE entry IN (?%s)", s.c.WorldDB, strings.Repeat(",?", len(itemIDs)-1))
		if nameRows, err := s.s.World.QueryContext(r.Context(), q, args...); err == nil {
			for nameRows.Next() {
				var id uint32
				var name string
				if nameRows.Scan(&id, &name) == nil {
					names[id] = name
				}
			}
			nameRows.Close()
		}
	}
	for _, ref := range refs {
		name := names[ref.item.ItemID]
		if name == "" {
			name = fmt.Sprintf("item %d", ref.item.ItemID)
		}
		if (byID[ref.productID].Tier == "S6" || byID[ref.productID].Tier == "S7") && name == "Medallion of the Alliance" {
			name = "Medallion of the Alliance/Horde (selected for character)"
		}
		if byID[ref.productID].ServiceLevel == 80 {
			switch ref.item.ItemID {
			case allianceGroundMountItem:
				name = "Faction-appropriate epic ground mount"
			case allianceFlyingMountItem:
				name = "Faction-appropriate epic flying mount"
			}
		}
		byID[ref.productID].Includes = append(byID[ref.productID].Includes, fmt.Sprintf("%d × %s", ref.item.Quantity, name))
	}
	for i := range out {
		if out[i].ServiceLevel == 80 {
			out[i].Includes = append(out[i].Includes,
				"All class trainer spell ranks",
				"All class weapon proficiencies at 400",
				"Artisan Riding and Cold Weather Flying",
			)
		}
		if out[i].Gold > 0 {
			out[i].Includes = append(out[i].Includes, fmt.Sprintf("%s gold", commaNumber(out[i].Gold)))
		}
	}
	jsonOut(w, 200, map[string]any{"products": out, "deliveryEnabled": s.soap.Enabled()})
}

func commaNumber(value uint32) string {
	digits := strconv.FormatUint(uint64(value), 10)
	for i := len(digits) - 3; i > 0; i -= 3 {
		digits = digits[:i] + "," + digits[i:]
	}
	return digits
}

func (s *Server) purchase(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	var in struct {
		ProductID, CharacterGUID uint32
		Coupon                   string `json:"coupon"`
	}
	if !decode(w, r, &in) {
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var p product
	if e = tx.QueryRowContext(r.Context(), "SELECT id,name,item_id,quantity,price,class_id,tier_label,service_level,gold_amount,service_action,per_account_limit,featured,sale_price,stock_limit,sold_count,category_order FROM portal_products WHERE id=? AND realm_key=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) FOR UPDATE", in.ProductID, s.c.RealmKey).Scan(&p.ID, &p.Name, &p.ItemID, &p.Quantity, &p.Price, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold, &p.ServiceAction, &p.PerAccountLimit, &p.Featured, &p.SalePrice, &p.StockLimit, &p.SoldCount, &p.CategoryOrder); e != nil {
		problem(w, 404, "Product not found")
		return
	}
	if p.PerAccountLimit > 0 {
		var count uint32
		if e = tx.QueryRowContext(r.Context(), "SELECT COUNT(*) FROM portal_orders WHERE account_id=? AND product_id=? AND realm_key=? AND status NOT IN ('failed','refunded')", a.ID, p.ID, s.c.RealmKey).Scan(&count); e != nil {
			problem(w, 500, "Could not validate purchase limit")
			return
		}
		if count >= p.PerAccountLimit {
			problem(w, 409, "This product's account purchase limit has been reached")
			return
		}
	}
	if p.StockLimit > 0 && p.SoldCount >= p.StockLimit {
		problem(w, 409, "This product is sold out")
		return
	}
	basePrice := p.Price
	if p.SalePrice > 0 && p.SalePrice < basePrice {
		basePrice = p.SalePrice
	}
	discount, couponID, couponCode, e := s.applyCoupon(r, tx, a.ID, in.Coupon, basePrice)
	if e != nil {
		problem(w, 422, e.Error())
		return
	}
	total := basePrice - discount
	var balance uint32
	if e = tx.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=? FOR UPDATE", a.ID).Scan(&balance); e != nil || balance < total {
		problem(w, 422, "Not enough credits")
		return
	}
	var characterName string
	var online bool
	var characterClass, characterRace uint8
	cq := fmt.Sprintf("SELECT name,online,class,race FROM %s.characters WHERE guid=? AND account=? AND deleteDate IS NULL", s.c.CharactersDB)
	if e = s.s.Characters.QueryRowContext(r.Context(), cq, in.CharacterGUID, a.ID).Scan(&characterName, &online, &characterClass, &characterRace); e != nil {
		problem(w, 422, "Choose one of your characters")
		return
	}
	if online {
		problem(w, 409, "Character must be offline for delivery")
		return
	}
	if p.ClassID != 0 && characterClass != p.ClassID {
		problem(w, 422, "This package does not match the selected character's class")
		return
	}
	if !s.soap.Enabled() {
		problem(w, 503, "Shop delivery is not configured")
		return
	}
	if _, e = tx.ExecContext(r.Context(), "UPDATE portal_wallets SET balance=balance-? WHERE account_id=?", total, a.ID); e != nil {
		problem(w, 500, "Could not debit wallet")
		return
	}
	res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_orders(account_id,character_guid,realm_key,product_id,item_id,quantity,total,subtotal,discount,coupon_code,status,service_level,gold_amount,service_action) VALUES(?,?,?,?,?,?,?,?,?,?,'pending',?,?,?)", a.ID, in.CharacterGUID, s.c.RealmKey, p.ID, p.ItemID, p.Quantity, total, basePrice, discount, couponCode, p.ServiceLevel, p.Gold, p.ServiceAction)
	if e != nil {
		problem(w, 500, "Could not create order")
		return
	}
	orderID, _ := res.LastInsertId()
	if couponID > 0 {
		if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_coupon_uses(coupon_id,account_id,order_id) VALUES(?,?,?)", couponID, a.ID, orderID); e != nil {
			problem(w, 500, "Could not redeem coupon")
			return
		}
	}
	if strings.ContainsAny(characterName, " \t\r\n\"\\") {
		problem(w, 422, "Character name cannot be used for delivery")
		return
	}
	items := []bundleItem{}
	itemRows, itemErr := tx.QueryContext(r.Context(), "SELECT item_id,quantity FROM portal_product_items WHERE product_id=? ORDER BY item_id", p.ID)
	if itemErr == nil {
		for itemRows.Next() {
			var item bundleItem
			if itemRows.Scan(&item.ItemID, &item.Quantity) == nil {
				items = append(items, item)
			}
		}
		itemRows.Close()
	}
	if len(items) == 0 && p.ItemID != 0 {
		items = append(items, bundleItem{ItemID: p.ItemID, Quantity: p.Quantity})
	}
	if p.Tier == "S6" || p.Tier == "S7" {
		allianceID, hordeID, medallionErr := s.pvpMedallionIDs(r.Context())
		if medallionErr != nil {
			problem(w, 500, "Could not resolve faction PvP trinket")
			return
		}
		chosenID := hordeID
		if isAllianceRace(characterRace) {
			chosenID = allianceID
		} else if !isHordeRace(characterRace) {
			problem(w, 422, "Character race has no supported faction")
			return
		}
		replaced := false
		for i := range items {
			if items[i].ItemID == allianceID {
				items[i].ItemID = chosenID
				replaced = true
			}
		}
		if !replaced {
			problem(w, 500, "PvP package is missing its faction trinket")
			return
		}
	}
	if p.ServiceLevel == 80 {
		if e = applyStarterMountFaction(items, characterRace); e != nil {
			problem(w, 422, e.Error())
			return
		}
	}
	for _, item := range items {
		if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_order_items(order_id,item_id,quantity) VALUES(?,?,?)", orderID, item.ItemID, item.Quantity); e != nil {
			problem(w, 500, "Could not snapshot order items")
			return
		}
	}
	if p.StockLimit > 0 {
		result, stockErr := tx.ExecContext(r.Context(), "UPDATE portal_products SET sold_count=sold_count+1 WHERE id=? AND realm_key=? AND sold_count<stock_limit", p.ID, s.c.RealmKey)
		if stockErr != nil {
			problem(w, 500, "Could not reserve product stock")
			return
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			problem(w, 409, "This product just sold out")
			return
		}
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "Could not queue order")
		return
	}
	s.metrics.orders.Add(1)
	s.notifyDiscordAsync("New shop order", "Order **#%d** · **%s** bought **%s** for **%d credits** on **%s**.", orderID, a.Username, p.Name, total, s.c.RealmName)
	jsonOut(w, 202, map[string]any{"ok": true, "orderId": orderID, "message": "Order accepted and queued for in-game delivery."})
}

const (
	allianceGroundMountItem uint32 = 18777
	hordeGroundMountItem    uint32 = 18796
	allianceFlyingMountItem uint32 = 25528
	hordeFlyingMountItem    uint32 = 25533
)

func applyStarterMountFaction(items []bundleItem, race uint8) error {
	alliance := isAllianceRace(race)
	if !alliance && !isHordeRace(race) {
		return fmt.Errorf("character race has no supported faction")
	}
	if alliance {
		return nil
	}
	for i := range items {
		switch items[i].ItemID {
		case allianceGroundMountItem:
			items[i].ItemID = hordeGroundMountItem
		case allianceFlyingMountItem:
			items[i].ItemID = hordeFlyingMountItem
		}
	}
	return nil
}

func (s *Server) pvpMedallionIDs(ctx context.Context) (alliance, horde uint32, err error) {
	q := fmt.Sprintf("SELECT entry FROM `%s`.item_template WHERE name=? AND ItemLevel<=226 AND RequiredLevel<=80 AND VerifiedBuild>1 ORDER BY ItemLevel DESC,entry DESC LIMIT 1", s.c.WorldDB)
	if err = s.s.World.QueryRowContext(ctx, q, "Medallion of the Alliance").Scan(&alliance); err != nil {
		return 0, 0, err
	}
	if err = s.s.World.QueryRowContext(ctx, q, "Medallion of the Horde").Scan(&horde); err != nil {
		return 0, 0, err
	}
	return alliance, horde, nil
}

func isAllianceRace(race uint8) bool {
	return race == 1 || race == 3 || race == 4 || race == 7 || race == 11
}

func isHordeRace(race uint8) bool {
	return race == 2 || race == 5 || race == 6 || race == 8 || race == 10
}

func (s *Server) orders(w http.ResponseWriter, r *http.Request) {
	a, e := s.auth(r)
	if e != nil {
		problem(w, 401, "Sign in required")
		return
	}
	rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,item_id,quantity,total,status,created_at FROM portal_orders WHERE account_id=? AND realm_key=? ORDER BY id DESC LIMIT 50", a.ID, s.c.RealmKey)
	if e != nil {
		problem(w, 500, "Could not load orders")
		return
	}
	defer rows.Close()
	type order struct {
		ID                      uint64 `json:"id"`
		ItemID, Quantity, Total uint32
		Status                  string
		Created                 time.Time
	}
	out := []order{}
	for rows.Next() {
		var o order
		if rows.Scan(&o.ID, &o.ItemID, &o.Quantity, &o.Total, &o.Status, &o.Created) == nil {
			out = append(out, o)
		}
	}
	jsonOut(w, 200, map[string]any{"orders": out})
}

func (s *Server) adminProduct(w http.ResponseWriter, r *http.Request) {
	provided := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	tokenOK := s.c.AdminToken != "" && len(provided) == len(s.c.AdminToken) && subtle.ConstantTimeCompare([]byte(provided), []byte(s.c.AdminToken)) == 1
	gmOK := false
	var actorID uint32
	if !tokenOK {
		actor, ok := s.requireStaffPermission(r, "commerce")
		gmOK = ok
		actorID = actor.ID
	}
	if !tokenOK && !gmOK {
		problem(w, 401, "GM session or admin token required")
		return
	}
	var p product
	if !decode(w, r, &p) {
		return
	}
	if err := validateManagedProduct(p); err != nil {
		problem(w, 422, err.Error())
		return
	}
	if p.ServiceAction != "" && p.ServiceAction != "race_change" && p.ServiceAction != "faction_change" {
		problem(w, 422, "serviceAction must be race_change or faction_change")
		return
	}
	if p.ImageURL != "" {
		u, err := url.ParseRequestURI(p.ImageURL)
		if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
			problem(w, 422, "imageUrl must be an absolute HTTP or HTTPS URL")
			return
		}
	}
	if len(p.Items) > 48 {
		problem(w, 422, "A package supports at most 48 distinct items")
		return
	}
	if e := s.validateProductItems(r.Context(), p); e != nil {
		problem(w, 422, e.Error())
		return
	}
	tx, e := s.s.Auth.BeginTx(r.Context(), nil)
	if e != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	if p.Gold > 200000 {
		problem(w, 422, "Gold amount exceeds the WotLK safe limit")
		return
	}
	res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_products(name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount,service_action,active,starts_at,ends_at,per_account_limit,realm_key,featured,sale_price,stock_limit,category_order) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?,?,?,?)", p.Name, p.Description, p.ItemID, p.Quantity, p.Price, p.Category, p.ImageURL, p.ClassID, p.Tier, p.ServiceLevel, p.Gold, p.ServiceAction, p.StartsAt, p.EndsAt, p.PerAccountLimit, s.c.RealmKey, p.Featured, p.SalePrice, p.StockLimit, p.CategoryOrder)
	if e != nil {
		problem(w, 500, "Could not create product")
		return
	}
	id, _ := res.LastInsertId()
	for _, item := range p.Items {
		if item.ItemID == 0 || item.Quantity == 0 {
			problem(w, 422, "Bundle item IDs and quantities must be positive")
			return
		}
		if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_product_items(product_id,item_id,quantity) VALUES(?,?,?)", id, item.ItemID, item.Quantity); e != nil {
			problem(w, 500, "Could not create product bundle")
			return
		}
	}
	if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_admin_audit(actor_account_id,action,target,details) VALUES(?,'product.create',?,?)", actorID, strconv.FormatInt(id, 10), p.Name); e != nil {
		problem(w, 500, "Could not audit product creation")
		return
	}
	if e = tx.Commit(); e != nil {
		problem(w, 500, "Could not create product")
		return
	}
	jsonOut(w, 201, map[string]any{"id": id})
}

func (s *Server) gmLevel(ctx context.Context, accountID uint32) uint8 {
	q := fmt.Sprintf("SELECT COALESCE(MAX(gmlevel),0) FROM `%s`.account_access WHERE id=? AND (RealmID=-1 OR RealmID=?)", s.c.AuthDB)
	var level uint8
	_ = s.s.Auth.QueryRowContext(ctx, q, accountID, s.c.RealmID).Scan(&level)
	return level
}

func (s *Server) adminCredits(w http.ResponseWriter, r *http.Request) {
	actor, ok := s.requireStaffPermission(r, "commerce")
	if !ok {
		problem(w, 403, "GM access required")
		return
	}
	var err error
	var in struct {
		Username string `json:"username"`
		Amount   uint32 `json:"amount"`
		Reason   string `json:"reason"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Username = strings.ToUpper(strings.TrimSpace(in.Username))
	in.Reason = strings.TrimSpace(in.Reason)
	if in.Username == "" || in.Amount == 0 || in.Amount > 1000000 || len(in.Reason) < 3 || len(in.Reason) > 255 {
		problem(w, 422, "Username, 1–1,000,000 credits, and a reason are required")
		return
	}
	tx, err := s.s.Auth.BeginTx(r.Context(), nil)
	if err != nil {
		problem(w, 503, "Database unavailable")
		return
	}
	defer tx.Rollback()
	var targetID uint32
	q := fmt.Sprintf("SELECT id FROM `%s`.account WHERE username=?", s.c.AuthDB)
	if err = tx.QueryRowContext(r.Context(), q, in.Username).Scan(&targetID); err != nil {
		problem(w, 404, "Account not found")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_wallets(account_id,balance) VALUES(?,?) ON DUPLICATE KEY UPDATE balance=balance+VALUES(balance)", targetID, in.Amount); err != nil {
		problem(w, 500, "Could not update wallet")
		return
	}
	if _, err = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(?,?,?,?)", actor.ID, targetID, in.Amount, in.Reason); err != nil {
		problem(w, 500, "Could not record credit grant")
		return
	}
	var balance uint32
	if err = tx.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", targetID).Scan(&balance); err != nil {
		problem(w, 500, "Could not read wallet")
		return
	}
	if err = tx.Commit(); err != nil {
		problem(w, 500, "Could not commit credit grant")
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true, "username": in.Username, "amount": in.Amount, "balance": balance})
}

func (s *Server) rate(max int, window time.Duration, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		host := s.clientIP(r)
		key := host + ":" + r.URL.Path
		now := time.Now()
		s.limiter.mu.Lock()
		if s.limiter.lastSweep.IsZero() || now.Sub(s.limiter.lastSweep) >= 5*time.Minute {
			for candidate, timestamps := range s.limiter.hits {
				fresh := timestamps[:0]
				for _, timestamp := range timestamps {
					if now.Sub(timestamp) < time.Hour {
						fresh = append(fresh, timestamp)
					}
				}
				if len(fresh) == 0 {
					delete(s.limiter.hits, candidate)
				} else {
					s.limiter.hits[candidate] = fresh
				}
			}
			s.limiter.lastSweep = now
		}
		old := s.limiter.hits[key]
		keep := old[:0]
		for _, t := range old {
			if now.Sub(t) < window {
				keep = append(keep, t)
			}
		}
		allowed := len(keep) < max
		if allowed {
			keep = append(keep, now)
		}
		s.limiter.hits[key] = keep
		s.limiter.mu.Unlock()
		if !allowed {
			problem(w, 429, "Too many attempts. Try again later.")
			return
		}
		next(w, r)
	}
}

func (s *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if !s.c.TrustProxy {
		return host
	}
	forwarded := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0])
	if net.ParseIP(forwarded) != nil {
		return forwarded
	}
	realIP := strings.TrimSpace(r.Header.Get("X-Real-IP"))
	if net.ParseIP(realIP) != nil {
		return realIP
	}
	return host
}
func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if e := d.Decode(v); e != nil {
		problem(w, 400, "Invalid request body")
		return false
	}
	return true
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func problem(w http.ResponseWriter, status int, msg string) {
	jsonOut(w, status, map[string]string{"error": msg})
}
func spaHandler(root fs.FS) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.Trim(strings.TrimSpace(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if info, e := fs.Stat(root, p); e == nil {
			if info.IsDir() {
				p += "/index.html"
				info, e = fs.Stat(root, p)
				if e != nil {
					http.NotFound(w, r)
					return
				}
			}
			data, readErr := fs.ReadFile(root, p)
			if readErr != nil {
				http.NotFound(w, r)
				return
			}
			http.ServeContent(w, r, p, info.ModTime(), bytes.NewReader(data))
			return
		}
		if strings.Contains(p, ".") {
			http.NotFound(w, r)
			return
		}
		if strings.HasPrefix(p, "admin/") {
			if data, e := fs.ReadFile(root, "admin/index.html"); e == nil {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write(data)
				return
			}
		}
		data, e := fs.ReadFile(root, "index.html")
		if e != nil {
			http.Error(w, "UI not built", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
	})
}

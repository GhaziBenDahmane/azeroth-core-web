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
	s            *store.Store
	c            config.Config
	soap         *soap.Client
	static       fs.FS
	limiter      *limiter
	mock         *mockState
	metrics      portalMetrics
	stopDelivery chan struct{}
	stopOnce     sync.Once
	deliveryWG   sync.WaitGroup
}
type portalMetrics struct {
	requests, errors, orders                       atomic.Uint64
	loginFailures, rateLimitHits                   atomic.Uint64
	webhookFailures, emailFailures                 atomic.Uint64
	soapRequests, soapFaults, deliverySuccess      atomic.Uint64
	deliveryReview                                 atomic.Uint64
	authDBLatencyMicros, charactersDBLatencyMicros atomic.Int64
	worldDBLatencyMicros, soapLatencyMicros        atomic.Int64
	authDBReachable, charactersDBReachable         atomic.Bool
	worldDBReachable                               atomic.Bool
	dbLastSuccessUnix, soapLastSuccessUnix         atomic.Int64
}
type limiter struct {
	mu        sync.Mutex
	hits      map[string][]time.Time
	lastSweep time.Time
}

func New(s *store.Store, c config.Config, static fs.FS) *Server {
	server := &Server{s: s, c: c, soap: soap.New(c.SOAPURL, c.SOAPUser, c.SOAPPassword), static: static, limiter: &limiter{hits: map[string][]time.Time{}}, mock: newMockState(), stopDelivery: make(chan struct{})}
	if !c.MockMode && server.soap.Enabled() {
		server.deliveryWG.Add(1)
		go server.deliveryLoop()
	}
	return server
}

// Close stops background workers and waits for the current delivery attempt to
// finish, up to the caller's shutdown deadline.
func (s *Server) Close(ctx context.Context) {
	s.stopOnce.Do(func() { close(s.stopDelivery) })
	done := make(chan struct{})
	go func() { s.deliveryWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-ctx.Done():
	}
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
	m.HandleFunc("GET /api/auth/discord/start", s.discordOAuthStart)
	m.HandleFunc("GET /api/auth/discord/callback", s.discordOAuthCallback)
	m.HandleFunc("GET /api/auth/google/start", s.googleOAuthStart)
	m.HandleFunc("GET /api/auth/google/callback", s.googleOAuthCallback)
	m.HandleFunc("POST /api/auth/passkey/options", s.rate(20, 10*time.Minute, s.passkeyAuthenticationOptions))
	m.HandleFunc("POST /api/auth/passkey", s.rate(20, 10*time.Minute, s.passkeyAuthenticationFinish))
	m.HandleFunc("POST /api/auth/password/request", s.rate(5, time.Hour, s.passwordResetRequest))
	m.HandleFunc("POST /api/auth/password/reset", s.rate(10, time.Hour, s.passwordResetConfirm))
	m.HandleFunc("POST /api/auth/email/verify", s.rate(10, time.Hour, s.emailVerificationConfirm))
	m.HandleFunc("POST /api/auth/email/resend", s.rate(5, time.Hour, s.emailVerificationResend))
	m.HandleFunc("GET /api/public-config", s.publicConfig)
	m.HandleFunc("GET /api/media/{id}/{name}", s.mediaServe)
	m.HandleFunc("GET /api/news", s.newsList)
	m.HandleFunc("GET /api/news/{slug}", s.newsDetail)
	m.HandleFunc("GET /api/pages", s.publicPages)
	m.HandleFunc("GET /api/pages/{slug}", s.publicPage)
	m.HandleFunc("GET /api/events", s.publicEvents)
	m.HandleFunc("POST /api/events/{id}/registration", s.rate(10, time.Hour, s.eventRegistrationAction))
	m.HandleFunc("DELETE /api/events/{id}/registration", s.rate(10, time.Hour, s.eventRegistrationAction))
	m.HandleFunc("GET /api/community/discord", s.discordStatus)
	m.HandleFunc("GET /api/community/issues", s.communityIssues)
	m.HandleFunc("POST /api/community/issues", s.rate(5, time.Hour, s.createCommunityIssue))
	m.HandleFunc("GET /api/community/issues/{id}", s.communityIssueDetail)
	m.HandleFunc("POST /api/community/issues/{id}/vote", s.rate(30, time.Minute, s.communityIssueVote))
	m.HandleFunc("POST /api/community/issues/{id}/comments", s.rate(20, time.Hour, s.communityIssueComment))
	m.HandleFunc("GET /api/downloads", s.downloads)
	m.HandleFunc("GET /api/launcher/manifest", s.launcherManifest)
	m.HandleFunc("GET /api/tools/resources", s.publicTools)
	m.HandleFunc("GET /api/tools/items", s.itemDatabase)
	m.HandleFunc("GET /api/tools/talents", s.talentCalculator)
	m.HandleFunc("GET /api/me", s.me)
	m.HandleFunc("GET /api/identity/accounts", s.identityAccounts)
	m.HandleFunc("POST /api/identity/accounts", s.identityLinkAccount)
	m.HandleFunc("POST /api/identity/accounts/{id}/switch", s.identitySwitchAccount)
	m.HandleFunc("PATCH /api/identity/accounts/{id}", s.identityRenameAccount)
	m.HandleFunc("POST /api/identity/accounts/{id}/primary", s.identityPromoteAccount)
	m.HandleFunc("DELETE /api/identity/accounts/{id}", s.identityUnlinkAccount)
	m.HandleFunc("DELETE /api/identity/providers/{provider}", s.identityUnlinkProvider)
	m.HandleFunc("GET /api/security/passkeys", s.passkeyList)
	m.HandleFunc("POST /api/security/passkeys/register/options", s.passkeyRegistrationOptions)
	m.HandleFunc("POST /api/security/passkeys/register", s.passkeyRegistrationFinish)
	m.HandleFunc("DELETE /api/security/passkeys/{id}", s.passkeyDelete)
	m.HandleFunc("GET /api/characters", s.characters)
	m.HandleFunc("GET /api/armory", s.feature(s.c.EnableArmory, "Armory", s.armorySearch))
	m.HandleFunc("GET /api/armory/{name}", s.feature(s.c.EnableArmory, "Armory", s.armoryCharacter))
	m.HandleFunc("GET /api/armory/{name}/insights", s.feature(s.c.EnableArmory, "Armory", s.armoryInsights))
	m.HandleFunc("GET /api/arena", s.feature(s.c.EnableRankings, "Rankings", s.arenaRankings))
	m.HandleFunc("GET /api/rankings", s.feature(s.c.EnableRankings, "Rankings", s.expandedRankings))
	m.HandleFunc("GET /api/rankings/capabilities", s.feature(s.c.EnableRankings, "Rankings", s.rankingCapabilities))
	m.HandleFunc("GET /api/rankings/raids", s.feature(s.c.EnableRankings, "Rankings", s.raidRankings))
	m.HandleFunc("GET /api/progression/{name}", s.feature(s.c.EnableArmory, "Armory", s.raidProgression))
	m.HandleFunc("GET /api/realm", s.feature(s.c.EnableRealmStatus, "Realm status", s.realmOverview))
	m.HandleFunc("GET /api/guilds", s.feature(s.c.EnableGuilds, "Guilds", s.guildList))
	m.HandleFunc("GET /api/guilds/{id}", s.feature(s.c.EnableGuilds, "Guilds", s.guildDetail))
	m.HandleFunc("GET /api/guilds/{id}/recruitment", s.feature(s.c.EnableGuilds, "Guilds", s.guildRecruitmentProfile))
	m.HandleFunc("POST /api/guilds/{id}/applications", s.feature(s.c.EnableGuilds, "Guilds", s.rate(5, 24*time.Hour, s.createGuildApplication)))
	m.HandleFunc("GET /api/guild-applications", s.feature(s.c.EnableGuilds, "Guilds", s.guildApplications))
	m.HandleFunc("DELETE /api/guild-applications/{id}", s.feature(s.c.EnableGuilds, "Guilds", s.withdrawGuildApplication))
	m.HandleFunc("GET /healthz", s.health)
	m.HandleFunc("GET /readyz", s.ready)
	m.HandleFunc("GET /metrics", s.prometheusMetrics)
	m.HandleFunc("GET /api/shop", s.feature(s.c.EnableShop, "Shop", s.shop))
	m.HandleFunc("GET /api/shop/collections", s.feature(s.c.EnableShop, "Shop", s.shopCollections))
	m.HandleFunc("GET /api/shop/{id}", s.feature(s.c.EnableShop, "Shop", s.shopProductDetail))
	m.HandleFunc("GET /api/shop/{id}/eligibility", s.feature(s.c.EnableShop, "Shop", s.shopProductEligibility))
	m.HandleFunc("GET /api/wishlist", s.feature(s.c.EnableShop, "Shop", s.wishlist))
	m.HandleFunc("PUT /api/wishlist/{id}", s.feature(s.c.EnableShop, "Shop", s.wishlistItem))
	m.HandleFunc("DELETE /api/wishlist/{id}", s.feature(s.c.EnableShop, "Shop", s.wishlistItem))
	m.HandleFunc("POST /api/shop/purchase", s.feature(s.c.EnableShop, "Shop", s.rate(10, time.Minute, s.purchase)))
	m.HandleFunc("GET /api/characters/deleted", s.deletedCharacters)
	m.HandleFunc("POST /api/characters/{guid}/service", s.rate(8, time.Hour, s.characterService))
	m.HandleFunc("GET /api/characters/{guid}/privacy", s.characterPrivacySettings)
	m.HandleFunc("PUT /api/characters/{guid}/privacy", s.characterPrivacySettings)
	m.HandleFunc("GET /api/orders", s.orders)
	m.HandleFunc("GET /api/wallet", s.wallet)
	m.HandleFunc("GET /api/notifications", s.notifications)
	m.HandleFunc("POST /api/notifications/{id}/read", s.notificationRead)
	m.HandleFunc("GET /api/dashboard", s.dashboard)
	m.HandleFunc("POST /api/rewards/daily", s.rate(3, time.Hour, s.claimDailyReward))
	m.HandleFunc("POST /api/rewards/referrals/{id}/claim", s.rate(10, time.Hour, s.claimReferralMilestone))
	m.HandleFunc("POST /api/rewards/missions/{id}/claim", s.rate(10, time.Hour, s.claimPlayerMission))
	m.HandleFunc("POST /api/rewards/vote/callback", s.rate(60, time.Minute, s.voteRewardCallback))
	m.HandleFunc("POST /api/integrations/discord/rewards", s.rate(120, time.Minute, s.discordRewardCallback))
	m.HandleFunc("GET /api/votes", s.votes)
	m.HandleFunc("GET /api/votes/history", s.voteHistory)
	m.HandleFunc("GET /api/votes/leaderboard", s.voteLeaderboard)
	m.HandleFunc("GET /api/votes/campaigns", s.voteCampaigns)
	m.HandleFunc("POST /api/votes/{id}/visit", s.rate(20, time.Minute, s.visitVoteSite))
	m.HandleFunc("POST /api/integrations/votes/{slug}", s.rate(120, time.Minute, s.voteSiteCallback))
	m.HandleFunc("POST /api/integrations/raids", s.rate(240, time.Minute, s.ingestRaidKill))
	m.HandleFunc("POST /api/integrations/pvp", s.rate(240, time.Minute, s.ingestPvPMatch))
	m.HandleFunc("POST /api/integrations/battlegrounds", s.rate(240, time.Minute, s.ingestBattlegroundMatch))
	m.HandleFunc("GET /api/tickets", s.feature(s.c.EnableSupport, "Support", s.tickets))
	m.HandleFunc("POST /api/tickets", s.feature(s.c.EnableSupport, "Support", s.rate(5, time.Hour, s.createTicket)))
	m.HandleFunc("POST /api/tickets/{id}/messages", s.feature(s.c.EnableSupport, "Support", s.rate(20, time.Hour, s.ticketMessage)))
	m.HandleFunc("GET /api/moderation/sanctions", s.playerSanctions)
	m.HandleFunc("POST /api/moderation/sanctions/{id}/appeal", s.rate(3, 24*time.Hour, s.createSanctionAppeal))
	m.HandleFunc("GET /api/transfers", s.transfers)
	m.HandleFunc("POST /api/transfers", s.rate(3, 24*time.Hour, s.createTransfer))
	m.HandleFunc("POST /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProduct))
	m.HandleFunc("POST /api/admin/credits", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.adminCredits)))
	m.HandleFunc("POST /api/admin/orders/{id}/retry", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRetryOrder))
	m.HandleFunc("POST /api/admin/orders/bulk-retry", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(10, time.Minute, s.adminBulkRetryOrders)))
	m.HandleFunc("POST /api/admin/orders/{id}/refund", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRefundOrder))
	m.HandleFunc("GET /api/admin/orders", s.feature(s.c.EnableAdminPanel, "Administration", s.adminOrders))
	m.HandleFunc("GET /api/admin/orders/{id}/steps", s.feature(s.c.EnableAdminPanel, "Administration", s.adminOrderSteps))
	m.HandleFunc("POST /api/admin/orders/{id}/steps/{key}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminResolveOrderStep))
	m.HandleFunc("GET /api/admin/status", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStatus))
	m.HandleFunc("POST /api/admin/delivery-diagnostic", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(3, time.Hour, s.adminDeliveryDiagnostic)))
	m.HandleFunc("GET /api/admin/analytics", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAnalytics))
	m.HandleFunc("GET /api/admin/ledger", s.feature(s.c.EnableAdminPanel, "Administration", s.adminLedger))
	m.HandleFunc("GET /api/admin/products", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProducts))
	m.HandleFunc("GET /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductDetail))
	m.HandleFunc("GET /api/admin/items", s.feature(s.c.EnableAdminPanel, "Administration", s.adminItemSearch))
	m.HandleFunc("PUT /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductUpdate))
	m.HandleFunc("DELETE /api/admin/products/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductDelete))
	m.HandleFunc("POST /api/admin/products/{id}/validate", s.feature(s.c.EnableAdminPanel, "Administration", s.adminProductValidation))
	m.HandleFunc("POST /api/admin/products/import", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCatalogImport))
	m.HandleFunc("GET /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("POST /api/admin/coupons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCoupons))
	m.HandleFunc("DELETE /api/admin/coupons/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCouponDelete))
	m.HandleFunc("GET /api/admin/coupons/{id}/history", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCouponHistory))
	m.HandleFunc("GET /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("GET /api/admin/news/{id}/revisions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsRevisions))
	m.HandleFunc("GET /api/admin/vote-sites", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteSites))
	m.HandleFunc("POST /api/admin/vote-sites", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteSites))
	m.HandleFunc("PUT /api/admin/vote-sites/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteSite))
	m.HandleFunc("DELETE /api/admin/vote-sites/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteSite))
	m.HandleFunc("GET /api/admin/vote-campaigns", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteCampaigns))
	m.HandleFunc("POST /api/admin/vote-campaigns", s.feature(s.c.EnableAdminPanel, "Administration", s.adminVoteCampaigns))
	m.HandleFunc("POST /api/admin/vote-campaigns/{id}/draw", s.feature(s.c.EnableAdminPanel, "Administration", s.drawVoteCampaign))
	m.HandleFunc("GET /api/admin/missions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPlayerMissions))
	m.HandleFunc("POST /api/admin/missions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPlayerMissions))
	m.HandleFunc("PUT /api/admin/missions/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPlayerMission))
	m.HandleFunc("DELETE /api/admin/missions/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPlayerMission))
	m.HandleFunc("GET /api/admin/downloads", s.feature(s.c.EnableAdminPanel, "Administration", s.adminDownloads))
	m.HandleFunc("POST /api/admin/downloads", s.feature(s.c.EnableAdminPanel, "Administration", s.adminDownloads))
	m.HandleFunc("DELETE /api/admin/downloads/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminDownloadDelete))
	m.HandleFunc("GET /api/admin/launcher-patches", s.feature(s.c.EnableAdminPanel, "Administration", s.adminLauncherPatches))
	m.HandleFunc("POST /api/admin/launcher-patches", s.feature(s.c.EnableAdminPanel, "Administration", s.adminLauncherPatches))
	m.HandleFunc("DELETE /api/admin/launcher-patches/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminLauncherPatchDelete))
	m.HandleFunc("POST /api/admin/news", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNews))
	m.HandleFunc("PUT /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("DELETE /api/admin/news/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNewsItem))
	m.HandleFunc("GET /api/admin/pages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPages))
	m.HandleFunc("POST /api/admin/pages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPages))
	m.HandleFunc("PUT /api/admin/pages/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPageItem))
	m.HandleFunc("DELETE /api/admin/pages/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPageItem))
	m.HandleFunc("GET /api/admin/events", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEvents))
	m.HandleFunc("POST /api/admin/events", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEvents))
	m.HandleFunc("PUT /api/admin/events/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventItem))
	m.HandleFunc("DELETE /api/admin/events/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventItem))
	m.HandleFunc("GET /api/admin/events/{id}/participants", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventParticipants))
	m.HandleFunc("PUT /api/admin/events/{id}/participants/{account}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventParticipantStatus))
	m.HandleFunc("POST /api/admin/events/{id}/rewards", s.feature(s.c.EnableAdminPanel, "Administration", s.adminEventRewards))
	m.HandleFunc("GET /api/admin/community/issues", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCommunityIssues))
	m.HandleFunc("PUT /api/admin/community/issues/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCommunityIssue))
	m.HandleFunc("GET /api/admin/guild-recruitment", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildRecruitment))
	m.HandleFunc("POST /api/admin/guild-recruitment", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildRecruitment))
	m.HandleFunc("GET /api/admin/guild-applications", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildApplications))
	m.HandleFunc("PUT /api/admin/guild-applications/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGuildApplication))
	m.HandleFunc("GET /api/admin/arena-seasons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminArenaSeasons))
	m.HandleFunc("POST /api/admin/arena-seasons", s.feature(s.c.EnableAdminPanel, "Administration", s.adminArenaSeasons))
	m.HandleFunc("GET /api/admin/ranking-exclusions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRankingExclusions))
	m.HandleFunc("POST /api/admin/ranking-exclusions", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRankingExclusions))
	m.HandleFunc("DELETE /api/admin/ranking-exclusions/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRankingExclusionDelete))
	m.HandleFunc("GET /api/admin/shop/collections", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollections))
	m.HandleFunc("POST /api/admin/shop/collections", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollections))
	m.HandleFunc("PUT /api/admin/shop/collections/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollection))
	m.HandleFunc("DELETE /api/admin/shop/collections/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminShopCollection))
	m.HandleFunc("GET /api/admin/shop/stock", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStock))
	m.HandleFunc("POST /api/admin/shop/stock", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStock))
	m.HandleFunc("GET /api/admin/shop/bundles", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplates))
	m.HandleFunc("POST /api/admin/shop/bundles", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplates))
	m.HandleFunc("PUT /api/admin/shop/bundles/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplateUpdate))
	m.HandleFunc("DELETE /api/admin/shop/bundles/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminBundleTemplateDelete))
	m.HandleFunc("GET /api/admin/raid-eligibility", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRaidEligibilityRules))
	m.HandleFunc("PUT /api/admin/raid-eligibility", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRaidEligibilityRules))
	m.HandleFunc("GET /api/admin/resources", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTools))
	m.HandleFunc("POST /api/admin/resources", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTools))
	m.HandleFunc("PUT /api/admin/resources/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminToolItem))
	m.HandleFunc("DELETE /api/admin/resources/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminToolItem))
	m.HandleFunc("GET /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("PUT /api/admin/settings", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSettings))
	m.HandleFunc("GET /api/admin/realm-config", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRealmConfig))
	m.HandleFunc("POST /api/admin/realm-config/apply", s.feature(s.c.EnableAdminPanel, "Administration", s.adminRealmConfig))
	m.HandleFunc("GET /api/admin/accounts", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAccounts))
	m.HandleFunc("POST /api/admin/accounts/{id}/revoke-sessions", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Hour, s.adminRevokeAccountSessions)))
	m.HandleFunc("POST /api/admin/accounts/{id}/require-password-reset", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(10, time.Hour, s.adminRequirePasswordReset)))
	m.HandleFunc("POST /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Minute, s.adminModeration)))
	m.HandleFunc("GET /api/admin/moderation", s.feature(s.c.EnableAdminPanel, "Administration", s.adminModerationLog))
	m.HandleFunc("GET /api/admin/sanction-appeals", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSanctionAppeals))
	m.HandleFunc("PUT /api/admin/sanction-appeals/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminSanctionAppeal))
	m.HandleFunc("GET /api/admin/investigations/policy", s.feature(s.c.EnableAdminPanel, "Administration", s.adminInvestigationPolicy))
	m.HandleFunc("POST /api/admin/investigations/search", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(20, time.Hour, s.adminInvestigationSearch)))
	m.HandleFunc("POST /api/admin/investigations/evidence", s.feature(s.c.EnableAdminPanel, "Administration", s.rate(30, time.Hour, s.adminInvestigationEvidence)))
	m.HandleFunc("GET /api/admin/audit", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAudit))
	m.HandleFunc("GET /api/admin/media", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMedia))
	m.HandleFunc("POST /api/admin/media", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMediaUpload))
	m.HandleFunc("PATCH /api/admin/media/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMediaUpdate))
	m.HandleFunc("DELETE /api/admin/media/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminMediaDelete))
	m.HandleFunc("GET /api/admin/navigation", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigation))
	m.HandleFunc("POST /api/admin/navigation", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigationCreate))
	m.HandleFunc("PUT /api/admin/navigation/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigationUpdate))
	m.HandleFunc("DELETE /api/admin/navigation/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminNavigationDelete))
	m.HandleFunc("GET /api/admin/audit/export", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditExport))
	m.HandleFunc("GET /api/admin/audit/filters", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditFilters))
	m.HandleFunc("POST /api/admin/audit/filters", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditFilters))
	m.HandleFunc("DELETE /api/admin/audit/filters/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminAuditFilterDelete))
	m.HandleFunc("GET /api/admin/staff", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStaff))
	m.HandleFunc("POST /api/admin/staff", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStaff))
	m.HandleFunc("DELETE /api/admin/staff/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminStaffDelete))
	m.HandleFunc("GET /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.adminConsoleHistory))
	m.HandleFunc("POST /api/admin/console", s.feature(s.c.EnableAdminPanel && s.c.EnableGMConsole, "GM console", s.rate(20, time.Minute, s.adminConsoleExecute)))
	m.HandleFunc("GET /api/admin/tickets", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminTickets))
	m.HandleFunc("POST /api/admin/tickets/{id}", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminTicketUpdate))
	m.HandleFunc("GET /api/admin/transfers", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTransfers))
	m.HandleFunc("POST /api/admin/transfers/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminTransferUpdate))
	m.HandleFunc("GET /api/admin/tickets/{id}/events", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminTicketEvents))
	m.HandleFunc("GET /api/admin/canned-replies", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminCannedReplies))
	m.HandleFunc("POST /api/admin/canned-replies", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminCannedReplies))
	m.HandleFunc("DELETE /api/admin/canned-replies/{id}", s.feature(s.c.EnableAdminPanel && s.c.EnableSupport, "Administration", s.adminCannedReplyDelete))
	m.HandleFunc("POST /api/billing/checkout", s.feature(s.c.EnableShop, "Shop", s.billingCheckout))
	m.HandleFunc("POST /api/gift-codes/redeem", s.rate(10, time.Hour, s.redeemGiftCode))
	m.HandleFunc("GET /api/billing/packages", s.feature(s.c.EnableShop, "Shop", s.billingPackages))
	m.HandleFunc("GET /api/billing/transactions", s.feature(s.c.EnableShop, "Shop", s.paymentTransactions))
	m.HandleFunc("GET /api/admin/credit-packages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCreditPackages))
	m.HandleFunc("POST /api/admin/credit-packages", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCreditPackages))
	m.HandleFunc("DELETE /api/admin/credit-packages/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminCreditPackageDelete))
	m.HandleFunc("GET /api/admin/payments", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPayments))
	m.HandleFunc("POST /api/admin/payments/{id}/refund", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPaymentRefund))
	m.HandleFunc("GET /api/admin/gift-codes", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGiftCodes))
	m.HandleFunc("POST /api/admin/gift-codes", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGiftCodes))
	m.HandleFunc("DELETE /api/admin/gift-codes/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminGiftCodeDelete))
	m.HandleFunc("POST /api/billing/webhook", s.billingWebhook)
	m.HandleFunc("GET /api/security/sessions", s.securitySessions)
	m.HandleFunc("DELETE /api/security/sessions/{id}", s.securityRevokeSession)
	m.HandleFunc("POST /api/security/password", s.securityPassword)
	m.HandleFunc("POST /api/security/email", s.securityEmail)
	m.HandleFunc("POST /api/security/totp/setup", s.securityTOTPSetup)
	m.HandleFunc("POST /api/security/totp/enable", s.securityTOTPEnable)
	m.HandleFunc("POST /api/security/totp/disable", s.securityTOTPDisable)
	m.HandleFunc("GET /api/security/totp/status", s.securityTOTPStatus)
	m.HandleFunc("POST /api/security/step-up", s.rate(10, 10*time.Minute, s.securityStepUp))
	m.HandleFunc("GET /api/privacy/export", s.privacyExport)
	m.HandleFunc("GET /api/privacy/requests", s.privacyRequests)
	m.HandleFunc("POST /api/privacy/deletion", s.rate(3, 24*time.Hour, s.privacyDeletionRequest))
	m.HandleFunc("DELETE /api/privacy/requests/{id}", s.privacyRequestCancel)
	m.HandleFunc("GET /api/admin/privacy-requests", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPrivacyRequests))
	m.HandleFunc("POST /api/admin/privacy-requests/{id}", s.feature(s.c.EnableAdminPanel, "Administration", s.adminPrivacyRequestUpdate))
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
		start := time.Now()
		rw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		requestID := validRequestID(r.Header.Get("X-Request-ID"))
		if requestID == "" {
			requestID = newRequestID()
		}
		w.Header().Set("X-Request-ID", requestID)
		if strings.HasPrefix(r.URL.Path, "/api/") {
			w.Header().Set("X-Portal-API-Version", "1")
		}
		r = r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, requestID))
		defer func() {
			s.recordAdministrativeRequest(r, rw.status)
			s.metrics.requests.Add(1)
			if rw.status >= 500 {
				s.metrics.errors.Add(1)
			}
			if rw.status >= 400 && r.URL.Path == "/api/auth/login" {
				s.metrics.loginFailures.Add(1)
			}
			if rw.status >= 400 && r.URL.Path == "/api/billing/webhook" {
				s.metrics.webhookFailures.Add(1)
			}
			slog.Info("request", "method", r.Method, "path", r.URL.Path, "status", rw.status, "latency_ms", time.Since(start).Milliseconds(), "ip", s.clientIP(r), "realm", s.c.RealmKey, "request_id", requestID)
		}()
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		csp := "default-src 'self' blob:; img-src 'self' https: data: blob:; style-src 'self' 'unsafe-inline' https://wow.zamimg.com; script-src 'self' https://code.jquery.com https://wow.zamimg.com https://challenges.cloudflare.com; connect-src 'self' https://nether.wowhead.com https://wow.zamimg.com https://challenges.cloudflare.com; frame-src https://challenges.cloudflare.com; worker-src 'self' blob:"
		if analyticsURL, err := url.Parse(s.c.AnalyticsScriptURL); err == nil && analyticsURL.Scheme == "https" && analyticsURL.Host != "" {
			origin := analyticsURL.Scheme + "://" + analyticsURL.Host
			csp = strings.Replace(csp, "; connect-src", " "+origin+"; connect-src", 1)
			csp = strings.Replace(csp, "; frame-src", " "+origin+"; frame-src", 1)
		}
		w.Header().Set("Content-Security-Policy", csp)
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		if s.c.CookieSecure {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !s.sameOrigin(r) {
			problem(rw, http.StatusForbidden, "Invalid request origin")
			return
		}
		sensitiveMutation := strings.HasPrefix(r.URL.Path, "/api/admin/") || strings.HasPrefix(r.URL.Path, "/api/identity/") || strings.HasPrefix(r.URL.Path, "/api/security/passkeys/")
		if r.Method != http.MethodGet && r.Method != http.MethodHead && sensitiveMutation && !s.adminTokenValid(r) && s.hasAuthenticatedSession(r) && !s.stepUpValid(r) {
			problem(rw, http.StatusPreconditionRequired, "Confirm your password and authenticator code to continue")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && !strings.HasPrefix(r.URL.Path, "/api/admin/") && r.URL.Path != "/api/auth/login" && r.URL.Path != "/api/auth/logout" && r.URL.Path != "/api/billing/webhook" {
			if active, message := s.maintenanceActive(r); active {
				if _, gm := s.requireGM(r); !gm {
					if strings.TrimSpace(message) == "" {
						message = "Scheduled maintenance is in progress"
					}
					problem(rw, http.StatusServiceUnavailable, message)
					return
				}
			}
		}
		next.ServeHTTP(rw, r)
	})
}

func (s *Server) recordAdministrativeRequest(r *http.Request, status int) {
	if s.c.MockMode || s.s == nil || s.s.Auth == nil || status >= http.StatusBadRequest || (r.Method == http.MethodGet || r.Method == http.MethodHead) || !strings.HasPrefix(r.URL.Path, "/api/admin/") {
		return
	}
	actor, err := s.trackerAccount(r)
	if err != nil {
		return
	}
	metadata, _ := json.Marshal(map[string]any{"method": r.Method, "path": r.URL.Path, "status": status})
	_, err = s.s.Auth.ExecContext(context.WithoutCancel(r.Context()), `INSERT INTO portal_admin_audit(actor_account_id,action,target,details,realm_key,request_id,ip_address,user_agent,metadata_json) VALUES(?,?,?,'Administrative API request',?,?,?,?,?)`, actor.ID, "admin."+strings.ToLower(r.Method), r.URL.Path, s.c.RealmKey, RequestID(r.Context()), s.clientIP(r), truncate(r.UserAgent(), 500), metadata)
	if err != nil {
		slog.Error("record structured admin audit", "path", r.URL.Path, "request_id", RequestID(r.Context()), "error", err)
	}
}

func (s *Server) adminTokenValid(r *http.Request) bool {
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	return s.c.AdminToken != "" && len(provided) == len(s.c.AdminToken) && subtle.ConstantTimeCompare([]byte(provided), []byte(s.c.AdminToken)) == 1
}

func (s *Server) hasAuthenticatedSession(r *http.Request) bool {
	if s.c.MockMode {
		_, ok := s.mockUser(r)
		return ok
	}
	_, err := s.auth(r)
	return err == nil
}

type requestIDContextKey struct{}

func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDContextKey{}).(string)
	return id
}

func validRequestID(value string) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > 64 {
		return ""
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' && r != '.' {
			return ""
		}
	}
	return value
}

func newRequestID() string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err == nil {
		return hex.EncodeToString(raw)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
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
	database, databaseLatency := s.databaseChecks(r.Context())
	dbOK := database["auth"] && database["characters"] && database["world"]
	realmOnline := false
	if database["auth"] && database["characters"] {
		q := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s`.uptime WHERE realmid=? AND starttime+uptime>=UNIX_TIMESTAMP()-900)", s.c.AuthDB)
		_ = s.s.Auth.QueryRowContext(r.Context(), q, s.c.RealmID).Scan(&realmOnline)
		if !realmOnline {
			cq := fmt.Sprintf("SELECT EXISTS(SELECT 1 FROM `%s`.characters WHERE online=1 AND deleteDate IS NULL)", s.c.CharactersDB)
			_ = s.s.Characters.QueryRowContext(r.Context(), cq).Scan(&realmOnline)
		}
	}
	now := time.Now()
	maintenance := cfg.MaintenanceEnabled && (cfg.MaintenanceStarts == nil || !now.Before(*cfg.MaintenanceStarts)) && (cfg.MaintenanceEnds == nil || now.Before(*cfg.MaintenanceEnds))
	lastDBSuccess := time.Unix(s.metrics.dbLastSuccessUnix.Load(), 0)
	lastSOAPSuccess := time.Unix(s.metrics.soapLastSuccessUnix.Load(), 0)
	dependencies := []map[string]any{
		{"name": "Portal", "configured": true, "reachable": true, "authorized": true, "detail": "HTTP service is responding"},
		{"name": "Authentication database", "configured": true, "reachable": database["auth"], "authorized": database["auth"], "latencyMs": databaseLatency["auth"].Milliseconds()},
		{"name": "Character database", "configured": true, "reachable": database["characters"], "authorized": database["characters"], "latencyMs": databaseLatency["characters"].Milliseconds()},
		{"name": "World database", "configured": true, "reachable": database["world"], "authorized": database["world"], "latencyMs": databaseLatency["world"].Milliseconds()},
		{"name": "AzerothCore SOAP", "configured": s.soap.Enabled(), "reachable": nil, "authorized": nil, "lastSuccess": optionalMetricTime(lastSOAPSuccess), "detail": "Reachability is observed during delivery; no mutating probe is sent"},
		{"name": "SMTP", "configured": s.c.SMTPAddr != "" && s.c.SMTPFrom != "", "reachable": nil, "authorized": nil, "detail": "Reachability is observed when mail is sent"},
		{"name": "Stripe", "configured": s.c.StripeSecret != "" && s.c.StripeWebhookSecret != "", "reachable": nil, "authorized": nil, "detail": "Checkout and signed webhook credentials"},
		{"name": "Competitive ingestion", "configured": s.c.CompetitiveIngestSecret != "", "reachable": nil, "authorized": nil, "detail": "Authenticated raid and PvP event receiver"},
		{"name": "Realm configuration agent", "configured": s.c.RealmAgentURL != "", "reachable": nil, "authorized": nil, "detail": "Allow-listed configuration reconciliation; verified from the Realm configuration screen"},
	}
	return map[string]any{"online": realmOnline, "realm": cfg.RealmName, "address": cfg.RealmAddress, "shopDelivery": s.soap.Enabled(), "portal": true, "database": dbOK, "databases": database, "databaseLatencyMs": map[string]int64{"auth": databaseLatency["auth"].Milliseconds(), "characters": databaseLatency["characters"].Milliseconds(), "world": databaseLatency["world"].Milliseconds()}, "databaseLastSuccess": optionalMetricTime(lastDBSuccess), "soapConfigured": s.soap.Enabled(), "deliveryDiagnostic": map[string]any{"configured": strings.TrimSpace(s.c.DeliveryDiagnosticCharacter) != "", "character": s.c.DeliveryDiagnosticCharacter, "itemId": deliveryDiagnosticItemID, "requiresOffline": true}, "dependencies": dependencies, "maintenance": maintenance, "maintenanceMessage": cfg.MaintenanceMessage, "checkedAt": now}
}

func optionalMetricTime(value time.Time) any {
	if value.Unix() <= 0 {
		return nil
	}
	return value.UTC().Format(time.RFC3339)
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
	identityResult, err := tx.ExecContext(r.Context(), `INSERT INTO portal_identities(email,display_name) VALUES(?,?)`, in.Email, in.Username)
	if err != nil {
		problem(w, 500, "Could not initialize master account")
		return
	}
	identityID, _ := identityResult.LastInsertId()
	if _, err = tx.ExecContext(r.Context(), `INSERT INTO portal_identity_accounts(identity_id,account_id,label,is_primary) VALUES(?,?,?,1)`, identityID, id, in.Username); err != nil {
		problem(w, 500, "Could not link game account")
		return
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
	var salt, verifier, storedTOTP []byte
	var totpEnabled bool
	q := fmt.Sprintf("SELECT a.id,a.username,a.email,a.salt,a.verifier,COALESCE(ps.totp_secret,''),COALESCE(ps.totp_enabled,0) FROM `%s`.account a LEFT JOIN portal_account_security ps ON ps.account_id=a.id WHERE username=? AND locked=0 AND NOT EXISTS (SELECT 1 FROM `%s`.account_banned b WHERE b.id=a.id AND b.active=1)", s.c.AuthDB, s.c.AuthDB)
	if err := s.s.Auth.QueryRowContext(r.Context(), q, in.Username).Scan(&a.ID, &a.Username, &a.Email, &salt, &verifier, &storedTOTP, &totpEnabled); err != nil || !srp.Verify(a.Username, in.Password, salt, verifier) {
		problem(w, 401, "Invalid username or password")
		return
	}
	var forcedReset bool
	if err := s.s.Auth.QueryRowContext(r.Context(), "SELECT EXISTS(SELECT 1 FROM portal_forced_password_resets WHERE account_id=?)", a.ID).Scan(&forcedReset); err != nil {
		problem(w, http.StatusInternalServerError, "Could not verify account security state")
		return
	}
	if forcedReset {
		problem(w, http.StatusForbidden, "A staff-required password reset is pending. Use the link sent to your email or request a new one.")
		return
	}
	if totpEnabled {
		secret, decryptErr := s.decryptTOTP(storedTOTP)
		valid := decryptErr == nil && validTOTP(secret, in.OTP, time.Now())
		if !valid {
			valid = s.consumeRecoveryCode(r.Context(), a.ID, in.OTP)
		}
		if !valid {
			problem(w, 401, "A valid authenticator or recovery code is required")
			return
		}
		if decryptErr == nil && !strings.HasPrefix(string(storedTOTP), "v1:") && len(s.c.TOTPEncryptionKey) == 32 {
			if encrypted, encryptErr := s.encryptTOTP(secret); encryptErr == nil {
				_, _ = s.s.Auth.ExecContext(r.Context(), "UPDATE portal_account_security SET totp_secret=? WHERE account_id=?", encrypted, a.ID)
			}
		}
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
	var activeSessions, recognizedSessions uint32
	_ = s.s.Auth.QueryRowContext(r.Context(), "SELECT COUNT(*),COALESCE(SUM(ip_address=? AND user_agent=?),0) FROM portal_sessions WHERE account_id=? AND expires_at>NOW()", ip, ua, a.ID).Scan(&activeSessions, &recognizedSessions)
	identityID, err := s.ensureIdentity(r.Context(), a.ID, a.Username, a.Email)
	if err != nil {
		problem(w, 500, "Could not load master account")
		return
	}
	if _, err := s.s.Auth.ExecContext(r.Context(), "INSERT INTO portal_sessions (token_hash,account_id,identity_id,expires_at,ip_address,user_agent) VALUES (?,?,?,?,?,?)", hash[:], a.ID, identityID, expires, ip, ua); err != nil {
		problem(w, 500, "Could not create session")
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "portal_session", Value: token, Path: "/", Expires: expires, MaxAge: 604800, HttpOnly: true, Secure: s.c.CookieSecure, SameSite: http.SameSiteLaxMode})
	if activeSessions > 0 && recognizedSessions == 0 {
		browser := strings.TrimSpace(ua)
		if browser == "" {
			browser = "an unidentified client"
		}
		if len(browser) > 90 {
			browser = browser[:90] + "…"
		}
		s.notifyAccount(r.Context(), a.ID, "security", "New sign-in detected", "A new session signed in from "+ip+" using "+browser+".", "/account/security")
	}
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
	role, permissions := s.effectiveStaff(r.Context(), a)
	jsonOut(w, 200, map[string]any{"account": a, "balance": balance, "staffRole": role, "permissions": permissions})
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
	q := fmt.Sprintf(`SELECT c.guid,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,COALESCE(g.name,''),COALESCE(g.guildid,0) FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.deleteDate IS NULL AND c.name LIKE ? ORDER BY c.level DESC,c.name LIMIT 24`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	rows, e := s.s.Characters.QueryContext(r.Context(), q, "%"+term+"%")
	if e != nil {
		problem(w, 500, "Could not search armory")
		return
	}
	defer rows.Close()
	out := []character{}
	hidden := s.hiddenCharacterGUIDs(r)
	for rows.Next() {
		var c character
		if rows.Scan(&c.GUID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Guild, &c.GuildID) == nil {
			if !hidden[c.GUID] {
				out = append(out, c)
			}
		}
	}
	jsonOut(w, 200, map[string]any{"characters": out})
}

func (s *Server) armoryCharacter(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	var c character
	var ownerID uint32
	q := fmt.Sprintf(`SELECT c.guid,c.account,c.name,c.race,c.class,c.gender,c.level,c.zone,c.online,c.totaltime,COALESCE(g.name,''),COALESCE(g.guildid,0) FROM %s.characters c LEFT JOIN %s.guild_member gm ON gm.guid=c.guid LEFT JOIN %s.guild g ON g.guildid=gm.guildid WHERE c.name=? AND c.deleteDate IS NULL`, s.c.CharactersDB, s.c.CharactersDB, s.c.CharactersDB)
	if e := s.s.Characters.QueryRowContext(r.Context(), q, name).Scan(&c.GUID, &ownerID, &c.Name, &c.Race, &c.Class, &c.Gender, &c.Level, &c.Zone, &c.Online, &c.TotalTime, &c.Guild, &c.GuildID); e != nil {
		problem(w, 404, "Character not found")
		return
	}
	privacy, visible := s.armoryPrivacy(r, c.GUID, ownerID)
	if !visible {
		problem(w, http.StatusNotFound, "Character not found")
		return
	}
	items := []armoryItem{}
	if privacy.ShowGear {
		iq := fmt.Sprintf(`SELECT ci.slot,ii.itemEntry,it.name,it.Quality,it.displayid,it.ItemLevel,it.RequiredLevel,it.armor,it.InventoryType,it.itemset,ii.enchantments,ii.durability,it.MaxDurability,it.stat_type1,it.stat_value1,it.stat_type2,it.stat_value2,it.stat_type3,it.stat_value3,it.stat_type4,it.stat_value4,it.stat_type5,it.stat_value5,it.stat_type6,it.stat_value6,it.stat_type7,it.stat_value7,it.stat_type8,it.stat_value8,it.stat_type9,it.stat_value9,it.stat_type10,it.stat_value10 FROM %s.character_inventory ci JOIN %s.item_instance ii ON ii.guid=ci.item JOIN %s.item_template it ON it.entry=ii.itemEntry WHERE ci.guid=? AND ci.bag=0 AND ci.slot<19 ORDER BY ci.slot`, s.c.CharactersDB, s.c.CharactersDB, s.c.WorldDB)
		rows, e := s.s.Characters.QueryContext(r.Context(), iq, c.GUID)
		if e == nil {
			for rows.Next() {
				var i armoryItem
				var statTypes, statValues [10]int16
				if rows.Scan(&i.Slot, &i.Entry, &i.Name, &i.Quality, &i.DisplayID, &i.ItemLevel, &i.RequiredLevel, &i.Armor, &i.InventoryType, &i.SetID, &i.Enchantments, &i.Durability, &i.MaxDurability,
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
			rows.Close()
		}
	}
	s.enrichItemEnhancements(r.Context(), items)
	jsonOut(w, 200, map[string]any{"character": c, "equipment": items, "itemSets": s.loadEquippedItemSets(r.Context(), items), "profile": s.loadCharacterProfile(r.Context(), c.GUID), "arenaTeams": s.characterArenaTeams(r, c.GUID), "privacy": privacy})
}

func (s *Server) shop(w http.ResponseWriter, r *http.Request) {
	rows, e := s.s.Auth.QueryContext(r.Context(), "SELECT id,name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount,service_action,active,starts_at,ends_at,per_account_limit,featured,sale_price,stock_limit,sold_count,category_order,tags,visibility_segment,variant_required,bundle_template_id FROM portal_products WHERE realm_key=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) ORDER BY featured DESC,category_order,category,class_id,price,name", s.c.RealmKey)
	if e != nil {
		problem(w, 500, "Could not load shop")
		return
	}
	defer rows.Close()
	out := []product{}
	for rows.Next() {
		var p product
		if rows.Scan(&p.ID, &p.Name, &p.Description, &p.ItemID, &p.Quantity, &p.Price, &p.Category, &p.ImageURL, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold, &p.ServiceAction, &p.Active, &p.StartsAt, &p.EndsAt, &p.PerAccountLimit, &p.Featured, &p.SalePrice, &p.StockLimit, &p.SoldCount, &p.CategoryOrder, &p.Tags, &p.Visibility, &p.VariantRequired, &p.BundleID) == nil {
			out = append(out, p)
		}
	}
	rows.Close()
	out = s.filterProductsForAudience(r, out)
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
	for index := range out {
		if out[index].BundleID == 0 {
			continue
		}
		bundleItems, bundleErr := s.loadBundleItems(r.Context(), out[index].BundleID)
		if bundleErr != nil {
			problem(w, 500, "Could not load reusable product bundle")
			return
		}
		out[index].Items = append(out[index].Items, bundleItems...)
		for _, item := range bundleItems {
			out[index].Includes = append(out[index].Includes, fmt.Sprintf("%d × %s", item.Quantity, item.Name))
		}
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
	if e = s.loadProductMerchandising(r.Context(), out); e != nil {
		problem(w, http.StatusInternalServerError, "Could not load shop merchandising")
		return
	}
	collections, _ := s.listShopCollections(r.Context(), false)
	jsonOut(w, 200, map[string]any{"products": out, "collections": collections, "deliveryEnabled": s.soap.Enabled()})
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
		VariantID                uint64 `json:"variantId"`
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
	if e = tx.QueryRowContext(r.Context(), "SELECT id,name,item_id,quantity,price,category,class_id,tier_label,service_level,gold_amount,service_action,per_account_limit,featured,sale_price,stock_limit,sold_count,category_order,variant_required,bundle_template_id FROM portal_products WHERE id=? AND realm_key=? AND active=1 AND (starts_at IS NULL OR starts_at<=NOW()) AND (ends_at IS NULL OR ends_at>NOW()) FOR UPDATE", in.ProductID, s.c.RealmKey).Scan(&p.ID, &p.Name, &p.ItemID, &p.Quantity, &p.Price, &p.Category, &p.ClassID, &p.Tier, &p.ServiceLevel, &p.Gold, &p.ServiceAction, &p.PerAccountLimit, &p.Featured, &p.SalePrice, &p.StockLimit, &p.SoldCount, &p.CategoryOrder, &p.VariantRequired, &p.BundleID); e != nil {
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
	var variantName string
	var variantAdjustment int32
	if in.VariantID > 0 {
		if e = tx.QueryRowContext(r.Context(), `SELECT name,price_adjustment FROM portal_product_variants WHERE id=? AND product_id=? AND active=1`, in.VariantID, p.ID).Scan(&variantName, &variantAdjustment); e != nil {
			problem(w, http.StatusUnprocessableEntity, "Choose a valid product variant")
			return
		}
	} else if p.VariantRequired {
		problem(w, http.StatusUnprocessableEntity, "Choose a product variant")
		return
	}
	adjustedPrice := int64(basePrice) + int64(variantAdjustment)
	if adjustedPrice < 0 || adjustedPrice > 10_000_000 {
		problem(w, http.StatusUnprocessableEntity, "Variant price is invalid")
		return
	}
	basePrice = uint32(adjustedPrice)
	discount, couponID, couponCode, e := s.applyCoupon(r, tx, a.ID, in.Coupon, basePrice, p.SalePrice > 0, p.Category)
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
	res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_orders(account_id,character_guid,realm_key,product_id,item_id,quantity,total,subtotal,discount,coupon_code,status,service_level,gold_amount,service_action,variant_id,variant_name) VALUES(?,?,?,?,?,?,?,?,?,?,'pending',?,?,?,?,?)", a.ID, in.CharacterGUID, s.c.RealmKey, p.ID, p.ItemID, p.Quantity, total, basePrice, discount, couponCode, p.ServiceLevel, p.Gold, p.ServiceAction, in.VariantID, variantName)
	if e != nil {
		problem(w, 500, "Could not create order")
		return
	}
	orderID, _ := res.LastInsertId()
	if _, e = tx.ExecContext(r.Context(), "INSERT INTO portal_credit_ledger(actor_account_id,target_account_id,amount,reason) VALUES(?,?,?,?)", a.ID, a.ID, -int64(total), "Order "+strconv.FormatInt(orderID, 10)+" purchase"); e != nil {
		problem(w, 500, "Could not record wallet transaction")
		return
	}
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
	if p.BundleID > 0 {
		bundleItems, bundleErr := s.loadBundleItems(r.Context(), p.BundleID)
		if bundleErr != nil {
			problem(w, 500, "Could not load reusable product bundle")
			return
		}
		items = append(items, bundleItems...)
	}
	if in.VariantID > 0 {
		variantRows, variantErr := tx.QueryContext(r.Context(), `SELECT item_id,quantity FROM portal_product_variant_items WHERE variant_id=? ORDER BY item_id`, in.VariantID)
		if variantErr != nil {
			problem(w, 500, "Could not load variant items")
			return
		}
		variantItems := []bundleItem{}
		for variantRows.Next() {
			var item bundleItem
			if variantRows.Scan(&item.ItemID, &item.Quantity) == nil {
				variantItems = append(variantItems, item)
			}
		}
		variantRows.Close()
		if len(variantItems) > 0 {
			items = variantItems
		}
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
	items = consolidateBundleItems(items)
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
		if _, e = tx.ExecContext(r.Context(), `INSERT INTO portal_stock_movements(realm_key,product_id,quantity_delta,movement_type,reference_id,reason,actor_account_id) VALUES(?,?,?,'sale',?,'Order reserved',?)`, s.c.RealmKey, p.ID, -1, strconv.FormatInt(orderID, 10), a.ID); e != nil {
			problem(w, 500, "Could not record stock movement")
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
	rows, e := s.s.Auth.QueryContext(r.Context(), `SELECT o.id,o.item_id,o.quantity,o.total,o.status,o.created_at,COALESCE(p.name,'')
		FROM portal_orders o LEFT JOIN portal_products p ON p.id=o.product_id
		WHERE o.account_id=? AND o.realm_key=? ORDER BY o.id DESC LIMIT 50`, a.ID, s.c.RealmKey)
	if e != nil {
		problem(w, 500, "Could not load orders")
		return
	}
	defer rows.Close()
	type order struct {
		ID       uint64    `json:"id"`
		ItemID   uint32    `json:"itemId"`
		Quantity uint32    `json:"quantity"`
		Total    uint32    `json:"total"`
		Status   string    `json:"status"`
		Created  time.Time `json:"created"`
		Product  string    `json:"product"`
	}
	out := []order{}
	for rows.Next() {
		var o order
		if rows.Scan(&o.ID, &o.ItemID, &o.Quantity, &o.Total, &o.Status, &o.Created, &o.Product) == nil {
			out = append(out, o)
		}
	}
	jsonOut(w, 200, map[string]any{"orders": out})
}

func (s *Server) wallet(w http.ResponseWriter, r *http.Request) {
	a, err := s.auth(r)
	if err != nil {
		problem(w, http.StatusUnauthorized, "Sign in required")
		return
	}
	var balance uint32
	if err = s.s.Auth.QueryRowContext(r.Context(), "SELECT balance FROM portal_wallets WHERE account_id=?", a.ID).Scan(&balance); err != nil {
		problem(w, http.StatusInternalServerError, "Could not load wallet")
		return
	}
	type transaction struct {
		ID      uint64    `json:"id"`
		Amount  int64     `json:"amount"`
		Reason  string    `json:"reason"`
		Created time.Time `json:"created"`
	}
	rows, err := s.s.Auth.QueryContext(r.Context(), "SELECT id,amount,reason,created_at FROM portal_credit_ledger WHERE target_account_id=? ORDER BY id DESC LIMIT 100", a.ID)
	if err != nil {
		problem(w, http.StatusInternalServerError, "Could not load wallet history")
		return
	}
	defer rows.Close()
	entries := []transaction{}
	for rows.Next() {
		var entry transaction
		if err = rows.Scan(&entry.ID, &entry.Amount, &entry.Reason, &entry.Created); err != nil {
			problem(w, http.StatusInternalServerError, "Could not read wallet history")
			return
		}
		entries = append(entries, entry)
	}
	if err = rows.Err(); err != nil {
		problem(w, http.StatusInternalServerError, "Could not read wallet history")
		return
	}
	jsonOut(w, http.StatusOK, map[string]any{"balance": balance, "transactions": entries})
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
	p.Tags, p.Visibility = strings.TrimSpace(p.Tags), strings.ToLower(strings.TrimSpace(p.Visibility))
	if p.Visibility == "" {
		p.Visibility = "all"
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
	res, e := tx.ExecContext(r.Context(), "INSERT INTO portal_products(name,description,item_id,quantity,price,category,image_url,class_id,tier_label,service_level,gold_amount,service_action,active,starts_at,ends_at,per_account_limit,realm_key,featured,sale_price,stock_limit,category_order,tags,visibility_segment,variant_required,bundle_template_id) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,1,?,?,?,?,?,?,?,?,?,?,?,?)", p.Name, p.Description, p.ItemID, p.Quantity, p.Price, p.Category, p.ImageURL, p.ClassID, p.Tier, p.ServiceLevel, p.Gold, p.ServiceAction, p.StartsAt, p.EndsAt, p.PerAccountLimit, s.c.RealmKey, p.Featured, p.SalePrice, p.StockLimit, p.CategoryOrder, p.Tags, p.Visibility, p.VariantRequired, p.BundleID)
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
	if e = saveProductVariants(r.Context(), tx, uint32(id), p.Variants); e != nil {
		problem(w, 500, "Could not create product variants")
		return
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
		if _, exists := s.limiter.hits[key]; !exists && len(s.limiter.hits) >= 10000 {
			allowed = false
		}
		if allowed {
			keep = append(keep, now)
			s.limiter.hits[key] = keep
		}
		s.limiter.mu.Unlock()
		if !allowed {
			s.metrics.rateLimitHits.Add(1)
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
			if strings.HasPrefix(p, "assets/") || strings.HasPrefix(p, "_astro/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else if strings.HasSuffix(p, ".html") {
				w.Header().Set("Cache-Control", "no-cache")
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
		for prefix, entry := range map[string]string{"armory/": "armory/index.html", "guilds/": "guilds/index.html", "account/": "account/index.html", "shop/": "shop/index.html", "news/": "news/index.html", "pages/": "pages/index.html", "tracker/": "tracker/index.html"} {
			if strings.HasPrefix(p, prefix) {
				if data, e := fs.ReadFile(root, entry); e == nil {
					w.Header().Set("Content-Type", "text/html; charset=utf-8")
					_, _ = w.Write(data)
					return
				}
			}
		}
		data, e := fs.ReadFile(root, "404.html")
		if e != nil {
			http.Error(w, "UI not built", 500)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write(data)
	})
}

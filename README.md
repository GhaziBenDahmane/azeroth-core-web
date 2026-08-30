# Azeroth Portal

A production-minded AzerothCore web portal in one container: a Go API serves an embedded Astro frontend and connects to your existing AzerothCore MySQL databases and worldserver SOAP endpoint.

## Included

- AzerothCore-compatible SRP6 account registration and login
- Secure HTTP-only sessions, same-origin checks, request limits, and security headers
- AES-256-GCM encrypted authenticator secrets, single-use recovery codes, and ten-minute staff step-up authentication
- New-device sign-in alerts in the player notification inbox
- Downloadable personal-data export and a tracked deletion/anonymization request workflow
- Public armory search, 19-slot equipment paper-doll, item tooltips/icons, guilds, and play time
- Character-controlled armory privacy plus achievement categories, reputations, titles, mounts, and companions
- 2v2, 3v3, and 5v5 arena team ladders with ratings, records, and rosters
- Immutable 2v2, 3v3, and 5v5 arena season snapshots with public archive selection
- Character ladders for honorable kills, achievements, played time, and level
- PvE raid progression from character achievement dates, including guild-first dates
- Routed account dashboard with characters, wallet ledger, orders, notifications, activity, service history, seven-day rewards, loyalty levels, monthly PvE/PvP/community missions, referrals, and provider-verified voting rewards
- Safe self-service rename, customization, unstuck, and deleted-character restoration through AzerothCore SOAP
- Password changes, TOTP two-factor authentication, and session revocation
- Single-use email password recovery and optional Cloudflare Turnstile registration protection
- Audited GM credit grants authorized from AzerothCore `account_access`
- GM player lookup with active-ban status, audited account bans/unbans, character kicks, portal-session revocation, and administrator-required password resets
- Optional audited GM worldserver console with configurable command prefixes and an explicit unrestricted mode
- Character mutes, IP bans, announcements, MOTD updates, guarded GM-level changes, and realm start/restart/shutdown controls
- Player support ticket conversations with follow-up replies and staff status management
- GM delivery queue/reconciliation view and credit ledger
- Gold bundles, level services, and class-restricted multi-item gear packages
- Queued race-change and faction-change services using AzerothCore's supported character commands
- 51 specialization-aware, full-slot WotLK S6, S7, and T8 loadouts resolved from the installed world database
- Stripe Checkout credit packs with signed, replay-safe webhooks
- Player-visible payment receipts plus idempotent refund/dispute processing and staff refund requests
- Realm-scoped shop with full product CRUD, featured products, sales, bundles, stock limits, category ordering, purchase-history filters, WotLK item autocomplete, equipment/bag preview, scheduling, and limited-use coupons
- Durable queued fulfillment through AzerothCore SOAP, with review/refund controls
- Realm status, faction population, dedicated Play/download/troubleshooting guide, community hub, guild directory, and guild rosters
- Public suggestions and bug tracker with search, filters, voting, discussions, staff responses, labels, priorities, and audited staff triage
- Guild recruitment profiles, player applications, private staff review notes, and applicant-visible decisions
- Database-backed news/announcement editor, live website configuration, scheduled maintenance, and GM-only service monitoring
- Managed client download mirrors with platform, version, release date, requirements, checksum, signature, VirusTotal report, changelog, and troubleshooting information
- Versioned, realm-aware `/api/launcher/manifest` contract for optional desktop launchers and patchers
- Multi-mirror full-client downloads plus checksum-enforced incremental launcher patch graphs
- Realm-event registration, character reservations, staff attendance tracking, and exactly-once credit rewards
- Multi-site voting with provider verification, cooldown countdowns, personal history, monthly rankings, account/network abuse controls, and transparent committed-seed prize draws
- Public WotLK tools hub with a searchable item database, shareable talent planner, and staff-managed addon/WeakAura library
- Tiered staff access for support agents, moderators, and administrators
- Database-backed per-account support, shop-manager, moderator, and administrator roles with AzerothCore GM-level fallback
- Independent shop-manager assignments, CSV exports, bulk order retries, typed confirmations, dashboard charts, and audit filters
- Optional Discord notifications for registrations, purchases, tickets, and moderation actions
- Admin dashboard analytics for accounts, characters, population, tickets, orders, and credits
- Health, readiness, and Prometheus metrics endpoints
- Responsive, dependency-light Astro UI
- One multi-stage Docker image; no Node runtime in production

The portal creates its own `portal_*` tables in `acore_auth`. It never modifies AzerothCore's inventory tables. Shop items are sent by the worldserver itself via in-game mail.

Deployment automation can idempotently create a dedicated AzerothCore account with valid SRP6 credentials before the server starts. Existing credentials are never overwritten:

```bash
BOOTSTRAP_USERNAME=PORTALSOAP \
BOOTSTRAP_PASSWORD='generated-secret' \
BOOTSTRAP_EMAIL=portal-soap@localhost.invalid \
BOOTSTRAP_GM_LEVEL=3 \
BOOTSTRAP_REALM_ID=-1 \
portal bootstrap-account
```

AzerothCore requires administrator security level for every SOAP account. Keep that account machine-only, keep SOAP on the private container network, and do not publish port `7878`.

## Quick start

```bash
cp .env.example .env
# Edit database, realm, SOAP, and public URL settings.
docker compose up --build -d
```

Open `http://localhost:8080`. The application exits early with a useful error if any database cannot be reached.

### Published Docker image

GitHub Actions tests every pull request plus pushes to `main` and version tags. Successful pushes publish multi-architecture images for AMD64 and ARM64 to GitHub Container Registry:

```bash
docker pull ghcr.io/ghazibendahmane/azeroth-core-web:latest
cp .env.example .env
# Configure the database, public URL, branding, and optional setup wizard.
docker compose -f docker-compose.prod.yml up -d
```

Version tags such as `v1.0.0` also publish `1.0.0` and `1.0` image tags. The package initially follows the GitHub repository's visibility. For a private package, authenticate Docker with a GitHub token that has `read:packages`:

```bash
echo "$GHCR_TOKEN" | docker login ghcr.io -u YOUR_GITHUB_USERNAME --password-stdin
```

Docker Hub is not required. If desired later, a second registry login and image tag can be added to the same workflow.

### Production checklist

Before exposing the portal publicly:

1. Use a versioned image such as `:1.0.0`, rather than `:latest`, after testing it against a staging copy of your AzerothCore databases.
2. Set `PUBLIC_URL=https://your-domain` and `COOKIE_SECURE=true`; place a TLS reverse proxy in front of the default `127.0.0.1:8080` binding.
3. Set `TRUST_PROXY=true` only when direct access to port 8080 is blocked and the proxy overwrites client-IP headers.
4. Keep `MOCK_MODE=false`, `ENABLE_SETUP=false`, and `SETUP_TOKEN` empty after onboarding.
5. Leave `ADMIN_TOKEN` empty unless API automation needs it. If enabled, generate it with `openssl rand -hex 32`.
6. Keep MySQL and SOAP private, use the least-privilege database grants below, and use a dedicated restricted SOAP account.
7. Back up the AzerothCore databases before the first migration and before portal upgrades.

For a controlled production rollout, set `AUTO_MIGRATE=false` and run migrations as a separate deployment step before starting the new application version:

```bash
docker compose -f docker-compose.prod.yml run --rm portal migrate
docker compose -f docker-compose.prod.yml up -d
```

With automatic migrations disabled, startup performs a read-only schema-version check and exits with a clear instruction if the database is behind. The migration command uses a MySQL advisory lock, so concurrent deployment jobs cannot apply schema changes simultaneously.

The production Compose file runs the container read-only, drops Linux capabilities, prevents privilege escalation, and binds only to localhost by default. Set `PORTAL_BIND=0.0.0.0` only when an external firewall or container network provides equivalent isolation.

### First-time administrator setup

The optional web wizard creates one AzerothCore account, initializes its portal wallet and realm rows, and grants its GM access through `account_access`. Generate a one-time secret, add it to `.env`, and start the portal:

```bash
openssl rand -hex 32
# Set ENABLE_SETUP=true and SETUP_TOKEN=<generated value> in .env
docker compose up --build -d
```

Open `/setup`. The wizard locks permanently after the first successful administrator creation. It also treats an existing AzerothCore GM account as completed setup, preventing an upgraded installation from being claimed. After setup, set `ENABLE_SETUP=false`, remove `SETUP_TOKEN`, and restart the container.

### Self-contained demo

To preview every screen without AzerothCore or MySQL:

```bash
docker run --rm -p 8080:8080 -e MOCK_MODE=true azeroth-portal:latest
```

Sign in with `DEMO` / `demo1234`. Demo mode includes characters, armory equipment, 500 credits, products, and simulated mail delivery. Its state is intentionally held in memory and resets when the process stops.

Equipment names, levels, armor, and stats come from AzerothCore. The center paper-doll loads pinned jQuery 3.7.1 before Wowhead's WebGL model viewer, then supplies the character's race/gender model and equipped display IDs. Since the world database stores client display IDs rather than icon filenames, the browser also resolves missing live-server icons through Wowhead's tooltip endpoint and caches the result locally. Demo icons are bundled as known icon names. Raid dates come only from `character_achievement`; AzerothCore's current-lockout encounter mask is intentionally not presented as permanent progression history.

The container is the portal only. AzerothCore's existing auth, characters, world databases and worldserver stay external.

## Database access

Before a production deployment, follow and rehearse [the backup and restore runbook](docs/BACKUP-RESTORE.md). Keep the pre-migration backup and exact container digest together.

Use a dedicated MySQL user. It needs to read the three core databases, insert accounts and realm-character rows, and manage the portal tables. A simple initial setup is:

```sql
CREATE USER 'portal'@'%' IDENTIFIED BY 'use-a-long-password';
GRANT SELECT ON acore_world.* TO 'portal'@'%';
GRANT SELECT ON acore_characters.* TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.account TO 'portal'@'%';
GRANT SELECT ON acore_auth.account_banned TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.realmcharacters TO 'portal'@'%';
GRANT SELECT ON acore_auth.realmlist TO 'portal'@'%';
GRANT CREATE, ALTER, INDEX ON acore_auth.* TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_schema_migrations TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_sessions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_account_security TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_totp_recovery_codes TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_privacy_requests TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_settings TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_wallets TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_products TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_product_items TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_orders TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_order_items TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_order_steps TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_credit_ledger TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_notifications TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_staff_roles TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_ticket_messages TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_ticket_events TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_canned_replies TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_community_issues TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_community_issue_votes TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_community_issue_comments TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_guild_recruitment TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_guild_applications TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_arena_seasons TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_arena_snapshots TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_resources TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_character_privacy TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_downloads TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_launcher_patches TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_competitive_events TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_raid_kills TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_raid_members TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_pvp_matches TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_pvp_match_members TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_payment_events TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_payment_transactions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_payment_webhooks TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_moderation_log TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_command_log TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_support_tickets TO 'portal'@'%';
GRANT SELECT, INSERT, DELETE ON acore_auth.portal_password_resets TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_email_verifications TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_news TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_news_revisions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_pages TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_page_revisions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_events TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_event_registrations TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_event_reward_grants TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_transfer_requests TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_credit_packages TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_gifts TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_gift_codes TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_gift_code_uses TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_coupons TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_coupon_uses TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_vote_sites TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_vote_clicks TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_vote_events TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_vote_rewards TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_vote_campaigns TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_vote_campaign_winners TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_daily_rewards TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_external_reward_events TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_missions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_mission_claims TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_referrals TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_referral_milestones TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_referral_milestone_claims TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_identities TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_identity_accounts TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_identity_providers TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_oauth_states TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_passkey_challenges TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_passkey_credentials TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_forced_password_resets TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_media_assets TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_navigation_items TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_audit_filters TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_bundle_templates TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_bundle_template_items TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_product_variants TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_product_variant_items TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_shop_collections TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_collection_products TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_stock_movements TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_coupon_events TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_wishlist TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_battleground_matches TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_battleground_members TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_raid_attempts TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_raid_attempt_members TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_raid_eligibility_rules TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_ranking_exclusions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_moderation_evidence TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_sanctions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_sanction_notes TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_sanction_appeals TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_character_services TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_admin_audit TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.account_access TO 'portal'@'%';
```

For the first run, granting broader rights on `acore_auth` is easier; reduce them after the portal tables have been created. The `account_access` insert permission is needed only by the optional setup wizard and may be revoked after setup.

## Enable shop delivery

In `worldserver.conf`:

```ini
SOAP.Enabled = 1
SOAP.IP = "0.0.0.0"
SOAP.Port = 7878
```

Create a dedicated AzerothCore account for SOAP, give it only the RBAC permission needed for `send items`, and put its credentials in `SOAP_USER` and `SOAP_PASSWORD`. Do not reuse an owner account.

Add a product with the admin API:

```bash
curl -X POST http://localhost:8080/api/admin/products \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Traveler’s Satchel",
    "description":"A useful bag delivered by in-game mail.",
    "itemId":21841,
    "quantity":1,
    "price":25,
    "category":"Utility",
    "imageUrl":""
  }'
```

Class-restricted custom packages accept up to 48 distinct items (split into safe twelve-attachment mails) and can also apply a level service. For example:

```bash
curl -X POST http://localhost:8080/api/admin/products \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{
    "name":"Paladin T8 Package",
    "description":"Realm-approved PvE set, gems and enchants.",
    "price":135,
    "category":"PvE",
    "classId":2,
    "tier":"T8",
    "serviceLevel":80,
    "items":[
      {"itemId":45638,"quantity":1},
      {"itemId":45632,"quantity":1},
      {"itemId":45644,"quantity":1},
      {"itemId":45650,"quantity":1},
      {"itemId":45656,"quantity":1}
    ]
  }'
```

Use item IDs approved for your exact AzerothCore world database. Gems and enchant scrolls are ordinary bundle items and arrive in the same mail; the portal deliberately does not write enchantment data directly into character inventory tables.

The default specialization-aware S6, S7, and T8 catalog is populated automatically from the installed AzerothCore `item_template` table. Every package includes its five-piece armor set, matching off-pieces, jewelry, trinkets, class-appropriate weapons and relics, gems, enchants, a complete level-80 boost, four Frostweave Bags, faction-appropriate ground and flying mounts, and 10,000 gold. The boost learns all class-trainer spell ranks through level 80, grants only the weapon proficiencies supported by that class, raises combat skills to 400, and teaches Artisan Riding plus Cold Weather Flying. See [CATALOG.md](CATALOG.md) for the exact behavior and safety rationale.

Credits can be granted through the audited GM console. To sell fixed credit packs, configure the five `STRIPE_*` variables and point a Stripe webhook at `/api/billing/webhook` for `checkout.session.completed` events.

Race Change and Faction Change are seeded as standard shop services. Fulfillment calls AzerothCore's allow-listed `character changerace` or `character changefaction` command through SOAP; it never writes `at_login` flags directly. The character must remain offline until delivery completes and chooses its new race or faction appearance on the next login.

```sql
UPDATE acore_auth.portal_wallets SET balance = balance + 100 WHERE account_id = 1;
```

Purchases commit the debit and an immutable item snapshot before entering the delivery queue. Ambiguous SOAP failures move to `review` instead of being retried automatically, preventing duplicate mail. A GM can inspect, retry, or refund them from the account console.

Operational probes are available at `/healthz`, `/readyz`, and `/metrics`.

The player-facing `/realm` page reports realm population and availability. Technical portal, database, realm, and delivery health is restricted to the Monitoring section of `/admin`. GMs can edit branding and links, disable feature modules, schedule maintenance, publish announcements, manage products, and create coupons from the dedicated admin panel. The catalog editor searches the installed WotLK `item_template`, displays equipment in a slot preview, and supports arbitrary item and bag quantities alongside gold, services, schedules, limits, and credit pricing. Environment feature flags remain hard security gates; saved settings in `portal_settings` take effect immediately and can be reset by deleting the `site_config` row.

Character services verify account ownership and offline state before issuing a fixed, allow-listed AzerothCore command. The browser never supplies a raw command. Every attempted real service is recorded in `portal_character_services`.

## Local development

Requirements: Go 1.26+, Node 24+, and access to an AzerothCore database.

```bash
npm install
npm run build
cp .env.example .env
set -a; source .env; set +a
go run .
```

For UI-only work, run `npm run dev`. API calls still expect the Go service, so use a reverse proxy or build and run Go for complete flows.

## Configuration

| Variable | Purpose | Default |
|---|---|---|
| `AUTH_DSN` | Go MySQL DSN for auth DB | required |
| `PORTAL_BIND`, `PORTAL_PORT` | Host interface and port used by production Compose | `127.0.0.1`, `8080` |
| `CHARACTERS_DSN`, `WORLD_DSN` | DSNs for other core DBs | `AUTH_DSN` |
| `AUTH_DB`, `CHARACTERS_DB`, `WORLD_DB` | Qualified schema names | AzerothCore defaults |
| `PUBLIC_URL` | Canonical origin used for CSRF checks | `http://localhost:8080` |
| `COOKIE_SECURE` | Require HTTPS for session cookies; mandatory with an HTTPS `PUBLIC_URL` | `false` |
| `TRUST_PROXY` | Trust proxy-provided client IP headers for sessions and rate limiting | `false` |
| `REALM_NAME`, `REALM_ADDRESS`, `REALM_TYPE`, `REALM_TIMEZONE` | Per-realm identity, realmlist address, PvE/PvP/RP mode, and timezone | `Azeroth`, example host, `PvE`, `UTC` |
| `REALM_KEY` | Stable key identifying this realm in the frontend realm switcher | `default` |
| `REALMS_JSON` | Optional in-process realm definitions with per-realm ID, character/world DSNs and SOAP connection; omitted fields inherit the single-realm variables | current realm only |
| `PORTAL_NAME`, `BRAND_MARK` | Site-wide display name and short sigil (up to 3 characters recommended) | realm name, `A` |
| `PORTAL_TAGLINE`, `FOOTER_TEXT` | Home-page introduction and footer copy | community-oriented defaults |
| `HOME_HEADLINE`, `HOME_EYEBROW`, `HOME_PRIMARY_CTA`, `HOME_CONNECT_TITLE`, `HOME_GUIDE_TEXT`, `HOME_RULES`, `DISCORD_STATUS`, `HOME_CHANGELOG` | Homepage content defaults, also editable from Administration | realm name and built-in labels |
| `HOME_FEATURES`, `HOME_PROGRESSION` | Homepage cards (`Title\|Description`) and roadmap stages (`Title\|Status\|Description`), one per line | hidden until configured |
| `ARENA_REWARD_POLICY` | Public per-realm arena eligibility, cutoff, and title policy shown on Rankings | explicitly shown as unpublished when empty |
| `EXPANSION_NAME`, `CLIENT_VERSION`, `CLIENT_BUILD` | Client information shown in connection instructions | WotLK, `3.3.5a`, `12340` |
| `EXPERIENCE_RATE`, `XP_QUEST_RATE`, `XP_KILL_RATE`, `XP_EXPLORATION_RATE` | Overall and granular displayed XP rates | `2×` |
| `DROP_RATE`, `REPUTATION_RATE`, `HONOR_RATE`, `PROFESSION_RATE` | Displayed gameplay rates | `1×` |
| `START_LEVEL`, `MAX_LEVEL`, `POPULATION_CAP`, `TRANSFER_SLA_HOURS`, `FACTION_POLICY`, `CROSS_FACTION`, `SEASON_NAME` | Realm rules, advertised character-transfer review target, and current phase metadata | `1`, `80`, `0`, `72`, `both`, `false`, unset |
| `CROSS_FACTION_ACCOUNTS`, `CROSS_FACTION_CALENDAR`, `CROSS_FACTION_CHANNELS`, `CROSS_FACTION_GROUPS`, `CROSS_FACTION_GUILDS`, `CROSS_FACTION_AUCTIONS`, `CROSS_FACTION_MAIL`, `CROSS_FACTION_WHO`, `CROSS_FACTION_FRIENDS`, `CROSS_FACTION_TRADE` | Granular public cross-faction capability flags matching AzerothCore's `AllowTwoSide.*` options | `CROSS_FACTION` fallback |

Realm gameplay values edited in the portal are public profile metadata by default. They do not silently rewrite `worldserver.conf`. Optionally connect a constrained realm agent to expose desired, observed, drifted, and restart-required states under **Admin → Realms → Configuration**.
| `DOWNLOAD_URL`, `COMMUNITY_URL` | Optional HTTP(S) client-download and Discord/community links | unset/hidden |
| `LOGO_URL`, `HERO_IMAGE_URL`, `FAVICON_URL` | Optional root-relative or HTTPS brand assets; the hero image identifies the server on the home page | bundled Northrend image for the hero, others unset |
| `THEME_PRIMARY`, `THEME_SECONDARY`, `THEME_ACCENT`, `THEME_BACKGROUND` | Six-digit hex colors used by the runtime theme | built-in green/gold theme |
| `PORTAL_LOCALE` | Document language tag used by browsers and formatters | `en` |
| `UI_TEXT_JSON` | JSON object overriding marked interface labels, such as `{"nav.home":"Accueil"}` | `{}` |
| `NEWS_JSON` | JSON array of up to 12 `{title,summary,date,url}` home-page news cards | `[]` |
| `TERMS_URL`, `PRIVACY_URL` | Optional HTTP(S) legal links shown in the footer | unset/hidden |
| `ENABLE_REGISTRATION`, `ENABLE_ARMORY`, `ENABLE_RANKINGS`, `ENABLE_GUILDS` | Enable or disable public modules and their API endpoints | `true` |
| `ENABLE_REALM_STATUS`, `ENABLE_SHOP`, `ENABLE_SUPPORT`, `ENABLE_ADMIN_PANEL` | Enable or disable operational modules and their API endpoints | `true` |
| `ENABLE_GM_CONSOLE`, `GM_CONSOLE_LEVEL` | Enable the SOAP-backed browser console and set its minimum GM level | `false`, `3` |
| `GM_CONSOLE_ALLOWED_PREFIXES` | Comma-separated command prefixes permitted by the browser console | Read-only information commands |
| `GM_CONSOLE_ALLOW_ALL` | Permit every valid one-line command; always requires a level-3 GM | `false` |
| `ENABLE_SETUP`, `SETUP_TOKEN` | Enable the one-time setup wizard and protect it with a 16–256 character secret | `false`, unset |
| `AUTO_MIGRATE` | Apply numbered portal migrations on startup; set false when migrations run as a separate deployment job | `true` |
| `AUDIT_RETENTION_DAYS` | Retain staff audit events before automatic deletion (minimum 30 days) | `365` |
| `AUDIT_IP_RETENTION_DAYS` | Redact IP addresses and user agents earlier than full audit retention | `30` |
| `SETUP_GM_LEVEL`, `SETUP_GM_REALM_ID` | Initial administrator access level and realm scope | `3`, `-1` |
| `ACCOUNT_EXPANSION` | New account expansion field | `2` |
| `REALM_ID` | Realm used when resolving GM access | `1` |
| `STAFF_SUPPORT_GM_LEVEL`, `STAFF_MODERATOR_GM_LEVEL`, `GM_LEVEL` | Ordered minimum AzerothCore levels for support, moderation, and full administration | `GM_LEVEL` for all three |
| `STAFF_SHOP_MANAGERS` | Comma-separated account names granted only shop and order management access | unset |
| `STARTING_CREDITS` | New wallet balance | `0` |
| `SOAP_URL`, `SOAP_USER`, `SOAP_PASSWORD` | Worldserver delivery endpoint | unset |
| `DELIVERY_DIAGNOSTIC_CHARACTER` | Dedicated disposable offline character for the guarded one-item SOAP test in Admin → Monitoring | unset/disabled |
| `REALM_START_WEBHOOK`, `REALM_CONTROL_TOKEN` | Optional authenticated orchestrator webhook for starting an offline worldserver | unset/disabled |
| `REALM_AGENT_URL`, `REALM_AGENT_TOKEN` | Optional HTTPS endpoint and 32+ character bearer token for allow-listed worldserver configuration reconciliation | unset/metadata-only |
| `ADMIN_TOKEN` | Bearer token for product creation | unset/disabled |
| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Stripe API and webhook signing secrets | unset/disabled |
| `STRIPE_PRICE_SMALL`, `STRIPE_PRICE_MEDIUM`, `STRIPE_PRICE_LARGE` | Stripe Price IDs for 100/550/1,200 credits | unset |
| `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET` | Optional registration bot protection | unset/disabled |
| `REQUIRE_EMAIL_VERIFICATION` | Lock new accounts until their one-time email link is confirmed; requires SMTP configuration | `false` |
| `SMTP_ADDR`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | SMTP transport for password recovery and account verification (`SMTP_ADDR` is `host:port`) | unset/disabled |
| `TOTP_ENCRYPTION_KEY` | Base64url-encoded 32-byte AES key for authenticator secrets; keep stable and backed up | unset; new TOTP enrollment disabled |
| `DISCORD_WEBHOOK_URL` | Optional Discord webhook for registrations, purchases, tickets, and moderation actions | unset |
| `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET` | Enable Discord sign-in and secure linking to a portal master account | unset/hidden |
| `DISCORD_GUILD_ID` | Show live public member presence from Discord's opt-in Server Widget | unset |
| `DISCORD_BOT_REWARD_SECRET` | Bearer secret for idempotent bot-managed rewards to explicitly linked Discord identities | unset |
| `DISCORD_REDIRECT_URL` | Exact Discord callback URL; defaults to `PUBLIC_URL/api/auth/discord/callback` and must use the same origin | derived |
| `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` | Enable Google sign-in and secure linking to a portal master account | unset/hidden |
| `GOOGLE_REDIRECT_URL` | Exact Google callback URL; defaults to `PUBLIC_URL/api/auth/google/callback` and must use the same origin | derived |
| `ANALYTICS_SCRIPT_URL`, `ANALYTICS_DOMAIN` | Optional HTTPS, `data-domain` compatible analytics integration | unset/hidden |
| `VOTE_URL`, `VOTE_REWARD_CREDITS`, `VOTE_CALLBACK_SECRET` | Legacy voting link/reward and the shared callback bearer secret | unset, `0`, unset |
| `COMPETITIVE_INGEST_SECRET` | Bearer secret for idempotent raid-kill and arena-match collector callbacks | unset/disabled |

Create one or more voting sites in **Admin → Content & coupons**. Configure each provider to POST `{"username":"PLAYER","eventId":"provider-unique-id","ip":"optional-voter-ip"}` to `/api/integrations/votes/<site-slug>` with `Authorization: Bearer <VOTE_CALLBACK_SECRET>`. Event IDs are unique per voting site, so provider retries cannot award credits twice. When the provider supplies the voter's IP, the portal validates it, stores only a salted hash, and enforces the site's cooldown across accounts sharing that network. Successful rewards are ledgered and audited. The older single-provider callback at `/api/rewards/vote/callback` remains available for `VOTE_URL` deployments.

Discord bots can award a linked player without trusting an email address by posting `{"discordUserId":"123…","eventId":"unique-event","credits":5,"reason":"Community event"}` to `/api/integrations/discord/rewards` with `Authorization: Bearer <DISCORD_BOT_REWARD_SECRET>`. A Discord identity must already be linked in the player's Security screen; each event ID is credited at most once and is recorded in both the external-event and wallet ledgers.

Optional desktop launchers can read `GET /api/launcher/manifest`. Schema version 2 contains the selected realm key/name/realmlist, expected WotLK client version/build, active full packages, operator-defined mirrors, and a directed incremental-patch graph (`fromVersion` → `toVersion`) for each platform. Packages retain release date, checksum, signature, VirusTotal report, changelog, and system requirements; patches require SHA-256 and may also be signed. A launcher should choose a valid path from the installed version, download from the primary URL or a mirror, verify every payload before applying it, and fall back to a full package when no verified patch path exists. The portal never asks players to bypass operating-system security warnings.

For raid-speed and per-match arena history, configure a trusted server script or log collector with `COMPETITIVE_INGEST_SECRET`. It can POST raid events to `/api/integrations/raids` and arena events to `/api/integrations/pvp` using `Authorization: Bearer <COMPETITIVE_INGEST_SECRET>`. Both payloads require a stable `eventId`; repeated delivery is accepted without duplicating results. Raid events accept `guildId`, `guildName`, `raid`, `boss`, `difficulty`, `result` (`wipe` or `kill`), `attemptNumber`, `bossHealthPercent`, `durationSeconds`, `occurredAt`, and `members`. Each member includes `guid`, `name`, `class`, and an optional `role` (`tank`, `healer`, or `damage`). PvP events accept `season`, `bracket`, `teamId`, `teamName`, `opponentId`, `opponentName`, `result`, `ratingBefore`, `ratingAfter`, `ratingChange`, `durationSeconds`, `playedAt`, and `members`.

To prove that the configured SOAP account can deliver, set `DELIVERY_DIAGNOSTIC_CHARACTER` to a dedicated disposable character and keep it offline. A realm operator can then use **Admin → Monitoring → Controlled delivery test**. The operation is deliberately limited to one Tough Jerky (item 117), requires step-up authentication and exact character-name confirmation, and returns a correlation ID plus cleanup instructions. It never exposes arbitrary commands or performs direct inventory writes.

Raid events are retained even when they fail public-ranking eligibility, but only eligible events appear in speed and recent-kill rankings. Realm operators configure 10/25-player roster bounds, duration bounds, ingestion age, and the character-GUID requirement under **Admin → Realm → Operations**. The public ranking page shows the effective rules and source provenance. This makes outliers reviewable without allowing undersized, stale, unsigned, or implausibly fast submissions onto the ladder.

Set `PUBLIC_URL=https://your-domain`, `COOKIE_SECURE=true`, terminate TLS at your proxy, and keep the portal and SOAP ports behind a firewall in production. Set `TRUST_PROXY=true` only when the portal cannot be reached directly and your proxy overwrites (rather than appends to) incoming client-IP headers.

For multiple realms, set `REALMS_JSON` and run the same single portal container. The auth database, accounts, sessions, credits, and catalog are shared; each realm entry selects its own character/world database, realm ID, address, XP rate, transfer review SLA (`transferSlaHours`), SOAP endpoint, and optional start webhook. The selected realm is validated by the backend and remembered in a secure same-site cookie. Every page and API call is routed to that realm, while recovery/setup tokens are discarded when switching realms.

### Realm configuration agent contract

The optional agent is deliberately separate from the public portal and receives no arbitrary paths, configuration names, or shell commands. It must expose:

- `GET /v1/config`: return `{"version":"agent-version","values":{"rate.xp.quest":2},"restartRequired":[],"observedAt":"RFC3339"}`.
- `POST /v1/config/apply`: accept `{"realmKey":"default","values":{...}}`, create a recoverable configuration backup, apply only the supported keys, and return the resulting snapshot plus `backupId`.

Both calls carry `Authorization: Bearer <REALM_AGENT_TOKEN>` and `X-Portal-Realm`. The current contract permits only the rate and `cross_faction.*` keys shown by the configuration screen. The agent should map those stable names to the matching AzerothCore version, validate types again, write atomically, retain backups outside the web root, and report exactly which keys need a worldserver restart. Use a distinct token per realm in `REALMS_JSON`. The portal refuses plaintext non-loopback agent URLs.

The shipped image implements that contract. Run it close to the worldserver with only the configuration directory mounted writable:

```sh
docker run --read-only --cap-drop=ALL --user 1000:1000 \
  -p 127.0.0.1:9090:9090 \
  -e REALM_AGENT_ADDR=:9090 \
  -e REALM_AGENT_REALM_KEY=default \
  -e REALM_AGENT_TOKEN='<64-character-random-secret>' \
  -e REALM_AGENT_CONFIG_PATH=/azeroth/etc/worldserver.conf \
  -e REALM_AGENT_BACKUP_DIR=/azeroth/etc/portal-backups \
  -v /host/azeroth/etc:/azeroth/etc \
  ghcr.io/ghazibendahmane/azeroth-core-web:latest realm-agent
```

Point the portal's `REALM_AGENT_URL` at that private endpoint. If it is not loopback, terminate TLS with a private reverse proxy or service mesh. The agent will never accept an arbitrary setting or path from an HTTP request. It creates a timestamped, checksummed backup before atomically replacing the configuration file. Mount the containing directory—not only the file—because atomic replacement requires creating and renaming a sibling file.

GM tools live in the dedicated `/admin` panel and are only exposed to authenticated GM accounts. Its catalog view provides search, status filtering, sortable columns, and the full product/package editor. The browser command console is disabled by default. When enabled, commands are executed using the configured SOAP account, so its RBAC permissions are the real upper bound. Every attempt is stored in `portal_command_log`; password-bearing account commands are redacted in that log. Prefer the prefix allow-list and enable `GM_CONSOLE_ALLOW_ALL=true` only for trusted level-3 operators with 2FA.

`UI_TEXT_JSON` supports every key attached to `data-i18n`, `data-i18n-placeholder`, `data-i18n-aria-label`, or `data-i18n-title`. Shared navigation, sign-in, registration, connection-guide, account navigation, shop discovery, armory search, and rankings copy is marked for translation; operators can add the same attributes to custom templates. Values are inserted as text or plain attribute values, never HTML.

## Verification

```bash
make test
docker build -t azeroth-portal .
```

The browser audit writes desktop screenshots and an accessibility report to
`test-results/browser`. After manually reviewing a known-good run, create or
refresh the visual baseline with `npm run test:visual:approve`. Later runs can
use `npm run test:visual`; it decodes the PNGs locally and fails when more than
2% of pixels differ (override with `VISUAL_DIFF_RATIO`). Baseline approval is
deliberately separate so a changed screenshot cannot approve itself in CI.

CI runs the browser suite with `STRICT_CONTRAST=1`, so detectable WCAG AA text
contrast regressions fail alongside missing labels, broken tab relationships,
duplicate IDs, overflow, and runtime errors.

The implementation follows AzerothCore's current `account` schema and `Acore::Crypto::SRP6::MakeRegistrationData` algorithm: uppercased Latin credentials, a random 32-byte salt, SHA-1 derivation, the AzerothCore modulus/generator, and little-endian 32-byte verifier storage.

# Azeroth Portal

A production-minded AzerothCore web portal in one container: a Go API serves an embedded Astro frontend and connects to your existing AzerothCore MySQL databases and worldserver SOAP endpoint.

## Included

- AzerothCore-compatible SRP6 account registration and login
- Secure HTTP-only sessions, same-origin checks, request limits, and security headers
- Public armory search, 19-slot equipment paper-doll, item tooltips/icons, guilds, and play time
- 2v2, 3v3, and 5v5 arena team ladders with ratings, records, and rosters
- PvE raid progression from character achievement dates, including guild-first dates
- Account dashboard with characters, credit balance, and order history
- Password changes, TOTP two-factor authentication, and session revocation
- Single-use email password recovery and optional Cloudflare Turnstile registration protection
- Audited GM credit grants authorized from AzerothCore `account_access`
- GM player lookup with active-ban status, audited account bans/unbans, and character kicks
- Character mutes, IP bans, announcements, MOTD updates, guarded GM-level changes, and scheduled realm restart/shutdown controls
- Player support tickets with GM replies and status management
- GM delivery queue/reconciliation view and credit ledger
- Gold bundles, level services, and class-restricted multi-item gear packages
- Queued race-change and faction-change services using AzerothCore's supported character commands
- 51 specialization-aware, full-slot WotLK S6, S7, and T8 loadouts resolved from the installed world database
- Stripe Checkout credit packs with signed, replay-safe webhooks
- Categorized shop with an admin product API
- Durable queued fulfillment through AzerothCore SOAP, with review/refund controls
- Realm status, faction population, guild directory, and guild rosters
- Health, readiness, and Prometheus metrics endpoints
- Responsive, dependency-light Astro UI
- One multi-stage Docker image; no Node runtime in production

The portal creates its own `portal_*` tables in `acore_auth`. It never modifies AzerothCore's inventory tables. Shop items are sent by the worldserver itself via in-game mail.

## Quick start

```bash
cp .env.example .env
# Edit database, realm, SOAP, and public URL settings.
docker compose up --build -d
```

Open `http://localhost:8080`. The application exits early with a useful error if any database cannot be reached.

### Self-contained demo

To preview every screen without AzerothCore or MySQL:

```bash
docker run --rm -p 8080:8080 -e MOCK_MODE=true azeroth-portal:latest
```

Sign in with `DEMO` / `demo1234`. Demo mode includes characters, armory equipment, 500 credits, products, and simulated mail delivery. Its state is intentionally held in memory and resets when the process stops.

Equipment names, levels, armor, and stats come from AzerothCore. The center paper-doll loads Wowhead's WebGL model viewer with the character's race/gender model and equipped display IDs. Since the world database stores client display IDs rather than icon filenames, the browser also resolves missing live-server icons through Wowhead's tooltip endpoint and caches the result locally. Demo icons are bundled as known icon names. Raid dates come only from `character_achievement`; AzerothCore's current-lockout encounter mask is intentionally not presented as permanent progression history.

The container is the portal only. AzerothCore's existing auth, characters, world databases and worldserver stay external.

## Database access

Use a dedicated MySQL user. It needs to read the three core databases, insert accounts and realm-character rows, and manage the portal tables. A simple initial setup is:

```sql
CREATE USER 'portal'@'%' IDENTIFIED BY 'use-a-long-password';
GRANT SELECT ON acore_world.* TO 'portal'@'%';
GRANT SELECT ON acore_characters.* TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.account TO 'portal'@'%';
GRANT SELECT ON acore_auth.account_banned TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.realmcharacters TO 'portal'@'%';
GRANT SELECT ON acore_auth.realmlist TO 'portal'@'%';
GRANT CREATE ON acore_auth.* TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_sessions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_wallets TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_products TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_product_items TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_orders TO 'portal'@'%';
GRANT SELECT, INSERT ON acore_auth.portal_credit_ledger TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_moderation_log TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE ON acore_auth.portal_support_tickets TO 'portal'@'%';
GRANT SELECT ON acore_auth.account_access TO 'portal'@'%';
```

For the first run, granting broader rights on `acore_auth` is easier; reduce them after the four portal tables have been created.

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

The default specialization-aware S6, S7, and T8 catalog is populated automatically from the installed AzerothCore `item_template` table. It includes each five-piece armor set, all matching off-pieces, jewelry, trinkets, class-appropriate weapons and relics, a phase-appropriate gem kit, a full enchant kit, and level 80. See [CATALOG.md](CATALOG.md) for the exact behavior and safety rationale.

Credits can be granted through the audited GM console. To sell fixed credit packs, configure the five `STRIPE_*` variables and point a Stripe webhook at `/api/billing/webhook` for `checkout.session.completed` events.

Race Change and Faction Change are seeded as standard shop services. Fulfillment calls AzerothCore's allow-listed `character changerace` or `character changefaction` command through SOAP; it never writes `at_login` flags directly. The character must remain offline until delivery completes and chooses its new race or faction appearance on the next login.

```sql
UPDATE acore_auth.portal_wallets SET balance = balance + 100 WHERE account_id = 1;
```

Purchases commit the debit and an immutable item snapshot before entering the delivery queue. Ambiguous SOAP failures move to `review` instead of being retried automatically, preventing duplicate mail. A GM can inspect, retry, or refund them from the account console.

Operational probes are available at `/healthz`, `/readyz`, and `/metrics`.

## Local development

Requirements: Go 1.24+, Node 22+, and access to an AzerothCore database.

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
| `CHARACTERS_DSN`, `WORLD_DSN` | DSNs for other core DBs | `AUTH_DSN` |
| `AUTH_DB`, `CHARACTERS_DB`, `WORLD_DB` | Qualified schema names | AzerothCore defaults |
| `PUBLIC_URL` | Canonical origin used for CSRF checks | `http://localhost:8080` |
| `COOKIE_SECURE` | Require HTTPS for session cookies | `false` |
| `REALM_NAME`, `REALM_ADDRESS` | UI realm identity and realmlist address | `Azeroth`, example host |
| `ACCOUNT_EXPANSION` | New account expansion field | `2` |
| `REALM_ID` | Realm used when resolving GM access | `1` |
| `GM_LEVEL` | Minimum AzerothCore GM level allowed to grant credits | `3` |
| `STARTING_CREDITS` | New wallet balance | `0` |
| `SOAP_URL`, `SOAP_USER`, `SOAP_PASSWORD` | Worldserver delivery endpoint | unset |
| `ADMIN_TOKEN` | Bearer token for product creation | unset/disabled |
| `STRIPE_SECRET_KEY`, `STRIPE_WEBHOOK_SECRET` | Stripe API and webhook signing secrets | unset/disabled |
| `STRIPE_PRICE_SMALL`, `STRIPE_PRICE_MEDIUM`, `STRIPE_PRICE_LARGE` | Stripe Price IDs for 100/550/1,200 credits | unset |
| `TURNSTILE_SITE_KEY`, `TURNSTILE_SECRET` | Optional registration bot protection | unset/disabled |
| `SMTP_ADDR`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM` | Optional password-recovery email transport | unset/disabled |

Set `PUBLIC_URL=https://your-domain`, `COOKIE_SECURE=true`, terminate TLS at your proxy, and keep the portal and SOAP ports behind a firewall in production.

## Verification

```bash
make test
docker build -t azeroth-portal .
```

The implementation follows AzerothCore's current `account` schema and `Acore::Crypto::SRP6::MakeRegistrationData` algorithm: uppercased Latin credentials, a random 32-byte salt, SHA-1 derivation, the AzerothCore modulus/generator, and little-endian 32-byte verifier storage.

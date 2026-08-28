# Azeroth Portal

A production-minded AzerothCore web portal in one container: a Go API serves an embedded Astro frontend and connects to your existing AzerothCore MySQL databases and worldserver SOAP endpoint.

## Included

- AzerothCore-compatible SRP6 account registration and login
- Secure HTTP-only sessions, same-origin checks, request limits, and security headers
- Public armory search, 19-slot equipment paper-doll, item tooltips/icons, guilds, and play time
- 2v2, 3v3, and 5v5 arena team ladders with ratings, records, and rosters
- PvE raid progression from character achievement dates, including guild-first dates
- Account dashboard with characters, credit balance, and order history
- Categorized shop with an admin product API
- Safe shop fulfillment through AzerothCore's `send items` SOAP command
- Responsive, dependency-light Astro UI
- One multi-stage Docker image; no Node runtime in production

The portal creates four `portal_*` tables in `acore_auth`. It never modifies AzerothCore's inventory tables. Shop items are sent by the worldserver itself via in-game mail.

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
GRANT SELECT, INSERT ON acore_auth.realmcharacters TO 'portal'@'%';
GRANT SELECT ON acore_auth.realmlist TO 'portal'@'%';
GRANT CREATE ON acore_auth.* TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_sessions TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_wallets TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_products TO 'portal'@'%';
GRANT SELECT, INSERT, UPDATE, DELETE ON acore_auth.portal_orders TO 'portal'@'%';
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

Credits are deliberately not tied to a payment provider. Grant credits through an audited staff tool or SQL until you integrate your payment processor:

```sql
UPDATE acore_auth.portal_wallets SET balance = balance + 100 WHERE account_id = 1;
```

For real-money sales, have the payment provider's signed webhook credit the wallet. Never trust a credit amount sent by the browser. The purchase endpoint locks both product and wallet rows and rolls back the debit when SOAP fails. As with any external delivery call, a process crash in the narrow interval after mail delivery and before the SQL commit needs staff reconciliation from worldserver and portal logs.

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
| `STARTING_CREDITS` | New wallet balance | `0` |
| `SOAP_URL`, `SOAP_USER`, `SOAP_PASSWORD` | Worldserver delivery endpoint | unset |
| `ADMIN_TOKEN` | Bearer token for product creation | unset/disabled |

Set `PUBLIC_URL=https://your-domain`, `COOKIE_SECURE=true`, terminate TLS at your proxy, and keep the portal and SOAP ports behind a firewall in production.

## Verification

```bash
make test
docker build -t azeroth-portal .
```

The implementation follows AzerothCore's current `account` schema and `Acore::Crypto::SRP6::MakeRegistrationData` algorithm: uppercased Latin credentials, a random 32-byte salt, SHA-1 derivation, the AzerothCore modulus/generator, and little-endian 32-byte verifier storage.

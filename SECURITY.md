# Security review

Reviewed on 2026-08-28 against the current repository state.

## Findings

### SQL injection: no exploitable path found

All request-derived values are passed as MySQL parameters. The only values interpolated into SQL strings are configured schema names (`AUTH_DB`, `CHARACTERS_DB`, and `WORLD_DB`), which are rejected during startup unless they match `^[A-Za-z0-9_]+$`. Progression placeholders are generated from a fixed server-side achievement list.

### Authentication and sessions

- Account verifiers follow AzerothCore's SRP6 registration algorithm and use cryptographic random salts.
- Portal session tokens contain 256 bits of randomness; only SHA-256 token hashes are stored.
- Users can rotate passwords, revoke sessions, and enable RFC 6238 TOTP authentication.
- Password reset links contain 256 random bits, are stored only as SHA-256 hashes, expire after one hour, and are single use.
- Cookies are HTTP-only and SameSite=Lax. Production must set `COOKIE_SECURE=true` and use HTTPS.
- Locked and actively banned AzerothCore accounts are denied login, and existing portal sessions stop resolving while the account remains restricted.
- Login and registration endpoints are rate limited. For a multi-instance deployment, replace the in-memory limiter with a shared store.

### Browser and API security

- Mutating browser requests are checked against `PUBLIC_URL` when an Origin header is present.
- JSON bodies are limited to 1 MiB and unknown fields are rejected.
- Responses set CSP, clickjacking, MIME-sniffing, referrer, and browser-permissions protections.
- Dynamic UI values are escaped or assigned through `textContent`.
- Runtime theme colors are restricted to six-digit hex values; brand assets require HTTPS or same-origin paths, and configurable links reject non-HTTP(S) schemes.
- Disabled portal modules are rejected by their API endpoints as well as hidden in navigation; feature switches are not treated as client-side authorization.
- Product image URLs are limited to absolute HTTP(S) URLs.
- The admin bearer token is compared in constant time.
- GM credit grants require a live authenticated session and the configured `account_access` level. Every grant is recorded in the append-only `portal_credit_ledger` table.
- Moderation and realm-operation endpoints require the configured AzerothCore GM level. Only fixed commands for bans, mutes, kicks, announcements, MOTD, GM levels, and guarded start/shutdown/restart operations can be issued; inputs are allow-listed, privilege escalation and self-role changes are rejected, and every attempt is recorded in `portal_moderation_log`.
- Starting an offline realm uses only the operator-configured `REALM_START_WEBHOOK`; users cannot choose its URL. Use HTTPS and a narrowly scoped bearer token, restrict outbound network access, and make the receiver idempotent.
- Stripe credits are applied only after HMAC verification with a five-minute timestamp tolerance. Event and checkout IDs enforce replay safety.
- Registration can require server-verified Cloudflare Turnstile tokens; it is disabled unless configured.

### Shop delivery

- Product IDs, prices, quantities, balances, and character ownership are verified server-side.
- The wallet and product rows are locked during checkout.
- Items are delivered through AzerothCore SOAP instead of direct inventory writes.
- Character command arguments and configured realm text are constrained before entering a SOAP command.
- A dedicated, least-privilege AzerothCore SOAP account is required.

## Residual risks and deployment requirements

1. Wowhead's JavaScript and asset CDN is a third-party runtime dependency. A compromise or incompatible API change affects the armory model viewer. CSP restricts it to Wowhead's asset host, but self-hosted reviewed assets are safer.
2. SOAP itself has no idempotency key. Ambiguous failures enter manual review; staff should inspect worldserver/mail logs before retrying.
3. TOTP secrets are stored in the portal database. Protect database backups and access as production credentials.
4. Terminate TLS at a trusted proxy, enable secure cookies, firewall SOAP/MySQL, rotate the admin token, and use the least-privilege grants documented in the README.
5. The in-memory demo mode must never be enabled on the public production deployment.
6. Dependency lockfiles and image digests should be maintained by deployment automation. Run dependency vulnerability scans whenever dependencies or base images change.
7. The optional realm-start webhook is an outbound trust boundary. Keep its token separate from AzerothCore credentials, rotate it regularly, and allow-list the destination at the network layer.

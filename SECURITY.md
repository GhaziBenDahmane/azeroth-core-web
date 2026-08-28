# Security review

Reviewed on 2026-08-28 against the current repository state.

## Findings

### SQL injection: no exploitable path found

All request-derived values are passed as MySQL parameters. The only values interpolated into SQL strings are configured schema names (`AUTH_DB`, `CHARACTERS_DB`, and `WORLD_DB`), which are rejected during startup unless they match `^[A-Za-z0-9_]+$`. Progression placeholders are generated from a fixed server-side achievement list.

### Authentication and sessions

- Account verifiers follow AzerothCore's SRP6 registration algorithm and use cryptographic random salts.
- Portal session tokens contain 256 bits of randomness; only SHA-256 token hashes are stored.
- Cookies are HTTP-only and SameSite=Lax. Production must set `COOKIE_SECURE=true` and use HTTPS.
- Locked and actively banned AzerothCore accounts are denied login, and existing portal sessions stop resolving while the account remains restricted.
- Login and registration endpoints are rate limited. For a multi-instance deployment, replace the in-memory limiter with a shared store.

### Browser and API security

- Mutating browser requests are checked against `PUBLIC_URL` when an Origin header is present.
- JSON bodies are limited to 1 MiB and unknown fields are rejected.
- Responses set CSP, clickjacking, MIME-sniffing, referrer, and browser-permissions protections.
- Dynamic UI values are escaped or assigned through `textContent`.
- Product image URLs are limited to absolute HTTP(S) URLs.
- The admin bearer token is compared in constant time.

### Shop delivery

- Product IDs, prices, quantities, balances, and character ownership are verified server-side.
- The wallet and product rows are locked during checkout.
- Items are delivered through AzerothCore SOAP instead of direct inventory writes.
- Character command arguments and configured realm text are constrained before entering a SOAP command.
- A dedicated, least-privilege AzerothCore SOAP account is required.

## Residual risks and deployment requirements

1. Wowhead's JavaScript and asset CDN is a third-party runtime dependency. A compromise or incompatible API change affects the armory model viewer. CSP restricts it to Wowhead's asset host, but self-hosted reviewed assets are safer.
2. SOAP delivery and the SQL transaction cannot be atomic. A process failure after worldserver delivery or one step of a bundle/level service but before the order commit can require manual reconciliation. Add an idempotent fulfillment worker before processing high-value real-money orders.
3. Credits are not a payment system. A payment integration must credit wallets only from authenticated, replay-protected provider webhooks.
4. Terminate TLS at a trusted proxy, enable secure cookies, firewall SOAP/MySQL, rotate the admin token, and use the least-privilege grants documented in the README.
5. The in-memory demo mode must never be enabled on the public production deployment.
6. Dependency lockfiles and image digests should be maintained by deployment automation. Run dependency vulnerability scans whenever dependencies or base images change.

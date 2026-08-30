# Azeroth Portal Product and UX Audit

Audit date: 2026-08-29

## Final validation addendum — 2026-08-30

This pass re-audited the current implementation rather than the earlier
prototype described farther down this document. The portal now covers the
expected public, player-account, commerce, community, staff, and realm-operator
surfaces. The remaining work is primarily UX refinement, frontend isolation,
content quality, and proof against real external systems.

### Verified in this pass

- The original competitor-comparison backlog has been reconciled against the
  current product. Onboarding/downloads, CMS pages, news and events, transfers,
  tracker, arena and battleground history, novelty rankings, tools, social and
  passkey identity, Master-account linking, voting, commerce, guild recruitment,
  security/privacy, notifications, and staff operations now have concrete
  routes and workflows rather than placeholder navigation.
- The download catalog now supports full-package mirrors and checksum-required
  incremental launcher patch graphs. The realm-aware manifest contract is
  version 2 and carries platform, version, release, signature, checksum,
  requirements, changelog, and mirror metadata.
- Realm events now support character reservations, capacity and deadline
  enforcement, player cancellation, attendance management, and transactional,
  exactly-once credit rewards with player notifications.
- Character transfers publish a global or per-realm review SLA through
  `TRANSFER_SLA_HOURS` / `transferSlaHours`; the value is validated, editable
  by staff, and displayed beside the player submission form.
- Voting now exposes personal verified history and consecutive-day progress in
  addition to provider cooldowns, monthly leaders, missions, and committed-seed
  draws. Provider callbacks are replay-safe, enforce per-account cooldowns,
  enforce per-network cooldowns when a provider supplies a valid voter IP,
  retain only a keyed hash of that address, create an audit record, and can
  notify Discord. Campaigns can publish a visible realm-wide participation goal
  and reward without pretending that the portal can change worldserver rates.
- Migrations 39–42 cover launcher mirrors/patches, event participation/rewards,
  indexed vote-network cooldown checks, and community voting goals. Fresh and
  upgrade-safe migration code continues to avoid unsupported
  `ADD COLUMN IF NOT EXISTS` syntax.
- API parity is now 253 production routes, 249 mock routes, and four explicitly
  production-only signed callbacks.

### Original comparison checklist

| Requested capability | Current evidence |
|---|---|
| Accurate realm status, population, uptime, faction totals, and naming | `/api/status`, `/api/realm`, the multi-realm home cards, runtime realm settings, and browser assertions use one realm-scoped source. Technical dependency health is restricted to Administration → Monitoring. |
| Complete joining and download experience | `/play`, managed package/mirror records, platform notes, requirements, troubleshooting, checksums, signatures, VirusTotal/changelog links, and the versioned launcher manifest/patch graph are implemented. |
| Public news, maintenance, incidents, events, history, team, recruitment, FAQ, rules, terms, privacy, and refunds | News/events have publishing workflows. The remaining informational sections are seeded as editable draft CMS pages so the software does not fabricate an operator's history or legal promises. Launch readiness flags missing publication. |
| Suggestions and bug tracker | `/tracker` supports searchable issues, votes, comments, labels, priorities, public staff responses, status workflows, and audited staff triage. |
| Arena, battleground, raid, achievement, collection, and novelty competition | Rankings and armory include stock-core data plus capability-gated signed ingestion for match histories, rating movement, attempts, kills, wipes, rosters, speed, and provenance. Mount, companion, reputation, achievement, played-time, kill, level, and guild-size ladders are present. |
| Addons, WeakAuras, talents, and items | `/tools` contains a staff-managed addon/WeakAura library, shareable WotLK talent planner, and live `item_template` search. |
| Discord and guild community tooling | Discord OAuth/linking, public widget status, validated guild recruitment invites, applications, outgoing operational webhooks, and a replay-safe bot reward callback are implemented. |
| Voting and retention | Multi-site cooldowns, verified callbacks, personal history, streaks, monthly leaders, vote missions, reminders, fraud controls, committed-seed draws, and optional community goals are implemented. Credits are the common reward/shop currency rather than creating a second incompatible vote-point wallet. |
| Identity and account security | Master identities can link/switch multiple AzerothCore accounts. Discord, Google, passkeys, TOTP, recovery codes, sessions, alerts, forced resets, privacy export/deletion, and armory privacy are implemented. |
| Shop, gifting, and donor history | Compact cards and details support search, sort, compare, wishlist, products, variants, reusable bundles, collections, stock, sales, coupons, gift codes, credit gifting, receipts, order history, refunds/disputes, and resumable delivery. |
| Player rewards and services | Daily cycle, loyalty, referrals/milestones, PvE/PvP/community missions, event attendance rewards, transfers with SLA, restore, unstuck, rename, customization, race/faction changes, gold, bags, mounts, training, and populated WotLK level/gear packages are represented. |
| Generic multi-realm administration | Realm-scoped catalogs, CMS, branding, rates, rules, cross-faction flags, maintenance, downloads, competitive seasons, and operations share one portal with a validated realm switcher. Staff roles and individual permissions cover support, commerce, content, credits, moderation, realm operations, monitoring, audit, and administration. |
| Operational safety | Versioned locked migrations, least-privilege grant documentation, read-only container defaults, health/readiness/metrics, structured audit IDs, retention, step-up checks, delivery diagnostics, reconciliation, backup/restore runbook, and MySQL/MariaDB matrices are implemented. |

The only intentionally non-automatic pieces are operator-owned facts or
external systems: legal wording, genuine download binaries/signatures, real
Discord/Google/Stripe credentials, production AzerothCore schemas, worldserver
SOAP behavior, and an actual restore rehearsal. The portal exposes the fields,
contracts, readiness warnings, and runbooks for these; manufacturing evidence
for them in mock mode would be misleading.

- A real browser completed all 46 audited routes at a 1440×1000 viewport in
  both the authored dark theme and the optional light theme, with no horizontal
  overflow, duplicate IDs, missing accessible control names, broken tab
  relationships, visible runtime failures, uncaught JavaScript exceptions, or
  WCAG AA text-contrast failures. Theme transitions are explicitly settled so
  the audit measures final rendered colors instead of an interpolated frame.
- Stateful browser journeys passed for registration, password recovery,
  authentication, shop purchase, ticket submission, character restoration,
  staff step-up, privacy-aware linked-account investigation, evidence
  attachment, destructive moderation, bulk order retry, failed-step
  reconciliation, CSV catalog validation, and reusable-bundle editing.
- The production Astro build generated all 23 static entry points.
- `go test ./...`, `go test -race ./...`, `go vet ./...`, JavaScript syntax
  checks, and `git diff --check` passed.
- Schema version 42 and its integration suites passed on MySQL 8 and MariaDB
  10.11, including launcher patches, event rewards, vote-network cooldowns,
  and community vote goals. Database-backed packages must run serially because they intentionally
  recreate shared fixture schemas; CI now uses `go test -p 1` to prevent DDL
  lock contention from masquerading as a migration failure.
- A source-level SQL-injection review found no request-controlled value
  interpolated into a query. Player and staff input is passed as driver
  parameters. The remaining dynamic query text is limited to database names
  validated at startup by `^[A-Za-z0-9_]+$`, fixed allow-listed columns and
  directions, generated `?` placeholder lists, schema-migration constants, and
  bounded integer limits. This conclusion is reinforced by the MySQL/MariaDB
  integration suites, but it is not a substitute for a production penetration
  test.
- The final local integrity pass on 2026-08-30 passed `go test ./...`,
  `go vet ./...`, `npm run test:api-contract`, `npm run build`, and
  `git diff --check`. The contract currently covers 253 production routes and
  249 mock routes, with four signed callbacks intentionally production-only;
  Astro generated all 23 static pages.
- The repeatability pass found and fixed two test-harness leaks: an existing
  event reservation no longer changes the expected Community state, and stale
  screenshots are removed before capture. A clean rerun passed all 46 routes,
  both themes, strict contrast, stateful journeys, and the approved visual
  threshold.
- The documented restricted-database grants now cover all 100 portal tables,
  including the newer identity, passkey, merchandising, moderation,
  competition, and reward tables. `npm run test:db-grants` derives the table
  set from numbered migrations and CI rejects missing or duplicate grants.
- API-provided links used by receipts, notifications, moderation evidence,
  guild recruitment, events, client downloads and mirrors, signatures,
  security reports, changelogs, addons, and WeakAuras are now passed through a
  browser-side HTTP(S)-only URL policy before entering an `href`. This adds a
  second boundary behind server validation and prevents unsafe schemes from
  becoming executable if stored data is corrupted. CI runs
  `npm run test:frontend-security` against malicious scheme and HTML payloads.
- A real admin-routing defect was found and fixed: the route-specific catalog
  and CMS loader referenced `parseAdminRoute` outside its lexical scope. The
  resulting error had been swallowed as a toast, leaving catalog data stuck at
  “Loading.” Route detection is now local to the loader, browser timeout
  diagnostics retain page state, and unexpected bootstrap failures are no
  longer mislabeled as access denial.
- A later route-isolation pass fixed an extracted account-controller regression,
  made commerce, payment, moderation, resources, and arena managers mount only
  on their owning admin routes, and added executable assertions preventing
  those workspaces from becoming eager again.
- The catalog editor now uses progressive disclosure for uncommon pricing and
  delivery constraints, with a persistent live summary of price, contents,
  variants, and publishing state. Public shop search, sorting, filters, and
  pagination retain URL state and are covered by a stateful browser journey.
- Account and administration now have physically separate browser controllers;
  public routes do not download the 2,000-line administration controller. The
  dispatcher loads each only on its owning route, and browser assertions guard
  that boundary.
- Homepage CMS now includes operator-authored differentiators and a progression
  roadmap, while the public homepage shows the next scheduled event. Mock mode
  includes representative content for all three so an empty integration cannot
  hide layout or runtime defects.
- Google OAuth now has first-class login, secure account linking and unlinking,
  state and PKCE protection, and exact same-origin callback validation. Discord
  and Google both remain capability-gated when they are not configured.
- A dedicated public Security and responsible-disclosure center explains
  account protection, staff accountability, privacy controls, and safe
  reporting. Its configured contact is included in launch-readiness checks.
- Player retention now includes a published seven-day reward cycle, transparent
  loyalty levels, and configurable monthly PvE, PvP, and community missions.
  Mission claims are idempotent and ledgered; progression uses real achievement
  dates, signed raid/battleground telemetry, and verified votes, and reports
  unavailable telemetry instead of inventing progress.
- Guild recruitment profiles can publish a separately validated Discord invite,
  while the CMS seeds editable draft templates for server history, team,
  staff recruitment, FAQ, realm rules, terms, privacy, refunds, and client
  troubleshooting instead of publishing invented or legally unreviewed claims.
- The tools/addon and realm competition administration workspaces now load
  through separate route-owned controllers, continuing the split of the
  remaining administration hotspot.
- Account search, support triage, character-transfer administration, moderation
  investigations, privacy requests, audit-log management, staff access,
  monitoring, and realm-configuration drift now load through their own
  route-owned controllers as well.
  Their mutations disable the affected action, retain actionable errors, and
  surface request IDs where returned; browser assertions prevent these
  controllers from loading on the dashboard.
- Account search results now deep-link into a prefilled moderation workflow;
  the admin SPA router preserves query parameters instead of silently dropping
  the selected target and action.
- Browser coverage now visits every declared administration route, including
  all content, configuration, player, monitoring, and console capability
  states rather than sampling only the most common views.
- Rankings display the configured per-realm arena reward/cutoff policy instead
  of implying undocumented rewards. Admin has a launch-readiness checklist for
  downloads, community, legal, homepage, news, and recovery configuration.
- Keyboard evidence now covers the account menu's Arrow, Home, End, Escape, and
  focus-return behavior. The browser run also caught and fixed an extracted
  Community/News controller regression (`publicLink is not defined`).
- Keyboard evidence also covers Home/End activation and focus movement for the
  public Tools and Armory tablists, plus Arrow/Enter behavior for the catalog
  item autocomplete.
- Operator localization overrides now cover text, placeholders, accessible
  labels, and title attributes. The sign-in, registration, and connection
  journeys expose stable translation keys instead of limiting localization to
  global navigation.
- Screenshots were reviewed for Home, Armory, Rankings, Shop, the operations
  dashboard, catalog, and realm/homepage settings. At 1440×1000 no text clips,
  container overflow, broken hierarchy, or accidental blue/Bootstrap styling
  was observed.
- Payment administration, overview analytics and fulfillment, the privileged
  GM console, and CMS media/navigation now have route-owned controllers. The
  console no longer initializes on every administration page, and browser
  assertions verify that these privileged or expensive modules load only on
  their owning routes. Bulk retry and failed-step reconciliation remain covered
  by the stateful staff journey after the extraction.
- Shop collections now provide a useful merchandising preview instead of an
  empty-looking banner: each collection shows its product count and up to three
  included products before a player applies the filter.
- Operator localization hooks now extend through account navigation, shop
  discovery, armory search, and rankings. Placeholder and accessible-name
  overrides remain plain-text-only.
- The post-refactor production build, Go test suite, static analysis, JavaScript
  syntax checks, and all 46 browser routes and stateful journeys passed again.
- Voting, community, CMS assets, credit grants, moderation actions, and realm
  operations now initialize only on their owning routes. The remaining
  administration controller is approximately 1,000 lines instead of the
  earlier 2,000-line hotspot, and browser assertions protect those lazy-load
  boundaries.
- The contrast scanner now understands modern `color(srgb …)` values and
  translucent background composition. Footer copy, the armory failure state,
  arena metadata, class tokens, and empty gear slots pass its WCAG AA threshold
  across all 46 routes in both themes, and CI enforces strict contrast. This
  also fixed class color rules that accidentally painted the entire armory
  model stage. Repeated account journeys now also tolerate an empty/null
  deleted-character collection instead of failing the whole character view.
- The browser audit now also consumes Chromium's computed accessibility tree
  on every route and rejects unnamed interactive controls or dialogs. This
  verifies the names exposed to assistive technology in addition to the DOM
  label and relationship checks. The expanded check passed all 46 routes.
- Catalog products and curated collections can now retain operator-managed
  artwork URLs. Custom product art is rendered as a resilient backdrop while
  single items and bundles continue to display the canonical WotLK item-ID
  icon requested for item identity.
- The final catalog/editor/merchandising hotspot now lives in a route-owned
  controller, while settings/content workspace construction has its own small
  layout module. The administration bootstrap fell from 1,021 to 379 lines, and
  browser assertions prove that catalog code is absent from the overview and
  present on catalog routes. Consolidating its bootstrap also removed a
  swallowed `cfg is not defined` error from the duplicated staff-access path.
- All 46 audited routes now have reviewed screenshot baselines and CI runs a
  deterministic pixel-diff check after the browser journey. The approved set
  covers public, account, commerce, CMS, moderation, monitoring, and realm
  operations views; intentional changes require an explicit baseline update.
- Realm-scoped client downloads now carry structured release dates, system
  requirements, changelog links, VirusTotal report links, signatures, and
  SHA-256 checksums. The public onboarding screen presents that trust metadata
  beside the download, while the content workspace validates and manages it.
  Migration 37 passed clean-schema suites on both MySQL 8 and MariaDB 10.11.
- The same download catalog now exposes a versioned, realm-aware launcher
  manifest with realmlist, client build, mirrors, checksums, signatures,
  VirusTotal/changelog links, and requirements. API responses advertise their
  contract version, and CI compares all 244 production routes with the 240 mock
  routes while explicitly allowing only four signed external callbacks to
  remain production-only.
- Player management now includes permission-scoped account session revocation
  and administrator-required password resets. Both require recent staff
  step-up, disable only the active control, expose request IDs, and are audited;
  the forced-reset flow revokes existing portal sessions, emails a one-hour
  token, blocks portal login until completion, and clears the requirement only
  in the password-change transaction. Migration 38 passed MySQL 8 and MariaDB
  10.11.
- Enabling, rescheduling, or cancelling maintenance now creates an account
  notification exactly when the maintenance state changes. Raid attempt rows
  now render the class composition already retained by signed ingestion rather
  than showing only role totals.
- Installing axe-core from the public npm registry remains unavailable in this
  workstation because its corporate TLS issuer is not trusted by npm. TLS
  verification was not disabled and no private registry reference was added.
  The existing Chromium accessibility-tree, semantic DOM, keyboard, contrast,
  overflow, and visual checks continue to run; axe/manual screen-reader review
  remains a production gate rather than a falsely claimed local pass.

### Current product verdict

| Area | Score | Assessment |
|---|---:|---|
| Feature coverage | 9/10 | Onboarding, account, armory, rankings, guilds, community, voting, shop, CMS, support, moderation, privacy, monitoring, and realm operations all exist. Advanced competitive history remains integration-dependent. |
| Player UX | 8/10 | Task routes and primary journeys are coherent. Optional game metadata, authentic imagery, and some empty/provenance states still need refinement. |
| Staff UX | 8/10 | Information architecture, permissions, route-owned initialization, catalog disclosure, and launch checks are strong. Some mutations still need more durable result reports. |
| Accessibility | 8/10 | The custom browser scanner now enforces labels, tab relationships, keyboard invariants, overflow, and WCAG AA text contrast in CI. Axe-core and a manual screen-reader review remain open. |
| Security and privacy | 8/10 | Parameterized data access, scoped roles, step-up, encrypted TOTP, passkeys, auditability, retention, and privacy-aware investigations are strong foundations. No production penetration test has been performed. |
| Reliability | 8/10 | Versioned migrations, resumable delivery, reconciliation, graceful shutdown, monitoring, and cross-engine tests are present. Real SOAP, Stripe, and disaster-recovery drills remain release gates. |
| Maintainability | 8/10 | Go responsibilities are separated and the former 4,700-line frontend bootstrap is now a dispatcher backed by route modules. The remaining admin controller should be split by workspace as it grows. |

### Highest-value remaining work

1. Continue splitting the remaining administration controller by workspace.
   Account and administration are physically separate, and accounts, support,
   transfers, moderation, privacy, audit, staff, monitoring, configuration,
   competition, resources, voting, payments, console, and CMS asset managers
   mount only on their owning routes. Catalog merchandising, settings forms,
   and general CMS editing remain the next useful boundaries.
2. Refine the visual language. Keep obsidian/gold as the reference theme,
   reduce nested bordered cards and eyebrow labels, add real class/spec/faction
   imagery, and use more deliberate empty states. Light mode should be an
   accessibility alternative, not the product's defining look.
3. Improve commerce art and decision support: replace generic set/service
   placeholders with operator-owned WotLK-appropriate assets, keep package
   summaries to three benefits, and show purchase consequence, eligible
   character, and delivery expectation together.
4. Turn rankings into stories rather than tables: visible season dates,
   eligibility/cutoff rules, rating deltas, class/spec/faction marks, profile
   links, raid roster composition, and kill/wipe timelines. Preserve explicit
   provenance whenever data comes from signed ingestion rather than stock
   AzerothCore.
5. Finish accessibility evidence with axe-core when it can be installed from a
   portable public dependency source, then add keyboard journeys for menus,
   dialogs, tabs, autocomplete, catalog editing, support, and moderation.
6. Complete localization beyond navigation-level copy, add operator-authored
   legal/refund/download/checksum content, and establish reviewed screenshot
   baselines with intentional diff thresholds.

### Production release gates

The portal is suitable for a private alpha or a public free test realm. It is
not yet proven safe for unattended paid production. Before accepting real
money, run all of the following in the target environment:

1. Representative reads and migrations against the exact AzerothCore schema
   revision and database versions in use.
2. A real SOAP drill covering an offline disposable character, item batching,
   level/training/money steps, forced partial failure, resume, and manual
   reconciliation.
3. Stripe test checkout, webhook replay, partial fulfillment, refund, dispute,
   and insufficient-wallet reversal.
4. Auth/characters/world/portal backup, restore, and rollback rehearsal with a
   measured recovery time.
5. Keyboard and screen-reader review plus a production security assessment.

## Validation addendum — 2026-08-30 02:11 CEST

The latest implementation pass closes three important audit findings with executable evidence:

- High-volume admin orders, ledger, catalog, accounts, support, and audit APIs now share a bounded server-pagination contract. Their UI filters and page numbers are URL-backed, and the browser suite asserts the resulting controls.
- Signed arena ingestion now retains season, team/opponent identifiers, rating before/after, rating movement, and match duration. Signed raid ingestion now retains every kill or wipe, pull number, remaining boss health, duration, and a roster snapshot. Armory and Rankings render the provenance rather than presenting derived history as core data.
- Admin Monitoring now offers a guarded real SOAP delivery diagnostic when `DELIVERY_DIAGNOSTIC_CHARACTER` is configured. It is fixed to one low-value WotLK item, requires an offline disposable character, staff realm permission, recent step-up authentication, and exact-name confirmation, and returns both correlation/audit IDs and cleanup instructions. It does not expose arbitrary commands or direct inventory writes.

Evidence from this pass:

| Check | Result |
|---|---|
| Go unit/API suite | `go test ./...` passed |
| Static analysis | `go vet ./...` passed |
| JavaScript parse | `node --check public/app.js` passed |
| Astro production build | 22 routes built successfully |
| MySQL 8 migration/integration matrix | Passed serially, including schema version 33 and competitive ingestion |
| MariaDB 10.11 migration/integration matrix | Passed, including schema version 33 and competitive ingestion |
| Desktop browser suite | 29/29 routes passed; purchase, support, step-up, CSV validation, bundle editing, pagination, armory, and accessibility invariants passed |
| Live mock smoke | `/healthz` healthy; raid API returned speed, kill, and pull history |

This does **not** remove the real-environment release gates below. The diagnostic exists, but it has not been run against the user's production worldserver. Stripe test-mode failure drills, representative production AzerothCore schema reads, and backup/restore rehearsal still require the deployment environment.

## Complete re-audit — 2026-08-30

The portal is now a strong private-alpha product and a credible base for a public
WotLK realm. It is no longer missing the expected top-level areas: onboarding,
news, events, guilds, armory, rankings, voting, shop, account self-service,
support, moderation, content management, realm operations, and staff controls
all have real routes and workflows. The remaining gap is depth and operational
proof, not another large collection of menu items.

### Current scorecard

| Area | Score | Current assessment |
|---|---:|---|
| Player journeys | 8/10 | The main journeys are discoverable and coherent. Some advanced armory and competition data still depends on optional collectors. |
| Community and retention | 8/10 | News, events, guild recruitment, voting, referrals, daily rewards, tracker, Discord status, and changelog are represented. |
| Commerce | 8/10 | Catalog CRUD, variants, bundles, collections, coupons, stock, gifting, payment history, resumable fulfillment, and reconciliation exist. Real SOAP/payment failure drills remain mandatory. |
| Administration | 7/10 | Roles, step-up authentication, audit, support, moderation, CMS, monitoring, and realm operations are broad. High-volume tables still need consistent server pagination and bulk-result reporting. |
| UX and accessibility | 7/10 | The public hierarchy and admin task routing are much clearer. Dense forms, sparse competitive rows, monolithic client code, and incomplete keyboard/contrast automation remain. |
| Security | 8/10 | Parameterized values, constrained identifiers, encrypted TOTP, passkeys, recovery codes, scoped staff roles, CSRF/origin controls, rate limiting, and security headers are solid foundations. Production integrations have not been penetration-tested. |
| Reliability | 7/10 | Versioned migrations, graceful shutdown, idempotent ledgers, step-based delivery, health data, and browser smoke coverage exist. Representative AzerothCore and disaster-recovery exercises are still absent. |

### What the rendered UX does well

- The homepage has a clear realm identity, live status, primary onboarding call
  to action, and a useful realm chooser without looking like an admin product.
- Public navigation follows player intent: Play, Armory, Rankings, Community,
  Tools, and Shop. Account and administration are correctly separated.
- Armory profiles use canonical URLs and task tabs. Equipment, recent activity,
  talents/glyphs, achievements/raids, collections, PvP history, and guild
  activity no longer compete in one continuous page.
- Shop cards are substantially easier to scan and full bundle manifests live on
  product-detail pages. Collections, filters, comparison, wishlist, variants,
  stock, sales, and eligibility are visible before purchase.
- Admin is now a route-driven workspace with a stable sidebar, breadcrumbs,
  permission-aware sections, dashboard metrics, and contextual destructive
  confirmations.

### Remaining UX problems

1. **The interface still has a control-panel accent.** Repeated bordered white
   cards, tiny uppercase eyebrow labels, and pale form controls make the light
   theme feel generic. The dark obsidian/gold theme should remain the reference
   theme, with fewer containers and stronger Warcraft-specific art direction.
2. **The shop wallet dominates the first viewport.** Collapse credit purchase
   into a compact wallet bar or drawer so featured products appear sooner.
   Clearly label gameplay-power packages and the realm's monetization policy.
3. **Competitive pages lack narrative context.** Add rating movement, season
   dates, cutoff/reward rules, faction marks, real class/spec icons, and links
   from team members to profiles. Raid rankings need composition and wipe/kill
   timelines, not only final clear rows.
4. **Armory depth depends on metadata availability.** Production spec detection,
   complete talent spell metadata, glyph descriptions, achievement criteria,
   and collection names must degrade explicitly when DBC/imported data is
   missing. Never infer a precise specialization from an unreliable row count.
5. **Admin form consistency remains uneven.** The catalog editor now keeps
   common fields visible, moves advanced pricing/delivery constraints behind a
   named disclosure, and shows a persistent live change summary. Apply that
   interaction pattern to the remaining dense realm-operation forms.
6. **Large admin datasets are still capped lists.** Products, orders, tickets,
   accounts, audit events, stock movements, transfers, payments, and privacy
   requests need a shared server-paginated table contract with URL-backed
   search, sort, cursor/page, selected rows, and partial-failure results.
7. **Feedback is not uniformly durable.** Every mutation should disable only
   the affected action, retain validation near the field, expose a request ID,
   and offer a retry or link to the resulting job/order/ticket.
8. **The combined account/admin controller is now the frontend hotspot.** The
   public bootstrap and major public routes have been separated, and admin-only
   managers mount by route. Account and administration should be split into
   separate controllers before another large feature wave.

### Remaining functional gaps

- A guarded real-delivery diagnostic is still needed: select a designated test
  character, run one allow-listed reversible/low-value operation, show the raw
  SOAP correlation ID, and record cleanup instructions. Preflight validation
  alone does not prove worldserver permissions.
- Arena history needs durable season context and rating movement. Stock
  AzerothCore does not record matches, so this requires the signed competitive
  ingestion integration and must remain capability-gated without it.
- Raid progression needs per-attempt/per-kill ingestion, roster snapshots, and
  provenance. Achievement timestamps alone cannot prove boss-specific first
  kills or raid speed.
- Moderation needs privacy-aware linked-account investigation views, evidence
  attachments, and explicit retention/access policy before staff can safely use
  IP/device correlations.
- Localization currently covers navigation-level copy, not complete player and
  staff workflows.
- Registration, password recovery, destructive moderation, order
  reconciliation, and restore-from-backup need stateful automated journeys.
- Automated accessibility checks should use axe-core once the dependency can be
  installed without introducing the forbidden private registry into lockfile
  history. Contrast diagnostics are currently advisory because the lightweight
  scanner cannot accurately resolve gradients and alpha compositing.
- Screenshot artifacts exist, but approved pixel-diff baselines and review
  thresholds do not.

### Release decision

Use the current build for a private alpha, internal operations, or a public
free-to-play test realm. Do not call it unattended production-ready and do not
accept real-money purchases until all of these gates pass:

1. Migrations and representative reads against the exact AzerothCore schema and
   database versions used in production.
2. A real worldserver SOAP run covering an offline character, item batching,
   level/training/money steps, a forced partial failure, resume, and manual
   reconciliation.
3. A paid checkout, webhook retry, partial fulfillment, refund, dispute, and
   insufficient-wallet reversal using a Stripe test account.
4. Backup, restore, and rollback rehearsal for auth, characters, world, and
   portal-owned data, with measured recovery time.
5. Real legal, privacy, refund, contact, download, checksum, and support content.
6. Keyboard and screen-reader review of navigation, dialogs, tablists,
   autocomplete, catalog editing, support, and moderation.

## Remediation status

Implementation work following this audit has already closed or materially reduced several findings:

- Paid fulfillment is now step-based, resumable, operator-visible, and blocks unsafe automatic refunds after completed delivery work.
- Schema changes are versioned and advisory-locked; deployments can use the explicit `migrate` command with `AUTO_MIGRATE=false`.
- The failing cross-collation audit query was replaced with isolated source queries merged in Go.
- Unsupported arena archives, specialization filtering, PvP history, raid ingestion, and billing controls are capability-gated instead of presented as working.
- Public navigation now follows player tasks and includes dedicated Play and Community experiences.
- Account management has clean task routes for characters, orders/wallet, rewards, notifications, support, and security.
- Wallet debits are ledgered; players can inspect their credit history.
- Staff roles can be assigned per account in the database for support, commerce, moderation, and administration.
- Support tickets retain a conversation history and players can send follow-up replies.
- Email changes require the current password and verification of the new address.
- Native confirmation prompts were replaced with contextual, keyboard-focusable dialogs.
- Rankings filters are URL-backed and real queries use bounded server-side pages.
- Runtime operations now include graceful HTTP/worker shutdown, request correlation IDs, structured status/latency logs, bounded rate-limit state, immutable asset caching, expanded Prometheus delivery/dependency metrics, and a dependency matrix in admin monitoring.
- CI now scans the filesystem for high-severity dependency vulnerabilities, embedded secrets, and license issues.
- Authenticator secrets are AES-256-GCM encrypted at rest, legacy plaintext secrets are upgraded after a successful login, and enrollment now issues ten hashed one-time recovery codes. State-changing staff APIs require a fresh password and, when enabled, second factor every ten minutes.
- Successful logins from a previously unseen IP and user-agent pair create an account security notification.
- Shop cards now lead to canonical product URLs with complete bundle contents and account-aware character eligibility reasons before purchase.
- Players can download a JSON data export and open or cancel a tracked deletion/anonymization request for staff review.
- News now has canonical slugs and public list/detail APIs, draft/publish/archive states, safe full-body rendering, cover images, authors, tags, previews, and immutable editor-attributed revision snapshots.
- Support now includes category and priority triage, SLA deadlines, self-assignment, tags, staff-only notes, canned replies, public/staff message separation, status filters, and an immutable event timeline.
- CI now executes every numbered migration twice against clean MySQL 8 and MariaDB 10.11 services; the same matrix passed locally on both engines on 2026-08-29.
- Admin sections now load their data on route demand instead of eagerly loading nearly every dataset, update document titles and breadcrumbs, expose `aria-current`, and return keyboard focus to the active heading. Reduced-motion preferences are honored globally.
- Operators can publish reusable custom pages with clean URLs, SEO metadata, safe text rendering, navigation/footer placement, and revision snapshots. A realm event calendar now has public upcoming-event cards and staff scheduling controls.
- A production-oriented backup, restore, rollback, and quarterly rehearsal runbook now covers all three AzerothCore databases and portal order reconciliation.
- Character transfers now have an honest staff-reviewed request workflow with source proof, duplicate-request protection, player-visible status/notes, staff audit entries, and account notifications; it does not claim unsafe automatic cross-server imports.
- Credit packs are now database-configurable per realm with admin management and legacy environment fallback. Checkout supports gifts to an existing account, records purchaser/recipient/message, and notifies the recipient after an idempotent paid webhook.
- Staff can generate limited-use, expiring gift codes; only SHA-256 hashes and a short hint are retained, redemption is transactional and one-per-account, and credits enter the normal wallet ledger.
- Staff authorization now includes Support, Shop Manager, Moderator, Content Manager, Realm Operator, and Administrator roles with per-realm or global scope, optional expiry, explicit permission overrides, scoped removal, and an admin editor. Existing GM-level mapping remains as a compatibility fallback.
- Realm settings are explicitly identified as display metadata when no agent is configured. An optional HTTPS-authenticated, per-realm, allow-listed agent contract now compares desired and observed rates/cross-faction settings, reports drift and restart requirements, requires staff step-up before apply, and records applications in the audit log without exposing file paths or arbitrary commands.
- CI now drives a real headless Chromium session through the home, onboarding, canonical armory, rankings, guilds, shop, community, account, admin, and realm-configuration routes. It authenticates through the mock API, checks core accessible-name/document invariants and horizontal overflow, and retains a screenshot of every journey as an artifact.
- The community hub now includes a canonical public suggestions and bug tracker with authenticated submissions, searchable/filterable status views, one-vote-per-account prioritization, discussion threads, public staff responses, labels, priorities, staff triage, and mock/API/browser regression coverage.
- Guild pages now support staff-managed recruitment profiles, authenticated applications from eligible characters, duplicate prevention, player withdrawal and history, an applicant-visible decision response, private staff notes, notifications, audit events, and a filtered staff inbox.
- Realm operators can now capture immutable arena-season snapshots before a reset; archived 2v2, 3v3, and 5v5 ladders retain rosters and are selectable through shareable ranking URLs. Live arena member loading was also changed from one query per team to one batched query per page.
- A public Tools area now provides a live WotLK `item_template` search, a shareable DBC-backed talent planner with point and tier constraints, and a searchable staff-curated addon/WeakAura library with draft/publish/archive workflow.
- Character owners can now hide a character, equipment, or activity from the public armory. The profile also exposes achievement category hierarchy, reputations, earned titles, mounts, and companions when the corresponding AzerothCore DBC data is installed.
- Stripe checkout, refund, and dispute events now use an idempotent transaction ledger. Partial refunds reverse proportional credits, won disputes restore held credits exactly once, insufficient wallets enter manual review, and the behavior is exercised against MySQL and MariaDB.
- Staff audit records now expose stable event/source IDs, realm and request correlation, IP/user-agent context, structured before/after/metadata fields, complete server-side CSV export, automatic request coverage for successful state-changing admin calls, and configurable event/network-identifier retention.
- Armory collections now include learned profession recipes with WotLK spell metadata and required skill levels when AzerothCore's DBC tables are installed.
- Signed-in players can maintain realm-specific wishlists, and compare up to three shop packages side by side before purchasing.
- Earned achievements now expose DBC-backed objectives and recorded criterion counters instead of only a name and external link.
- The competitive ingestion contract now accepts idempotent battleground results and per-character scoreboards; armory PvP history presents arena and battleground matches with explicit archive provenance.
- Realm moderators can exclude named characters, arena teams, or guilds from public ladders with reasons and optional expiry. Exalted-reputation, mount, and companion rankings are available only when their required source tables are detected.
- Stripe transactions now retain purchaser/recipient, amount, currency, receipt, refund, and dispute state. Signed webhook events are independently idempotent; refunds and lost disputes reverse credits exactly once or enter an explicit reconciliation state when the wallet cannot safely absorb the reversal. Players can view receipts and commerce staff can request guarded full refunds.
- A portal master identity can now own multiple AzerothCore game accounts. Linking verifies the target password and its authenticator when enabled; switching, renaming, primary-account changes, and unlinking require recent authentication and preserve active/primary safety invariants.
- Capability-gated Discord OAuth now supports explicit account linking and sign-in without matching accounts by email. The flow uses hashed expiring state, PKCE, an exact same-origin callback, provider-user uniqueness, and audited link/unlink/login events; unconfigured portals expose no Discord controls.
- Database-backed WebAuthn passkeys now provide discoverable, user-verified ES256 registration and sign-in for master identities. Challenges are one-time and expiring, RP/origin/authenticator flags and signatures are verified server-side, credential counters are tracked, and passkey management requires recent authentication.
- Content staff now have a realm-scoped managed image library. Uploads are size-, MIME-, decoder-, dimension-, and pixel-count validated, deduplicated by content hash, served through immutable same-origin URLs, previewed before upload, and retain editable alternative text and audit history.
- Operators can replace the built-in header and footer per realm with ordered, validated internal or HTTPS links, including explicit new-tab behavior. A capability-gated HTTPS analytics script integration updates CSP only for its exact configured origin.
- The dense content and settings screens are now task-routed workspaces with shareable URLs. Content is separated into news, pages/navigation, media, community, events, downloads, addons/auras, and voting; settings are separated into branding, homepage, realm profile, links/legal, features, and maintenance. Unrelated admin datasets are no longer fetched on every content route.
- Browser coverage now spans 27 public, account, and staff routes and exercises authenticated purchase, support submission, staff step-up, CSV validation, and reusable-bundle editing. The harness provisions its own shop balance, reports mutation errors instead of timing out, and closes its Chromium target after every run to avoid test-memory growth.
- Fast support-form submissions no longer race public capability loading, and server failures shown in the UI now include their request correlation ID for operator support.

## Current release verdict

The remediated build is appropriate for a private alpha or a public realm that does not yet accept real-money payments. It is not yet a responsible unattended production release for paid commerce. The remaining release gates are representative AzerothCore schema fixtures, a tested backup/restore procedure, end-to-end tests against an actual worldserver SOAP endpoint, and browser accessibility/regression coverage.

| Current area | Score | Release assessment |
|---|---:|---|
| Public/player journeys | 7/10 | Joining, community, voting, account, armory, rankings, and shop have coherent routes; content depth remains uneven. |
| Administration | 6/10 | Granular roles and reconciliation exist; realm operations, support workflow depth, and table scalability remain incomplete. |
| UX and accessibility | 6/10 | Navigation and spacing are substantially clearer; the monolithic frontend and lack of browser/axe regression tests remain the largest UX risks. |
| Security | 8/10 | Encrypted TOTP, recovery codes, step-up authentication, secure headers, bounded throttling, and CI scanning are in place; production integration and operational validation remain. |
| Reliability and commerce | 7/10 | Resumable fulfillment, migrations, graceful shutdown, correlation IDs, and expanded metrics are implemented; real integration validation is still absent. |
| Deployment readiness | 7/10 | The container and CI posture are solid, but backups, restore drills, and production dependency validation are not proven. |

## Latest rendered-interface review

The 1440×1000 Chromium capture set was reviewed after the remediation work. The interface is now structurally consistent and free of the earlier horizontal overflow and broken-control issues, but it still feels more like a clean SaaS control panel than a finished game community.

- The homepage has the strongest identity. Its art, hierarchy, status summary, and primary action work; the generic slogan should be replaced by factual realm differentiators supplied by the operator.
- The armory hid the requested paper doll below summary panels. Equipment now renders first, but the profile still needs collection views for reputations, mounts, companions, and titles.
- Rankings are legible, but rows are visually sparse and member metadata is too small. Class crests, faction markers, rating movement, eligibility rules, and season context would make the ladder easier to understand.
- The shop credit area consumes too much horizontal space and separates package selection from its consequence. Product cards are clearer than before, though generic package art and long bundle summaries still weaken merchandising.
- Account navigation is the clearest task-oriented area. The overview correctly limits itself to likely next actions.
- Admin information architecture is substantially better, but dense forms still need progressive disclosure. The rendered support screen exposed narrow action buttons wrapping mid-word; action labels are now forced to remain intact.
- Light mode has sufficient structural separation in the audited views, but it loses most of the Warcraft character of the dark obsidian/gold theme. It should remain an accessibility preference rather than the visual reference theme.
- Automated checks cover labels, names, tabs, overflow, visible runtime errors, console exceptions, and screenshots. They are not a replacement for axe-core, keyboard journey tests, or pixel-diff visual regression.

### Go-live gates

1. Validate schema migrations and representative reads against the exact AzerothCore, MySQL, and MariaDB versions you support.
2. Exercise a complete paid order, partial SOAP failure, reconciliation, and refund in a disposable real realm.
3. Validate staff step-up authentication, encrypted TOTP storage, and recovery-code handling against the production database and operating procedures.
4. Publish real legal, privacy, refund, contact, download, checksum, and support information.
5. Add automated browser journeys for registration, login, armory, purchase, ticketing, and staff reconciliation, including accessibility checks.
6. Document and rehearse database backup and restore before taking player payments.

Items in the sections below remain the original point-in-time findings unless explicitly listed above; they continue to serve as the product backlog.

## Executive assessment

The portal is a credible prototype with an unusually broad feature surface, but it is not yet a polished generic server platform. Registration, account security, basic armory data, rankings, guilds, catalog management, payments, staff operations, multi-realm selection, health endpoints, and a mock environment all exist. The main problem is no longer the number of screens: it is that incomplete integrations are presented beside production-ready features with almost the same visual weight.

The current experience feels like an operations dashboard styled as a game website. It needs a clearer player journey, a smaller and more coherent navigation model, honest capability states, and task-focused admin workflows. Paid shop delivery also needs stronger idempotency before it should be considered production-safe.

### Scorecard

| Area | Score | Assessment |
|---|---:|---|
| Public/player feature coverage | 6/10 | Broad, but several features are shallow or depend on data that is never collected. |
| Administration | 5/10 | Many controls exist, but roles, configuration, support, and commerce workflows are incomplete. |
| UX and information architecture | 4/10 | Dense, inconsistent, and difficult to scan; most behavior lives in one imperative script. |
| Accessibility | 4/10 | Basic labels exist, but dynamic widgets, focus management, status semantics, and keyboard behavior need work. |
| Security foundations | 7/10 | Parameterized SQL, constrained schema identifiers, secure cookies, rate limiting, TOTP, headers, and restricted SOAP are good foundations. |
| Reliability and commerce safety | 5/10 | Health checks and transactional order creation are good; delivery retries can duplicate partial fulfillment. |
| Maintainability | 4/10 | The Go code is reasonably separated, but the frontend is effectively a 3,000-line monolith with duplicated initialization. |
| Deployment readiness | 6/10 | Rootless read-only image, CI, SBOM, and multi-arch publishing are good; migration and integration testing need improvement. |

Recommendation: suitable for a private alpha or a free test realm. Do not enable real-money credit sales until the delivery and refund workflow is made idempotent and tested against a real AzerothCore database/worldserver pair.

## What was verified

- Astro production build succeeds.
- `go test -race ./...` succeeds across 46 tests.
- `go vet ./...` succeeds.
- JavaScript syntax validation succeeds.
- All primary mock APIs returned successful responses after authentication, including armory, rankings, guilds, shop, account, monitoring, catalog, audit, console, and support.
- Clean URLs for known admin subroutes are served correctly.
- Docker uses a non-root user, a read-only filesystem, dropped capabilities, a readiness health check, and a multi-stage build.

No real AzerothCore integration environment was available during this audit. Database compatibility, SOAP permissions, actual mail delivery, real character training, and Wowhead model behavior therefore remain integration risks.

## Highest-priority findings

### P0 — Shop delivery is not idempotent

An order can execute several SOAP commands: multiple item mails, a level command, money, and a character service. If an early command succeeds and a later command fails, the order moves to review. Retrying replays the entire sequence and can duplicate already delivered items or gold. Refunding a partially delivered order can also leave the player with both delivered value and returned credits.

Required fix:

- Persist one fulfillment step per mail, money grant, level change, training operation, and service.
- Give every step a durable state and an operator-visible response.
- Retry only incomplete steps.
- Distinguish `not_started`, `partial`, `delivered`, `compensated`, and `manual_review`.
- Prevent automatic refunds after any non-reversible step without an explicit staff reconciliation flow.

### P0 — Realm configuration is presented as functional configuration but is metadata

XP, drop, reputation, honor, profession, realm type, and cross-faction controls only change portal display values. They do not update `worldserver.conf`. This is easy for an operator to misunderstand and creates configuration drift.

Required fix:

- Rename the current section to **Displayed realm profile** immediately.
- Add an optional, separately authenticated realm-management agent that owns a strict allow-list of AzerothCore settings.
- Show `configured`, `observed`, `pending restart`, and `drifted` values separately.
- Support validation, a generated diff, backup, apply, `reload config`, and explicit restart-required states.
- Never give the public portal an unrestricted file editor or Docker socket.

### P0 — Several ranking and armory controls overpromise

- The arena season selector contains hard-coded seasons; selecting an archive displays an empty explanation rather than loading a snapshot.
- Real specialization filtering cannot work: the API never populates the specialization field before applying the filter.
- Talent “points” count stored spell rows, not actual ranks spent in each tree.
- Talent and glyph views expose raw spell IDs instead of names, icons, ranks, and descriptions.
- The achievement browser exposes raw IDs and links out to Wowhead rather than behaving like an achievement browser.
- Stock AzerothCore does not provide arena match history, so the real endpoint always returns an empty list.
- Raid speed rankings rely on `portal_raid_kills`, but no ingestion endpoint, addon, parser, or game script populates it.
- Raid progression maps section achievements to every boss in that section, so it cannot prove individual boss kill dates.

Required fix: hide unavailable controls based on backend capabilities, then implement data collectors/providers before re-enabling them. The UI should distinguish live core data, derived data, imported snapshots, and unavailable data.

### P0 — Runtime migrations are not a sufficient migration system

The process performs `CREATE TABLE`, column checks, indexes, data rewrites, and enum alterations at startup. There is no schema-version table, migration lock, ordered history, dry run, or rollback guidance. This has already caused MySQL-version compatibility failures.

Required fix: introduce numbered, idempotent migrations with a schema version, database advisory lock, preflight report, and tested MySQL/MariaDB compatibility matrix. Keep startup capable of detecting an outdated schema, but make mutation an explicit deployment step.

### P1 — Audit history failure

The audit endpoint used a cross-table SQL `UNION` across AzerothCore and portal strings. Different table collations can make that query fail. The local fix queries each source independently, merges and sorts in Go, and logs the exact failing source and database error. This fix is currently uncommitted.

## UX findings

### Visual system

- WebcoreUI is imported globally but only its Button component is used twice. The rest of the interface is custom HTML/CSS, so adopting WebcoreUI did not create a consistent component system.
- The compiled stylesheet is about 158 KB and combines WebcoreUI with three custom style layers. There are 32 `!important` declarations, a sign that layers are fighting each other.
- Icons are mostly text glyphs and Unicode symbols. They vary by platform and contribute to the unfinished appearance.
- Typography relies heavily on tiny uppercase eyebrow labels. It looks decorative but reduces scanability and makes unrelated sections feel identical.
- Cards, panels, tables, and forms use too many subtly different spacing and border rules. There is no small documented token system for density, elevation, radius, or semantic color.
- Light mode exists, but there is no automated contrast or visual-regression coverage.

Recommended direction: a restrained obsidian/gold player theme with one surface hierarchy, one spacing scale, real icons, fewer borders, and clearer typography. Use WebcoreUI consistently for dialog, tabs, table, pagination, alert, select, menu, and form controls—or remove it and own a smaller design system. Mixing both is the worst option.

### Information architecture

The public navigation gives every dataset equal prominence. A player usually arrives to answer one of four questions: how to join, whether the realm is online, what is happening, or how their account/characters are doing.

Recommended public navigation:

1. Home
2. Play (connection guide, downloads, rules, FAQ)
3. Armory
4. Rankings
5. Community (news, guilds, events, Discord)
6. Shop

Recommended account navigation:

- Overview
- Characters
- Orders and wallet
- Rewards
- Support
- Security

Recommended admin navigation:

- Dashboard
- Players
- Shop
- Content
- Realms
- Support
- Staff and audit
- System

The standalone Realm page mostly repeats homepage status and could become a detailed realm page under Play or a realm-specific landing page.

### Navigation and state

- Public pages perform full reloads while only the admin area behaves like an SPA.
- Armory characters use `/armory?q=Name` instead of canonical `/armory/Name` URLs.
- Guild details, filters, ranking metric, bracket, class, faction, and selected tab are not consistently encoded in the URL. Refreshing or sharing loses context.
- Unknown paths return the homepage with HTTP 200 instead of a real 404 page.
- Admin route changes do not update page title, announce navigation to assistive technology, or move focus to the new heading.
- There is no breadcrumb or task context in nested admin screens.

### Forms and feedback

- Destructive operations use native `prompt()` and `confirm()` dialogs. They cannot explain consequences, show the selected target clearly, or present recovery options.
- Forms do not consistently warn about unsaved changes.
- Validation is mostly shown only after submission, and some API failures become a transient toast with no persistent recovery path.
- Long-running delivery actions do not have a step timeline or live status.
- Empty states explain that data is missing but rarely tell the operator how to configure or ingest it.
- Credit packages remain visible when Stripe is not configured; the failure appears only after clicking.

### Admin experience

- The admin page contains every view in one HTML document and injects additional configuration sections from JavaScript after load.
- Admin initialization loads several datasets more than once and fetches data for views the user has not opened.
- Large tables are client-filtered fixed-size result sets rather than server-paginated tables.
- Catalog, audit, order, ticket, player, and ledger workflows lack stable URL query state.
- Bulk actions have minimal previews and no structured result report for partial failures.
- Settings mix brand, content, feature flags, maintenance, gameplay metadata, and realm policy into one long form.

Split settings into Branding, Homepage, Features, Realm profile, Integrations, Email, Payments, and Maintenance. Load each admin route on demand.

### Accessibility

- Add a skip link and visible keyboard focus treatment across all custom controls.
- Use semantic tables for tabular data rather than repeated generic `div` rows.
- Give filter inputs persistent labels instead of placeholder-only descriptions.
- Implement keyboard behavior and focus return for menus, dialogs, tabs, autocomplete results, and toasts.
- Add `aria-current` to navigation and announce asynchronous loading/errors.
- Do not communicate rarity, status, faction, or online state through color alone.
- Add reduced-motion support and automated axe checks.

Mobile work was intentionally not prioritized in this audit, based on the current product direction.

## Missing or incomplete player features

### Account and identity

- Change and re-verify email address.
- Recovery codes for TOTP and an administrator-assisted recovery workflow.
- Account event notifications and optional login alerts.
- Download/export personal account data and account deletion/anonymization workflow.
- A clear wallet ledger showing purchases, grants, refunds, votes, daily rewards, and Stripe payments.
- Notification inbox for deliveries, tickets, sanctions, maintenance, and rewards.
- Realm-aware account/character limits and account status explanation.

### Character services

- Service eligibility preview before purchase.
- Per-service status timeline and failure explanation.
- Cooldowns, realm restrictions, configurable prices, and staff approval policies.
- Name availability checking for rename.
- Race/class/faction compatibility preview.
- Transfer workflow is absent.

### Armory

- Real talent-tree layout, ranks, spec detection, glyph names/icons, and tooltips.
- Enchants, gems, sockets, set bonuses, durability, item comparison, and aggregate stats.
- Achievement categories, points, criteria, icons, descriptions, and search by name.
- Professions with recipes and progression.
- Mounts, companions, titles, reputations, statistics, and recent activity.
- Privacy controls and an operator policy for hidden characters.
- Canonical character URLs and share metadata.
- A maintained model-viewer integration contract; the current Wowhead/jQuery runtime dependency remains externally fragile.

### Rankings and competition

- Correct spec detection and filters.
- Pagination and player/guild search.
- Durable arena season snapshots and a season rollover job.
- An arena-match collector if match history is advertised.
- Raid encounter ingestion, composition snapshots, wipe/kill history, and verified speed-run rules.
- Clear ranking eligibility rules and staff disqualification controls.
- Separate character, guild, arena, PvE progression, and recent activity views.

### Community and content

- Full news articles with slugs, rich but sanitized content, cover image, author, tags, drafts, and preview.
- Events calendar and scheduled realm events.
- Server changelog with versions/categories instead of one text field.
- Real Discord widget/status integration.
- FAQ, rules pages, staff page, ban list policy, and contact information.
- Guild recruitment profiles and applications.
- Localization beyond the small set of navigation strings.

## Missing or incomplete shop features

- Idempotent fulfillment ledger and reconciliation UI.
- Product detail pages with complete bundle preview and eligibility.
- Configurable credit packages rather than three hard-coded Stripe packages.
- Tax/legal receipt strategy, payment history, chargeback state, and webhook reconciliation.
- Gift codes, gift purchases, and manual invoice notes.
- Product variants, realm overrides, visibility segments, tags, collections, and reusable bundles.
- Stock adjustment history and reservation release on refund.
- Sale/coupon conflict rules and coupon edit/deactivation history.
- Server-side search, pagination, ordering, and CSV import with validation preview.
- Test-delivery action and package validation against the selected realm's `item_template`.
- Explicit policy for direct database delivery versus SOAP delivery. Direct character-table writes should remain limited to carefully tested offline-only operations.

## Missing or incomplete administration

### Staff and authorization

- Staff roles are inferred from AzerothCore GM levels plus an environment-variable shop-manager list. There is no portal role CRUD, custom permission matrix, temporary access, realm scope, or staff invitation flow.
- Add database-backed roles: Support, Moderator, Shop Manager, Content Manager, Realm Operator, and Administrator.
- Require step-up TOTP for destructive actions, credit grants, refunds, role changes, console access, and configuration apply.
- Add staff session revocation and forced password reset.

### Audit and moderation

- Audit entries need event IDs, IP, user agent, realm, request correlation ID, before/after values, and structured metadata.
- Several sensitive operations do not create a complete audit event, including retry/refund and security changes.
- Add sanction history, evidence/notes, internal comments, expiry jobs, appeal state, and linked accounts/IPs with strict privacy controls.
- Add saved audit filters and server-side CSV export for complete datasets.

### Support

- Tickets need conversation threads rather than one response field.
- Add category, priority, assignment, tags, internal notes, canned replies, escalation, SLA timestamps, and email/Discord notifications.
- Preserve a full immutable ticket history.

### Realm operations

- Add a safe configuration agent and observed/configured drift view.
- Add scheduled announcements/restarts with cancellation and history.
- Add online-player lookup, queue state, latency, crash/restart history, SOAP reachability, database pool health, and delivery-worker state.
- Make raw console access a separate high-risk permission with mandatory 2FA and session re-authentication.

### CMS and branding

- Add draft/preview/publish workflows and revision history.
- Add navigation/footer management, custom pages, SEO fields, social image, analytics integration, and asset upload/storage.
- Validate external artwork dimensions and show previews rather than accepting bare URLs only.

## Engineering and operational findings

### Frontend architecture

- `public/app.js` is about 3,100 lines and uses roughly 150 direct `innerHTML` assignments.
- There are no frontend unit, component, browser, accessibility, or visual-regression tests.
- WebcoreUI is mostly bundled rather than used as the component foundation.
- The compiled UI ships about 116 KB of application JavaScript and 158 KB of CSS without explicit immutable cache headers.
- Admin initialization contains duplicated calls and duplicated assignment code.

Recommended fix: split page behavior into typed modules, use Astro components for stable markup, and use small framework islands only where state is needed. Establish reusable table, form, dialog, tabs, empty-state, loading, error, tooltip, and item components. Add Playwright journeys and screenshot comparisons.

### Backend and data

- Add real MySQL/MariaDB integration tests for supported AzerothCore schemas.
- Remove arena ranking N+1 queries by fetching members in one query.
- Stop silently swallowing optional armory query errors; return capability/error metadata.
- Add pagination cursors and bounded filters to large endpoints.
- Add request IDs and include response status/latency in structured request logs.
- Add graceful shutdown so in-flight requests and delivery work can finish.
- Bound and periodically prune in-memory rate-limit keys, or use a shared limiter for multiple replicas.
- Version public and admin API contracts or generate an OpenAPI schema.

### Observability

- Existing health, readiness, and basic Prometheus metrics are a good start.
- Add database latency, SOAP latency/fault count, queue depth/age, delivery outcomes, webhook outcomes, login failures, rate-limit hits, and email failures.
- Add an operator-facing dependency matrix showing configured, reachable, authorized, and last successful operation.
- Add alert recommendations and a documented backup/restore test.

### Security hardening

- Encrypt TOTP secrets at rest and provide recovery codes.
- Reduce the CSP dependency on `unsafe-inline`; pin or self-host third-party runtime assets where licensing permits.
- Add step-up authentication for high-risk staff workflows.
- Define retention and redaction policies for audit logs, IPs, support messages, and SOAP responses.
- Add dependency/license scanning and secret scanning to CI.
- Add an integration security test for proxy headers, CSRF assumptions, and multi-realm authorization boundaries.

## Recommended delivery sequence

### Phase 1 — Trust and correctness

1. Idempotent order fulfillment and safe reconciliation.
2. Versioned migrations and real AzerothCore integration tests.
3. Audit completeness and the current audit endpoint fix.
4. Capability flags that hide non-functional season, spec, PvP-history, and raid-speed controls.
5. Rename realm metadata and design the optional configuration agent.

### Phase 2 — UX foundation

1. Define tokens and reusable WebcoreUI-based components.
2. Replace the frontend monolith with route modules/components.
3. Rebuild information architecture and admin settings sections.
4. Add canonical detail routes and URL-synchronized filters/tabs.
5. Add proper dialogs, empty states, focus behavior, and a 404 page.

### Phase 3 — Complete core player journeys

1. Account overview, wallet ledger, notifications, and service timelines.
2. Product detail and checkout eligibility experience.
3. Full support conversations and notifications.
4. News/article CMS, events, Discord, rules, and download guide.

### Phase 4 — Real armory and competition

1. WotLK metadata provider for talents, glyphs, achievements, items, enchants, and gems.
2. Canonical character and guild profiles.
3. Arena season snapshot pipeline.
4. Optional PvP match and raid-event ingestion modules.

### Phase 5 — Generic server operations

1. Database-backed staff roles and realm scopes.
2. Safe realm configuration agent with drift detection.
3. Expanded monitoring, alerts, scheduled operations, backup guidance, and integration diagnostics.

## Product definition for 1.0

A feature should only appear as supported when the portal can identify its data source, report whether that source is available, exercise it in the mock environment, and validate it in a real AzerothCore integration test. Following that rule will make the product feel much more complete even before every optional module exists, because players and operators will no longer encounter polished controls that lead to placeholders or empty data.

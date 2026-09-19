# Deploying the portal

## Pipeline

1. Push to `main`.
2. GitHub Actions (`.github/workflows/ci-container.yml`) runs the test job
   (`npm ci`, `npm run build`, `npm audit --omit=dev --audit-level=high`,
   `go test -race ./...`, `go vet ./...`, `govulncheck`) and then publishes
   `ghcr.io/ghazibendahmane/azeroth-core-web` tagged `latest`, `main` and
   `sha-<commit>` for `linux/amd64` and `linux/arm64`.
3. Redeploy the Dokploy compose project **Wow → azerothcore-portal**.
4. Verify `https://valenoza.com` (see *Verification*).

## Why the compose sets `pull_policy: always`

Dokploy deploys this stack with:

```
docker compose -p <project> --env-file .env -f docker-compose.yml up -d --build --remove-orphans
```

That command never pulls: if the image tag already exists in the host's local
image store, Compose reuses it. Because the portal service is pinned to the
mutable `:latest` tag, a deploy would report success while still running the old
build — the container is left `Running`, so nothing is recreated either.

`pull_policy: always` is set on the four services built from the portal image
(`azerothcore-portal`, `-ahbot-account`, `-admin-bootstrap`, `-soap-bootstrap`).
That forces a pull on every deploy, and because it is part of the service
definition it also guarantees the container is recreated.

If you ever remove it, a deploy will silently keep serving the previous image.

## Verifying a deploy

`tools/production-check.mjs` exercises the deployed site in headless Chromium:

```bash
node tools/production-check.mjs            # defaults to https://valenoza.com
BASE_URL=http://127.0.0.1:8080 node tools/production-check.mjs
```

It asserts the rail shell renders on every route, the configured brand colour is
applied, no route overflows horizontally, there are no console errors, and the
3D armory preview reaches a correctly sized canvas with the model assets loaded
through `/modelviewer/`. It picks a character that actually exists on the target
realm, and treats a legitimately empty page (for example `/news` with nothing
published) as a pass when an empty state is rendered.

## Notes

- The portal reads `PORTAL_NAME`, `REALM_ADDRESS` and friends from the compose
  environment, which overrides the defaults in `internal/config/config.go`.
  The palette comes from the same config; `THEME_*` is not set on the compose,
  so the defaults in `internal/config/config.go` apply unless an operator saves
  different values in **Admin → Configuration** (those persist in the database
  and win over the environment).
- Before changing the compose through the API, read the current `env` first —
  the MCP tools redact it, and it holds database credentials. The raw API
  (`GET /api/compose.one?composeId=…` with an `x-api-key` header) returns it, so
  it can be backed up and passed back verbatim.

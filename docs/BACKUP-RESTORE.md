# Backup and restore runbook

This portal stores player identity and `portal_*` application state in the
AzerothCore authentication database, characters in the characters database,
and item metadata in the world database. A usable backup must be a consistent
set of all three databases.

## Policy

- Take encrypted daily backups and retain at least 14 daily and 3 monthly copies.
- Keep one copy outside the host and deployment provider.
- Back up before every portal migration or AzerothCore update.
- Use a protected MySQL option file or secret mount; do not put passwords in history.
- Record the portal image digest, AzerothCore revision, database engine/version,
  and `MAX(portal_schema_migrations.version)` beside every backup.
- Test restoration quarterly. A backup that has not been restored is unverified.

## Create a consistent backup

Put the portal in maintenance mode and stop world writes before the snapshot.
For small installations using InnoDB, a single transaction is sufficient:

```sh
mysqldump --defaults-extra-file=/run/secrets/mysql-backup.cnf \
  --single-transaction --quick --routines --events --triggers \
  --databases acore_auth acore_characters acore_world \
  | gzip > azeroth-$(date -u +%Y%m%dT%H%M%SZ).sql.gz
sha256sum azeroth-*.sql.gz > azeroth-backup.sha256
```

If databases are on different servers, coordinate a write freeze and dump each
server independently. Separately timed live dumps are not a recovery point.

## Restore rehearsal

1. Provision an isolated database host on the same engine major version.
2. Verify the checksum with `sha256sum -c`.
3. Restore into empty databases; never test against production.
4. Start the exact backed-up portal image with `AUTO_MIGRATE=false`.
5. Run `azeroth-portal migrate` only when deliberately testing an upgrade.
6. Verify `/readyz`, login, character ownership, wallet totals, open orders,
   support threads, staff roles, and the latest audit events.
7. Start an isolated worldserver and execute a disposable shop delivery through
   SOAP, including a failed-step reconciliation.
8. Record duration, errors, row counts, image digest, and sign-off. Remove the
   isolated restore only after retaining the report.

## Rollback after a failed release

Migrations are forward-only. Stop the new portal, restore the complete
pre-release database set, then deploy the previous image digest. Rolling back
only the binary while retaining a newer schema is unsupported unless tested.
Never delete rows from `portal_schema_migrations` to simulate a rollback.

## Minimum verification queries

```sql
SELECT MAX(version) FROM acore_auth.portal_schema_migrations;
SELECT status, COUNT(*) FROM acore_auth.portal_orders GROUP BY status;
SELECT COUNT(*) FROM acore_auth.portal_order_steps WHERE status IN ('executing','failed');
SELECT COUNT(*) FROM acore_auth.portal_staff_roles;
SELECT COUNT(*) FROM acore_characters.characters;
SELECT COUNT(*) FROM acore_world.item_template;
```

Resolve an order left in `executing` through admin reconciliation after checking
the target character in game. Do not blindly replay it.

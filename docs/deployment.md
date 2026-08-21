# Nodus API deployment

The current GitHub Actions pipeline validates every pull request to `main`. A
successful push to `main` builds one `linux/amd64` image, publishes it to the
private GitHub Container Registry package `ghcr.io/nodus-software/nodus-be`, and
records the immutable image reference in the workflow summary. It does not
deploy staging or production. The server deployment assets and the setup notes
below are retained for when server-based deployment is enabled again.

## One-time host preparation

Staging and production use separate Linux hosts. Each host must already have an
HTTPS reverse proxy and DNS. The proxy must forward its API hostname to
`http://127.0.0.1:8080`; the container port is intentionally not publicly bound.

Install Docker Engine with the Compose plugin, `curl`, and `util-linux` (for
`flock`). Create a dedicated, key-only SSH deployment user, add it to the Docker
group, and create its deployment directory. The examples below use
`/opt/nodus`:

```sh
sudo install -d -m 750 -o nodus-deploy -g nodus-deploy /opt/nodus
```

Authenticate once as that deployment user to the private GHCR package using a
dedicated token with read-only package access:

```sh
printf '%s' "$GHCR_READ_TOKEN" | docker login ghcr.io \
  --username YOUR_GITHUB_SERVICE_ACCOUNT --password-stdin
```

Do not use a developer's broad personal token. Restrict the package so the
service account can read only the Nodus image where the GitHub plan supports
granular package access.

## Host environment file

Create `/opt/nodus/.env` independently on each host, owned by the deployment
user and set to mode `0600`. GitHub Actions never creates or replaces this file.
It must contain the complete application production configuration, including:

```dotenv
APP_ENV=production
BASE_URL=https://example.com
ALLOWED_ORIGINS=https://example.com
TENANT_BASE_DOMAIN=example.com
TENANT_URL_SCHEME=https
TENANT_URL_PORT=
RESERVED_ORGANIZATION_SLUGS=

DB_URL=postgres://app_user:encoded-password@db.example.com:5432/nodus?sslmode=verify-full
DB_NAME=nodus
DB_USER=app_user
DB_PASSWORD=encoded-password
DB_SSL_MODE=verify-full
MIGRATION_DB_URL=postgres://migration_user:encoded-password@db.example.com:5432/nodus?sslmode=verify-full

JWT_SECRET=replace-with-a-long-random-secret
MFA_ENCRYPTION_KEY=replace-with-a-base64-encoded-32-byte-key
REFRESH_COOKIE_SECURE=true
WEBAUTHN_RP_ID=example.com
WEBAUTHN_ORIGINS=https://example.com

REDIS_ADDRESS=redis.internal:6379
REDIS_PASSWORD=replace-me
AUTH_SECURITY_MODE=observation
AUTH_RATE_LIMIT_HMAC_SECRET=replace-with-an-independent-random-secret
TURNSTILE_SECRET_KEY=replace-with-the-cloudflare-secret-key

EMAIL_PROVIDER=zeptomail
EMAIL_FROM_NAME=Nodus Health
EMAIL_FROM_ADDRESS=no-reply@example.com
EMAIL_HTTP_TIMEOUT=10s
ZEPTOMAIL_API_URL=https://api.zeptomail.com/v1.1/email
ZEPTOMAIL_SEND_TOKEN=replace-me
RESEND_API_URL=https://api.resend.com/emails
RESEND_API_KEY=replace-me
EMAIL_LOGO_URL=https://res.cloudinary.com/dwjwrng2b/image/upload/v1786552657/full_logo_no_bg_eqgkg0.png
```

Include any other settings from `config/config.go` that differ from their safe
defaults. URL-encode reserved characters in PostgreSQL credentials. The
migration role must be able to create and alter the schema and install the
`pg_trgm` extension. The runtime role should have only the permissions needed
by the application. Both agreed databases are empty, so the first deployment
will apply migrations 1 through 12 and create `schema_migrations`.

Transactional email is queued in PostgreSQL and delivered by the API's
background worker. Configure both ZeptoMail and Resend credentials if operators
must be able to switch providers by changing only `EMAIL_PROVIDER` and
redeploying. The selected provider's sender domain must be verified first.

## Grafana Cloud Logs

Create `/opt/nodus/alloy.env` independently on each host, owned by the
deployment user and set to mode `0600`. Keep it separate from `.env` so the API
container never receives the Grafana Cloud credential:

```dotenv
LOKI_URL=https://logs-prod-xxx.grafana.net/loki/api/v1/push
LOKI_USERNAME=replace-with-the-logs-tenant-id
GRAFANA_CLOUD_API_KEY=replace-with-a-log-write-only-access-policy-token
LOKI_ENVIRONMENT=production
LOKI_INSTANCE=prod-api-01
```

Use `LOKI_ENVIRONMENT=staging` and a distinct stable instance name on the
staging host. Copy the Loki URL and tenant ID from the **Send logs** details for
the Grafana Cloud stack. Create an access policy token with log ingestion
permission only.

The API continues writing JSON to `/app/logs/app.log`, backed by the
`nodus_logs` volume. Grafana Alloy mounts that volume read-only, extracts the
record timestamp and level, and sends the complete JSON line to Loki. Alloy's
positions and delivery state are persisted in `alloy_data`, allowing collection
to resume after a restart without replaying the complete file.

The pinned Alloy release runs its experimental Loki write-ahead log so accepted
records survive collector restarts while delivery is pending. Keep the image
version pinned and validate the configuration before upgrading Alloy because
Grafana may change experimental WAL behavior between releases.

Lumberjack creates `app.log` with owner-only permissions. Alloy therefore runs
as root inside its container solely to read that file. The collector has no
Docker socket or host filesystem access, its log and configuration mounts are
read-only, all Linux capabilities except the read-only `DAC_READ_SEARCH`
permission are dropped, privilege escalation is disabled, and no Alloy port is
exposed by the host.

## Future GitHub repository configuration

GitHub Free does not make deployment environments, environment secrets, or
required deployment reviewers available to private repositories. Store the
deployment settings under **Settings → Secrets and variables → Actions** at the
repository level instead. Do not create `staging` or `production` environments
for this workflow.

| Type | Name | Value |
| --- | --- | --- |
| Secret | `STAGING_DEPLOY_SSH_KEY` | Staging deployment user's private key |
| Secret | `PRODUCTION_DEPLOY_SSH_KEY` | Production deployment user's private key |
| Variable | `STAGING_DEPLOY_HOST` | Staging SSH hostname or IP address |
| Variable | `STAGING_DEPLOY_USER` | Staging deployment username |
| Variable | `STAGING_DEPLOY_PATH` | Staging path such as `/opt/nodus` |
| Variable | `STAGING_SSH_KNOWN_HOSTS` | Pre-verified staging host-key entry |
| Variable | `STAGING_HEALTH_URL` | Staging HTTPS URL ending in `/health` |
| Variable | `PRODUCTION_DEPLOY_HOST` | Production SSH hostname or IP address |
| Variable | `PRODUCTION_DEPLOY_USER` | Production deployment username |
| Variable | `PRODUCTION_DEPLOY_PATH` | Production path such as `/opt/nodus` |
| Variable | `PRODUCTION_SSH_KNOWN_HOSTS` | Pre-verified production host-key entry |
| Variable | `PRODUCTION_HEALTH_URL` | Production HTTPS URL ending in `/health` |

Obtain `SSH_KNOWN_HOSTS` through a trusted channel during provisioning. Do not
generate it dynamically in the workflow, because that would make host-key
checking ineffective. Use separate, host-restricted keys for staging and
production.

The repository Actions settings must permit the workflow `GITHUB_TOKEN` to
write packages. Keep the resulting GHCR package private and grant the
repository Actions access to it.

GitHub Free includes a monthly allowance for standard hosted-runner minutes in
private repositories. Pull-request runs that are superseded by a newer commit
are cancelled to avoid wasting that allowance. Pushes to `main` still repeat
the quality gate before publishing because private repositories on GitHub Free
do not have protected branches. Configure the account's Actions and Packages
budgets to zero if usage must stop instead of becoming billable after an
included allowance is exhausted.

Repository secrets are not equivalent to a protected production environment:
any collaborator who can modify and run trusted workflows may be able to use
them. Grant write access only to trusted maintainers. If production credentials
must remain owner-only, do not add `PRODUCTION_DEPLOY_SSH_KEY` to GitHub;
perform the final production deployment manually from a secured administrator
machine instead.

## Promoting staging to production

This promotion flow is not active in the current CI workflow. Restore the
staging and production deployment jobs before using it.

After the automatic staging deployment succeeds:

1. Open the completed **CI/CD** workflow run and copy the
   `ghcr.io/nodus-software/nodus-be@sha256:...` value from its summary.
2. Verify staging behavior and compatibility.
3. Open **Actions → CI/CD → Run workflow**, keep the branch set to `main`, paste
   the immutable value into `image_ref`, and run the workflow.

The manual run skips the quality and package jobs and deploys the exact image
already exercised in staging. The deployment script rejects mutable tags and
image names outside this repository.

## Deployment and recovery behavior

Unified account recovery uses single-use email tokens and separately hashed,
capability-scoped recovery sessions. Configure `RECOVERY_EMAIL_TOKEN_TTL`,
`RECOVERY_SESSION_TTL`, and `RECOVERY_MAX_ATTEMPTS`; the secure defaults are
10 minutes, 10 minutes, and five failed MFA confirmations respectively.

### Adaptive authentication rollout

Keep `AUTH_SECURITY_MODE=observation` until at least one representative traffic
cycle has been reviewed. Before enabling enforcement, compare password and MFA
failure distributions, shared-clinic IP throttling, Turnstile completion and
error rates, recovery completion, and email outbox terminal failures. Pay
particular attention to NATed clinic networks and accessibility-assisted users.

Enable enforcement by setting `AUTH_SECURITY_MODE=enforcement` and redeploying.
Production enforcement also requires `AUTH_RATE_LIMIT_HMAC_SECRET` and
`TURNSTILE_SECRET_KEY`. Roll back immediately—without a schema rollback—by
restoring `AUTH_SECURITY_MODE=observation` and redeploying. PostgreSQL continues
recording durable failure state in either mode.

Redis counters are supplemental. If Redis is unavailable, PostgreSQL account
and mechanism controls remain authoritative and login continues; investigate
the emitted operational errors and restore Redis before changing thresholds.
When Turnstile is required but unavailable, affected authentication returns a
generic temporary-unavailable response. Do not bypass Turnstile in production;
direct users to account recovery while the provider is restored.

Security email is written transactionally to `email_outbox`. The worker retries
transient failures with bounded exponential backoff, records permanent failure,
and removes message bodies after successful delivery. `dedupe_key` aggregates
repeated lock and lifecycle notifications. To retry a terminal failure after
correcting the provider, change the provider configuration and restart the
worker; it automatically requeues failures last attempted by a different
provider. Never copy token-bearing message bodies into tickets or logs.

Useful monitoring queries (run with the migration/operations role) include:

```sql
-- Failure and restriction distribution during the last day.
SELECT mechanism, failure_count, count(*)
FROM authentication_failure_states
WHERE updated_at >= now() - interval '24 hours'
GROUP BY mechanism, failure_count ORDER BY mechanism, failure_count;

-- Active restrictions, grouped by tenant and mechanism.
SELECT tenant_id, mechanism, count(*)
FROM authentication_failure_states
WHERE locked_until > now() GROUP BY tenant_id, mechanism;

-- Recovery starts and completions without exposing token hashes.
SELECT date_trunc('hour', created_at) AS hour, count(*) AS started,
       count(*) FILTER (WHERE consumed_at IS NOT NULL) AS consumed
FROM recovery_sessions
WHERE created_at >= now() - interval '24 hours' GROUP BY 1 ORDER BY 1;

-- Outbox health. Do not select recipient or message bodies during routine monitoring.
SELECT status, count(*), max(attempt_count) AS max_attempts
FROM email_outbox GROUP BY status;
```

For an authentication incident, switch to observation mode only when controls
are blocking legitimate traffic; preserve the audit and failure-state rows,
check Redis and Turnstile health, inspect outbox status without reading secrets,
revoke affected sessions, and use the explicit suspend endpoint for confirmed
account compromise. Recovery and administrative unlock never restore suspended
or deactivated accounts. Escalate platform-level recovery through the controlled
operational procedure until Phase 6 exists; do not delete MFA or lock rows by
hand as a normal recovery mechanism.

The remote script serializes deployments with `flock`, accepts only an
immutable `ghcr.io/nodus-software/nodus-be@sha256:...` reference, and performs this
sequence:

1. Pull the image and record the currently running image.
2. Pull Alloy, validate its configuration, and verify the collector starts.
3. Run all pending embedded migrations as a one-off container.
4. Recreate the API and wait for its Docker health check.
5. Verify the public HTTPS health URL.
6. Remove only unused images carrying this repository's OCI source label.

If migration fails, the running API is untouched. If the new API fails health
verification, the script recreates the previous application image. It never
automatically reverses a successful schema migration; migrations must remain
backward compatible with the immediately preceding application release.

Inspect a failed deployment on the host with:

```sh
cd /opt/nodus
export IMAGE_REF=ghcr.io/nodus-software/nodus-be@sha256:THE_FAILED_DIGEST
docker compose -f compose.production.yml ps
docker logs --tail 200 nodus-alloy
docker exec nodus-api tail -n 200 /app/logs/app.log
```

Normal application records are written to `app.log`, not the API container's
stdout, so `docker logs nodus-api` does not show them. In Grafana Cloud, open
**Explore**, select the Loki data source, and begin with:

```logql
{service_name="nodus-api", environment="production"}
```

Filter errors with:

```logql
{service_name="nodus-api", environment="production", level="ERROR"}
```

If records do not arrive, inspect `docker logs nodus-alloy` for authentication
or delivery errors and confirm the source file is visible with:

```sh
docker exec nodus-alloy ls -l /var/log/nodus/app.log
```

For an intentional application-only rollback, recover the previous digest
from the GitHub deployment history or GHCR and run:

```sh
cd /opt/nodus
IMAGE_REF=ghcr.io/nodus-software/nodus-be@sha256:THE_PREVIOUS_DIGEST \
  docker compose -f compose.production.yml up -d --no-deps --force-recreate api
```

Confirm that the previous application is compatible with the current schema
before a manual rollback. Prefer a forward fix whenever a migration has changed
or removed behavior used by the old release.

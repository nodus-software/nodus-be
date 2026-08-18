# Nodus Health Backend

Nodus Health is a multi-tenant healthcare API written in Go. It provides
organization onboarding, authentication and MFA, roles and permissions, staff
invitations, auditing, patient management, and clinical/outpatient workflows.

## Prerequisites

Install these before setting up the project:

- [Git](https://git-scm.com/)
- [Go 1.26 or newer](https://go.dev/doc/install)
- [Docker Engine](https://docs.docker.com/engine/install/) with the Docker
  Compose plugin
- GNU Make
- A C compiler such as GCC if you want to run race-detector tests

The Makefile uses Bash. Linux and macOS work directly; Windows contributors
should use WSL2 with Docker Desktop's WSL integration enabled.

Air and sqlc are pinned as Go tools in `go.mod`; they do not need separate
global installations.


## First-time setup

Clone the repository and enter it:

```sh
git clone git@github.com:Becaris/nodus-be.git
cd nodus-be
```

Create a local environment file. It is ignored by Git and must never be
committed:

```sh
cp .env.example .env
```

Download dependencies, start PostgreSQL, wait for it to become healthy, and
apply all pending migrations:

```sh
make setup
```

Start the development server with Air live reload:

```sh
make dev
```

The API listens on `http://localhost:8080` by default. Verify it in another
terminal:

```sh
curl http://localhost:8080/health
```

Expected response:

```json
{"success":true,"data":{"status":"ok"}}
```

Use `make run` instead of `make dev` when live reload is not needed. Stop the
server with `Ctrl+C`. Stop local containers without deleting database data with
`make db-down`.

## Local configuration

The application loads `.env` from the repository root or a parent directory.
Development has convenience defaults, including:

| Setting | Default | Purpose |
| --- | --- | --- |
| `APP_PORT` | `8080` | API port |
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5433` | Host port exposed by Compose |
| `DB_NAME` | `nodus_health` | Development database |
| `DB_USER` / `DB_PASSWORD` | `nodus` / `nodus` | Development credentials |
| `DB_SSL_MODE` | `disable` | Local PostgreSQL TLS mode |
| `ALLOWED_ORIGINS` | from `.env.example` | Space-separated frontend origins |
| `JWT_SECRET` | insecure development default | Access-token signing key |
| `MFA_ENCRYPTION_KEY` | insecure development default | MFA-secret encryption key |

The Compose database is available at:

```text
postgres://nodus:nodus@localhost:5433/nodus_health?sslmode=disable
```

`DB_URL` overrides the individual runtime database settings.
`MIGRATION_DB_URL` is deliberately separate because production migrations
should use a schema-owner role. The Makefile supplies the local URL by default;
override it when needed:

```sh
MIGRATION_DB_URL='postgres://user:password@host:5432/database?sslmode=require' make migrate-up
```

### Why the API connects as a different role

Tenant scoping is enforced by PostgreSQL row-level security: the clinical
queries carry no tenant predicate of their own and rely entirely on each table's
`tenant_isolation` policy, keyed off the `app.tenant_id` that
`middleware.TenantTransaction` sets per request.

PostgreSQL skips those policies for superusers and for roles holding
`BYPASSRLS`. Connecting the API as the schema owner therefore returns *every*
tenant's rows with no error and no warning. `make setup` creates `nodus_app`
(see `deploy/app_role.sql`) with neither privilege, and `DB_URL` points at it;
only `MIGRATION_DB_URL` uses the owner. Re-run `make db-role` if the role is
ever dropped — it is idempotent, and later migrations are covered by
`ALTER DEFAULT PRIVILEGES`.

The defaults are for local development only. Production must provide strong
values for secrets, enable secure refresh cookies, and use TLS-enabled database
connections. See [docs/deployment.md](docs/deployment.md) for production setup.

### Email in development

SMTP is optional for starting the API. When `SMTP_HOST` is empty, email sending
is a no-op, so registration and password-reset requests can succeed without
delivering their links. Configure `SMTP_HOST`, `SMTP_PORT`, `SMTP_SENDER`, and
`SMTP_PASSWORD` with a development SMTP account when testing email-driven
onboarding flows.

Email templates can be rendered locally without sending mail:

```sh
go run ./cmd/email-preview
```

The generated preview index is written to `tmp/email-previews/index.html`.

## Common development commands

Run `make` or `make help` to see the complete target list.

| Command | Description |
| --- | --- |
| `make setup` | Download dependencies, start PostgreSQL, create the runtime role, and migrate it |
| `make dev` | Run the API with Air live reload |
| `make run` | Run the API directly |
| `make db-up` | Start PostgreSQL and wait for its health check |
| `make db-role` | Create/refresh the least-privileged runtime role |
| `make db-status` | Show Compose service status |
| `make db-logs` | Follow PostgreSQL logs |
| `make db-down` | Stop services while preserving database data |
| `make migrate-up` | Apply all pending embedded migrations |
| `make import-icd11 FILE=/path/file.xlsx [COMMIT=1]` | Validate or atomically import a WHO ICD-11 MMS workbook |
| `make generate` | Regenerate sqlc code after SQL changes |
| `make test` | Run the test suite |
| `make test-race` | Run tests with the Go race detector |
| `make coverage` | Generate `coverage.out` and `coverage.html` |
| `make fmt` | Format Go source files |
| `make check` | Check formatting, run vet, and run tests |
| `make ci` | Run the local equivalent of the CI quality gate |
| `make build` | Build API and migration binaries into `tmp/bin` |
| `make clean` | Remove local build and coverage artifacts |

Run `make generate` whenever a file under a PostgreSQL `queries` directory or
`sqlc.yaml` changes. Commit the resulting generated Go code together with the
SQL change. Create migrations as paired, sequentially numbered `.up.sql` and
`.down.sql` files under `migrations/`; the application migration command applies
up migrations only.

## API usage and documentation

This is a multi-tenant API. Production requests resolve the tenant from the
hostname:

```text
https://example-clinic.example.com
```

Local clients may send `X-Tenant-Slug`; production ignores it. Public organization
registration, slug checks, and verified organization discovery are available only
on `TENANT_BASE_DOMAIN`. Authenticated requests also use:

```text
Authorization: Bearer <access-token>
```

The API contracts are stored in the repository rather than served by the API:

- [`auth-api-contract.yaml`](auth-api-contract.yaml) — authentication and
  session contract
- [`clinical-api-contract.yaml`](clinical-api-contract.yaml) — clinical API
  contract
- [`static/docs/swagger.yaml`](static/docs/swagger.yaml) — broader Swagger
  specification
- [`static/docs/README.md`](static/docs/README.md) — response envelopes,
  refresh-cookie behavior, and health-check details

JSON responses use a consistent envelope:

```json
{"success":true,"data":{}}
```

Errors use:

```json
{"success":false,"error":{"code":"...","message":"..."}}
```

## Project layout

```text
cmd/api/                   API entry point
cmd/migrate/               forward-only migration command
config/                    environment configuration
internal/                  domain services, handlers, and repositories
internal/platform/         shared database and migration infrastructure
migrations/                embedded PostgreSQL migrations
pkg/                       shared response, logging, and security packages
tests/                     higher-level domain tests
static/docs/               API documentation
deploy/                    production Compose and deployment scripts
```

Application logs are written to `logs/app.log` and rotated locally. Production
mounts the same log directory in Grafana Alloy and forwards the structured JSON
records to Grafana Cloud Loki; see [the deployment guide](docs/deployment.md#grafana-cloud-logs).
Air writes its temporary build output under `tmp/`. Both directories are
ignored by Git.

## Troubleshooting

### The API cannot connect to PostgreSQL

Check that the container is healthy and inspect its logs:

```sh
make db-status
make db-logs
```

Confirm that port `5433` is free and that `.env` database settings match
`docker-compose.yml`.

### Migrations fail

Start the database first, then retry:

```sh
make db-up
make migrate-up
```

The migration role needs schema modification privileges and permission to
install the PostgreSQL `pg_trgm` extension. Migrations are forward-only in the
normal development and deployment workflow; do not edit a migration that has
already been shared or deployed.

### Race tests fail before compiling

`make test-race` requires CGO and a C compiler. Install GCC or the equivalent
build toolchain for your operating system, then retry.

### Go tool or generated-code errors

Ensure your Go version matches `go.mod`, download modules, and regenerate sqlc
outputs:

```sh
make deps
make generate
```

Before opening a pull request, run:

```sh
make ci
```

The GitHub Actions quality gate checks formatting, runs `go vet`, executes the
race-enabled test suite, builds both commands, and verifies migrations against
a clean PostgreSQL database. Work on a feature branch and keep unrelated local
changes out of the pull request. Never commit `.env`, logs, coverage reports,
or files under `tmp/`.

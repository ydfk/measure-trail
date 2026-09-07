# MeasureTrail Backend

Go Fiber API with SQLite migrations, password and Apple authentication, profiles, versioned measurements, incremental sync, export, and the legacy SQLite inspection/import tool.

## Local development

1. Copy `../.env.example` to `../.env`, set two unique development JWT secrets, and keep `MEASURETRAIL_ENV=development` for local work.
2. From this directory, export the file before starting the server: `set -a; source ../.env; set +a; go run ./cmd`. The Go process does not load `.env` automatically. The default database is `data/measuretrail.sqlite` and the API listens on port `21000`.
3. Alternatively, run `docker compose up --build api` from the repository root. Compose honors `MEASURETRAIL_ENV` from the root `.env`; without that file it remains production-safe by default.
4. Check `http://localhost:21000/api/health`.

Run `go test ./...`, `go vet ./...`, and `./scripts/update-openapi.sh` before changing API behavior. The committed OpenAPI 3.0/3.1 snapshots are the shared contract for future iOS, Web, and Android clients.

## Deployment

Production uses the root `docker-compose.yml` with one Go instance and named volumes for SQLite, logs, and backups. The image may build a Vue/React application from `web/`, then copies only its `dist/` output into the runtime image; Go serves those files with SPA fallback. No frontend web server runs in the container. `MEASURETRAIL_ENV=production` requires an HTTPS public base URL, non-placeholder JWT secrets, and complete Apple configuration whenever Apple login is enabled. See [`../docs/deployment.md`](../docs/deployment.md) for deployment, backup, restore, and upgrade steps.

## Legacy data

The user-provided `../slimtrack.db` is read-only input and is ignored by Git. Run `go run ./cmd/legacy-import --source ../slimtrack.db --dry-run` first. After confirmation, a real import targets the bootstrapped default account unless `--owner-username` selects another existing account; see [`../docs/development.md`](../docs/development.md).

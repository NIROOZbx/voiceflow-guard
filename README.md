# Voiceflow Guard API

A Go HTTP API built with Gin, PostgreSQL connectivity through pgx, and sqlc for generated query code.

## Run

```sh
task dev
# or
go run ./cmd/server
```

The server listens on `0.0.0.0:8080` by default. Configure it with a `.env` file (see `.env.example`):

- `HOST` (default `0.0.0.0`)
- `PORT` (default `8080`)
- `DATABASE_URL` (PostgreSQL connection URL; when set, the server verifies the connection at startup)

## Database Migrations (Goose)

Migrations are located in `internal/database/schema` and managed via [Goose](https://github.com/pressly/goose).

```sh
# Create a new SQL migration file
task migrate:create -- add_users_table

# Check migration status
task migrate:status

# Run all pending migrations
task migrate:up

# Roll back the most recent migration
task migrate:down
```

## Database queries (sqlc)

Place schema migrations in `internal/database/schema` and SQL query files in `internal/database/queries`. Generate the Go query layer with:

```sh
task sqlc
# or
sqlc generate
```

The sqlc configuration is in `sqlc.yaml` and generates pgx/v5 code under `internal/database/sqlc`.

## Health check

```sh
curl http://localhost:8080/health
```

Expected response:

```json
{"status":"ok"}
```


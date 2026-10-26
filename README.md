# Voiceflow Guard API

A Go HTTP API built with Gin, PostgreSQL connectivity through pgx, and sqlc for generated query code.

## Run

```sh
go mod tidy
go run ./cmd/server
```

The server listens on `0.0.0.0:8080` by default. Configure it with:

- `HOST` (default `0.0.0.0`)
- `PORT` (default `8080`)
- `DATABASE_URL` (optional PostgreSQL connection URL; when set, the server verifies the connection at startup)

## Database queries

Place schema migrations or schema definitions in `internal/database/schema` and SQL query files in `internal/database/queries`. Generate the Go query layer with:

```sh
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

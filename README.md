# Voiceflow Guard API

A robust Go HTTP service designed to protect transactional Voiceflow flows and webhooks against duplicate submissions, retries, and network race conditions using an idempotency guard pattern.

Built with **Gin**, **PostgreSQL** (via `jackc/pgx/v5`), and **sqlc**.

---

## Features

- **Idempotency Guard**: Prevents double-booking and duplicate mutations across network retries or Voiceflow agent turn replays.
- **Payload Integrity Verification**: Uses SHA-256 payload hashing (`pkg/hasher`) to detect and reject requests reusing an existing idempotency key with conflicting parameters (`409 Conflict`).
- **Distributed Advisory Locking**: Uses PostgreSQL transaction-level advisory locks (`pg_advisory_xact_lock`) to serialize concurrent booking requests for the exact same slot and prevent race conditions.
- **Semantic Duplicate Booking Detection**: Detects potential duplicate bookings for the same phone number, date, and time slot. Prompts with a `409 Conflict` containing existing booking details unless overridden via `confirm_potential_duplicate: true`.
- **Operation Lifecycle State Machine**: Tracks operations through `PENDING`, `COMPLETED`, and `FAILED` states. Replaying a completed request safely returns the cached result without re-executing.
- **Clean Layered Architecture**: Strictly separated HTTP Handlers, Business Services, and Repositories with dependency injection.
- **Strict Error Sanitization**: Domain sentinel errors (`pkg/apperror`) map deterministically to HTTP status codes. Infrastructure and database errors are sanitized so internal errors are never leaked to clients.

---

## Architecture & Project Structure

```
voiceflow-guard/
├── cmd/
│   └── server/          # Application entrypoint & wiring (main.go)
├── internal/
│   ├── config/          # Environment configuration loading
│   ├── database/        # DB connection, goose migrations, and sqlc queries
│   │   ├── queries/     # SQL query definitions (.sql)
│   │   ├── schema/      # Goose SQL schema migrations
│   │   └── sqlc/        # Generated type-safe Go database code
│   ├── repository/      # Database abstraction layer (Operations & Bookings)
│   ├── server/          # HTTP handlers and Gin router
│   └── service/         # Core business logic, idempotency, and concurrency guards
└── pkg/
    ├── apperror/        # Domain sentinel errors and HTTP status mapping
    └── hasher/          # Deterministic SHA-256 hashing utilities
```

---

## API Reference

### 1. Health Check

**GET** `/health`

```sh
curl http://localhost:8080/health
```

**Response (`200 OK`)**:
```json
{
  "status": "ok"
}
```

---

### 2. Create Booking (Idempotent)

**POST** `/bookings`

Creates a new booking guarded by an idempotency key passed via the `Idempotency-Key` HTTP header.

#### Headers
| Header | Description | Required |
| :--- | :--- | :--- |
| `Idempotency-Key` | Unique key identifying this operation (max 255 chars) | **Yes** |
| `Content-Type` | `application/json` | **Yes** |

#### Request Body
```json
{
  "customer_name": "Jane Doe",
  "phone_no": "+1234567890",
  "party_size": 4,
  "booking_date": "2026-11-20",
  "booking_time": "19:30:00",
  "confirm_potential_duplicate": false
}
```

| Field | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `customer_name` | string | **Yes** | Name of the customer. |
| `phone_no` | string | **Yes** | Contact phone number. |
| `party_size` | integer | **Yes** | Number of guests (must be > 0). |
| `booking_date` | string | **Yes** | Date in `YYYY-MM-DD` format. |
| `booking_time` | string | **Yes** | Time in `HH:MM:SS` or `HH:MM` format. |
| `confirm_potential_duplicate` | boolean | No | Set to `true` to force booking if a duplicate slot is detected. |

#### Responses

**`201 Created` / `200 OK` (Success or Cached Idempotent Replay)**:
```json
{
  "operation_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
  "booking_id": "c3a5b6d7-8e9f-4012-a345-6789abcdef01",
  "status": "COMPLETED",
  "customer_name": "Jane Doe",
  "phone_no": "+1234567890",
  "booking_date": "2026-11-20",
  "booking_time": "19:30:00",
  "party_size": 4
}
```

**`409 Conflict` (Potential Duplicate Booking Detected)**:
Occurs when an existing booking already exists for the same phone number, date, and time:
```json
{
  "error": "potential duplicate booking detected",
  "potential_duplicate": {
    "booking_date": "2026-11-20",
    "booking_time": "19:30:00",
    "party_size": 4
  }
}
```
*To proceed with this duplicate booking, resend the request with `"confirm_potential_duplicate": true`.*

**`409 Conflict` (Payload Mismatch)**:
Occurs when the same `Idempotency-Key` is reused with different request data:
```json
{
  "error": "idempotency key reused with different request"
}
```

**`409 Conflict` (Concurrent Operation In-Progress)**:
```json
{
  "error": "operation is still in progress"
}
```

**`400 Bad Request` (Validation Error)**:
```json
{
  "error": "Idempotency-Key header is required and must not exceed 255 characters"
}
```

**`500 Internal Server Error` (Sanitized)**:
```json
{
  "error": "operation failed"
}
```

---

### 3. Query Bookings

**GET** `/bookings?phone_no={phone_no}[&date={YYYY-MM-DD}]`

List bookings filtered by phone number and an optional booking date.

#### Query Parameters
| Parameter | Type | Required | Description |
| :--- | :--- | :--- | :--- |
| `phone_no` | string | **Yes** | Phone number to query bookings for. |
| `date` | string | No | Optional booking date in `YYYY-MM-DD` format. |

#### Example
```sh
curl "http://localhost:8080/bookings?phone_no=+1234567890&date=2026-11-20"
```

**Response (`200 OK`)**:
```json
[
  {
    "operation_id": "9b1deb4d-3b7d-4bad-9bdd-2b0d7b3dcb6d",
    "booking_id": "c3a5b6d7-8e9f-4012-a345-6789abcdef01",
    "status": "COMPLETED",
    "customer_name": "Jane Doe",
    "phone_no": "+1234567890",
    "booking_date": "2026-11-20",
    "booking_time": "19:30:00",
    "party_size": 4
  }
]
```

---

## Getting Started

### Prerequisites

- [Go](https://go.dev/) (1.23+)
- [PostgreSQL](https://www.postgresql.org/)
- [Task](https://taskfile.dev/) (optional, but recommended)
- [Goose](https://github.com/pressly/goose) (for migrations)
- [sqlc](https://sqlc.dev/) (for query code generation)

### Configuration

Copy the example environment file:

```sh
cp .env.example .env
```

Configure your environment variables:
- `HOST`: Server host (default `0.0.0.0`)
- `PORT`: Server port (default `8080`)
- `DATABASE_URL`: PostgreSQL connection string (e.g. `postgres://postgres:postgres@localhost:5432/voiceflow_guard?sslmode=disable`)

### Database Migrations

Run database migrations via Task or Goose:

```sh
# Apply pending migrations
task migrate:up

# Check migration status
task migrate:status

# Roll back the last migration
task migrate:down
```

### Running the Server

```sh
task dev
# or
go run ./cmd/server
```

The server will start listening at `http://localhost:8080`.

### Running Tests

```sh
# Run unit tests
go test -v ./...

# Run all tests including isolated containerized integration suite (requires Docker)
task test
```

### Code Generation (`sqlc`)

After modifying queries in `internal/database/queries/` or schemas in `internal/database/schema/`:

```sh
task sqlc
# or
sqlc generate
```

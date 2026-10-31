//go:build integration

package integration

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/NIROOZbx/voiceflow-guard/internal/database"
	"github.com/NIROOZbx/voiceflow-guard/internal/database/sqlc"
	"github.com/NIROOZbx/voiceflow-guard/internal/repository"
	"github.com/NIROOZbx/voiceflow-guard/internal/server"
	"github.com/NIROOZbx/voiceflow-guard/internal/service"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	globalDBPool *pgxpool.Pool
	dbInitOnce   sync.Once
)

const defaultTestDBURL = "postgres://postgres:postgres@localhost:5434/voiceflow_guard_test?sslmode=disable"

// TestMain manages the global database pool and migrations for integration tests.
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	dbURL := getTestDBURL()
	if err := validateTestDBURL(dbURL); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Refusing to run tests against unsafe database: %v\n", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := database.Connect(ctx, dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to connect to test database (%s): %v\n", dbURL, err)
		os.Exit(1)
	}
	globalDBPool = pool

	if err := applyMigrations(ctx, pool); err != nil {
		fmt.Fprintf(os.Stderr, "FATAL: Failed to apply migrations to test database: %v\n", err)
		pool.Close()
		os.Exit(1)
	}

	code := m.Run()

	globalDBPool.Close()
	os.Exit(code)
}

func getTestDBURL() string {
	if url := os.Getenv("TEST_DATABASE_URL"); url != "" {
		return url
	}
	return defaultTestDBURL
}

func validateTestDBURL(dsn string) error {
	// Guard against accidentally running destructive operations on non-test databases.
	lower := strings.ToLower(dsn)
	if !strings.Contains(lower, "test") {
		return fmt.Errorf("database URL %q must contain 'test' to prevent data loss on production or dev environments", dsn)
	}
	return nil
}

// applyMigrations discovers and executes all .sql migration files in order.
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	schemaDir, err := findSchemaDir()
	if err != nil {
		return fmt.Errorf("locate schema directory: %w", err)
	}

	entries, err := os.ReadDir(schemaDir)
	if err != nil {
		return fmt.Errorf("read schema directory %s: %w", schemaDir, err)
	}

	var sqlFiles []string
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".sql") {
			sqlFiles = append(sqlFiles, entry.Name())
		}
	}
	sort.Strings(sqlFiles)

	for _, fileName := range sqlFiles {
		filePath := filepath.Join(schemaDir, fileName)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", fileName, err)
		}

		upSQL := extractGooseUpSQL(string(content))
		if strings.TrimSpace(upSQL) == "" {
			continue
		}

		if _, err := pool.Exec(ctx, upSQL); err != nil {
			return fmt.Errorf("execute migration %s: %w", fileName, err)
		}
	}

	return nil
}

func findSchemaDir() (string, error) {
	candidates := []string{
		"../../internal/database/schema",
		"internal/database/schema",
		"../internal/database/schema",
	}

	for _, cand := range candidates {
		abs, err := filepath.Abs(cand)
		if err == nil {
			if stat, err := os.Stat(abs); err == nil && stat.IsDir() {
				return abs, nil
			}
		}
	}
	return "", fmt.Errorf("could not find internal/database/schema from current directory")
}

// extractGooseUpSQL extracts the portion of the file between '-- +goose Up' and '-- +goose Down' (or EOF).
func extractGooseUpSQL(fileContent string) string {
	const upMarker = "-- +goose Up"
	const downMarker = "-- +goose Down"

	upIdx := strings.Index(fileContent, upMarker)
	if upIdx == -1 {
		return fileContent
	}
	start := upIdx + len(upMarker)

	downIdx := strings.Index(fileContent[start:], downMarker)
	if downIdx == -1 {
		return fileContent[start:]
	}
	return fileContent[start : start+downIdx]
}

// TestApp encapsulates dependencies for an integration test execution.
type TestApp struct {
	DB         *pgxpool.Pool
	Queries    *sqlc.Queries
	TxManager  repository.TransactionManager
	OpsService *service.BookingService
	Handler    *server.BookingHandler
	Router     *gin.Engine
	Server     *httptest.Server
	Client     *http.Client
}

// newTestApp initializes an isolated test application instance wired to the real test database.
func newTestApp(t *testing.T) *TestApp {
	t.Helper()
	txManager := repository.NewTransactionManager(globalDBPool)
	return newTestAppWithTxManager(t, txManager)
}

// newTestAppWithTxManager initializes an instance allowing custom TransactionManager injection.
func newTestAppWithTxManager(t *testing.T, txManager repository.TransactionManager) *TestApp {
	t.Helper()

	queries := sqlc.New(globalDBPool)
	opsRepo := repository.NewOpsRepo(queries)
	bookingsRepo := repository.NewBookingsRepo(queries)
	opsService := service.NewOpsService(opsRepo, txManager, bookingsRepo)
	bookingHandler := server.NewBookingHandler(opsService)
	router := server.NewRouter(bookingHandler)

	ts := httptest.NewServer(router)
	t.Cleanup(func() {
		ts.Close()
	})

	app := &TestApp{
		DB:         globalDBPool,
		Queries:    queries,
		TxManager:  txManager,
		OpsService: opsService,
		Handler:    bookingHandler,
		Router:     router,
		Server:     ts,
		Client:     ts.Client(),
	}

	app.TruncateTables(t)
	return app
}

// TruncateTables removes all test data between tests for deterministic isolation.
func (app *TestApp) TruncateTables(t *testing.T) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := app.DB.Exec(ctx, "TRUNCATE TABLE bookings, operations CASCADE;")
	if err != nil {
		t.Fatalf("failed to truncate tables: %v", err)
	}
}

// PostBooking sends an HTTP POST request to /bookings.
func (app *TestApp) PostBooking(t *testing.T, idempotencyKey string, payload any) (*http.Response, []byte) {
	t.Helper()

	var bodyReader io.Reader
	if payload != nil {
		bodyBytes, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequest(http.MethodPost, app.Server.URL+"/bookings", bodyReader)
	if err != nil {
		t.Fatalf("create HTTP request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}

	resp, err := app.Client.Do(req)
	if err != nil {
		t.Fatalf("execute HTTP request: %v", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}

	return resp, respBody
}

// CountOperations returns the number of operations matching the idempotency key.
func (app *TestApp) CountOperations(ctx context.Context, key string) (int, error) {
	var count int
	err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM operations WHERE idempotency_key = $1", key).Scan(&count)
	return count, err
}

// TotalOperations returns total operation count across the table.
func (app *TestApp) TotalOperations(ctx context.Context) (int, error) {
	var count int
	err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM operations").Scan(&count)
	return count, err
}

// TotalBookings returns total booking count across the table.
func (app *TestApp) TotalBookings(ctx context.Context) (int, error) {
	var count int
	err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM bookings").Scan(&count)
	return count, err
}

// CountBookingsByOperationID returns booking count associated with an operation ID.
func (app *TestApp) CountBookingsByOperationID(ctx context.Context, opID pgtype.UUID) (int, error) {
	var count int
	err := app.DB.QueryRow(ctx, "SELECT COUNT(*) FROM bookings WHERE operation_id = $1", opID).Scan(&count)
	return count, err
}

// validBookingPayload returns a standard valid booking payload for test cases.
func validBookingPayload() map[string]any {
	return map[string]any{
		"customer_name": "Alice Wonderland",
		"party_size":    4,
		"booking_date":  "2026-11-20",
		"booking_time":  "19:00:00",
	}
}

// newIdempotencyKey generates a UUID idempotency key for testing.
func newIdempotencyKey() string {
	return uuid.NewString()
}

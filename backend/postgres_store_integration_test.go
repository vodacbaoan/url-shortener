package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	pq "github.com/lib/pq"
)

func newIntegrationPostgresStore(t *testing.T) *postgresURLStore {
	t.Helper()

	databaseURL := strings.TrimSpace(os.Getenv("INTEGRATION_DATABASE_URL"))
	if databaseURL == "" {
		t.Skip("set INTEGRATION_DATABASE_URL to run Postgres integration tests")
	}

	baseDB, err := sql.Open("postgres", databaseURL)
	if err != nil {
		t.Fatalf("expected integration database connection: %v", err)
	}
	if err := baseDB.Ping(); err != nil {
		_ = baseDB.Close()
		t.Fatalf("expected integration database to be reachable: %v", err)
	}

	schemaName := integrationSchemaName()
	quotedSchemaName := pq.QuoteIdentifier(schemaName)
	if _, err := baseDB.Exec("CREATE SCHEMA " + quotedSchemaName); err != nil {
		_ = baseDB.Close()
		t.Fatalf("expected test schema to be created: %v", err)
	}
	t.Cleanup(func() {
		_, _ = baseDB.Exec("DROP SCHEMA IF EXISTS " + quotedSchemaName + " CASCADE")
		_ = baseDB.Close()
	})

	store, err := newPostgresURLStore(databaseURLWithSearchPath(databaseURL, schemaName))
	if err != nil {
		t.Fatalf("expected postgres store to start: %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	return store
}

func integrationSchemaName() string {
	return fmt.Sprintf("test_%d", time.Now().UnixNano())
}

func databaseURLWithSearchPath(databaseURL, schemaName string) string {
	separator := "?"
	if strings.Contains(databaseURL, "?") {
		separator = "&"
	}

	return databaseURL + separator + "search_path=" + schemaName
}

func TestPostgresURLStoreIntegrationUsersLinksAndStats(t *testing.T) {
	store := newIntegrationPostgresStore(t)

	userA, err := store.CreateUser("a@example.com", "hash-a")
	if err != nil {
		t.Fatalf("expected user A to be created: %v", err)
	}
	userB, err := store.CreateUser("b@example.com", "hash-b")
	if err != nil {
		t.Fatalf("expected user B to be created: %v", err)
	}
	if _, err := store.CreateUser("a@example.com", "other-hash"); !errors.Is(err, errUserExists) {
		t.Fatalf("expected duplicate user error, got %v", err)
	}

	if err := store.Save("abc123", "https://example.com/a", userA.ID); err != nil {
		t.Fatalf("expected user A link to be saved: %v", err)
	}
	if err := store.Save("abc123", "https://example.com/duplicate", userA.ID); !errors.Is(err, errShortCodeExists) {
		t.Fatalf("expected duplicate short code error, got %v", err)
	}
	if err := store.Save("xyz789", "https://example.com/b", userB.ID); err != nil {
		t.Fatalf("expected user B link to be saved: %v", err)
	}

	targetURL, err := store.Lookup("abc123")
	if err != nil {
		t.Fatalf("expected short code lookup to succeed: %v", err)
	}
	if targetURL != "https://example.com/a" {
		t.Fatalf("expected target URL %q, got %q", "https://example.com/a", targetURL)
	}
	if _, err := store.Lookup("missing"); !errors.Is(err, errShortCodeNotFound) {
		t.Fatalf("expected missing short code error, got %v", err)
	}

	if err := store.IncrementClickCount("abc123"); err != nil {
		t.Fatalf("expected first click increment to succeed: %v", err)
	}
	if err := store.IncrementClickCount("abc123"); err != nil {
		t.Fatalf("expected second click increment to succeed: %v", err)
	}
	if err := store.IncrementClickCount("missing"); !errors.Is(err, errShortCodeNotFound) {
		t.Fatalf("expected missing increment error, got %v", err)
	}

	stats, err := store.GetStats("abc123", userA.ID)
	if err != nil {
		t.Fatalf("expected owner stats lookup to succeed: %v", err)
	}
	if stats.ShortCode != "abc123" || stats.TargetURL != "https://example.com/a" || stats.ClickCount != 2 {
		t.Fatalf("unexpected stats response: %#v", stats)
	}
	if _, err := store.GetStats("abc123", userB.ID); !errors.Is(err, errForbidden) {
		t.Fatalf("expected forbidden stats lookup for non-owner, got %v", err)
	}
	if _, err := store.GetStats("missing", userA.ID); !errors.Is(err, errShortCodeNotFound) {
		t.Fatalf("expected missing stats error, got %v", err)
	}

	userALinks, err := store.ListOwnedLinks(userA.ID)
	if err != nil {
		t.Fatalf("expected user A links to load: %v", err)
	}
	if len(userALinks) != 1 || userALinks[0].ShortCode != "abc123" {
		t.Fatalf("expected only user A link, got %#v", userALinks)
	}

	userBLinks, err := store.ListOwnedLinks(userB.ID)
	if err != nil {
		t.Fatalf("expected user B links to load: %v", err)
	}
	if len(userBLinks) != 1 || userBLinks[0].ShortCode != "xyz789" {
		t.Fatalf("expected only user B link, got %#v", userBLinks)
	}
}

func TestPostgresURLStoreIntegrationRefreshTokenLifecycle(t *testing.T) {
	store := newIntegrationPostgresStore(t)

	user, err := store.CreateUser("refresh@example.com", "hash")
	if err != nil {
		t.Fatalf("expected user to be created: %v", err)
	}

	expiresAt := time.Now().UTC().Add(time.Hour)
	if err := store.StoreRefreshToken(user.ID, "old-token", expiresAt); err != nil {
		t.Fatalf("expected refresh token to be stored: %v", err)
	}

	rotatedUserID, err := store.RotateRefreshToken("old-token", "new-token", expiresAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("expected refresh token to rotate: %v", err)
	}
	if rotatedUserID != user.ID {
		t.Fatalf("expected rotated user ID %d, got %d", user.ID, rotatedUserID)
	}
	if _, err := store.RotateRefreshToken("old-token", "unused-token", expiresAt.Add(time.Hour)); !errors.Is(err, errInvalidRefreshToken) {
		t.Fatalf("expected old token to be invalid after rotation, got %v", err)
	}

	rotatedUserID, err = store.RotateRefreshToken("new-token", "next-token", expiresAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("expected new token to rotate: %v", err)
	}
	if rotatedUserID != user.ID {
		t.Fatalf("expected rotated user ID %d, got %d", user.ID, rotatedUserID)
	}

	if err := store.RevokeRefreshToken("next-token"); err != nil {
		t.Fatalf("expected refresh token revoke to succeed: %v", err)
	}
	if _, err := store.RotateRefreshToken("next-token", "revoked-token", expiresAt.Add(time.Hour)); !errors.Is(err, errInvalidRefreshToken) {
		t.Fatalf("expected revoked token to be invalid, got %v", err)
	}

	if err := store.StoreRefreshToken(user.ID, "expired-token", time.Now().UTC().Add(-time.Minute)); err != nil {
		t.Fatalf("expected expired token to be stored: %v", err)
	}
	if _, err := store.RotateRefreshToken("expired-token", "unused-expired-token", expiresAt); !errors.Is(err, errInvalidRefreshToken) {
		t.Fatalf("expected expired token to be invalid, got %v", err)
	}
}

func TestRunMigrationsIntegrationIsIdempotent(t *testing.T) {
	store := newIntegrationPostgresStore(t)

	if err := runMigrations(store.db); err != nil {
		t.Fatalf("expected migrations to run twice: %v", err)
	}

	migrations, err := loadMigrations(migrationFiles)
	if err != nil {
		t.Fatalf("expected migrations to load: %v", err)
	}

	var appliedCount int
	if err := store.db.QueryRow("SELECT COUNT(*) FROM schema_migrations;").Scan(&appliedCount); err != nil {
		t.Fatalf("expected applied migrations count: %v", err)
	}
	if appliedCount != len(migrations) {
		t.Fatalf("expected %d applied migrations, got %d", len(migrations), appliedCount)
	}
}

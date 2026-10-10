package database_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"judge/api/internal/database"
)

func TestRegistrationQuota(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	schema := fmt.Sprintf("test_registration_quotas_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) }()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	pool, err := database.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	quota := database.RegistrationQuota{Pool: pool}
	limitedFor := func(err error, minimum, maximum int) bool {
		var limited *database.RegistrationQuotaError
		return errors.As(err, &limited) && limited.RetryAfter >= minimum && limited.RetryAfter <= maximum
	}

	// An address gets three emails, then waits for one 20-minute slot.
	for i := range 3 {
		if err := quota.Consume(ctx, "203.0.113.1", "a@example.com"); err != nil {
			t.Fatal(i, err)
		}
	}
	if err := quota.Consume(ctx, "203.0.113.1", "a@example.com"); !limitedFor(err, 1100, 1200) {
		t.Fatalf("address limit: %v", err)
	}
	// The refused request charged neither subject, so the network still has 17 of 20.
	for i := range 17 {
		if err := quota.Consume(ctx, "203.0.113.1", fmt.Sprintf("n%d@example.com", i)); err != nil {
			t.Fatal(i, err)
		}
	}
	if err := quota.Consume(ctx, "203.0.113.1", "b@example.com"); !limitedFor(err, 100, 180) {
		t.Fatalf("network limit: %v", err)
	}
	// The network refusal must not have charged b@example.com either.
	for i := range 3 {
		if err := quota.Consume(ctx, "203.0.113.2", "b@example.com"); err != nil {
			t.Fatal(i, err)
		}
	}

	// Concurrent requests for one address serialize on its row.
	var wg sync.WaitGroup
	results := make(chan error, 10)
	for i := range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results <- quota.Consume(ctx, fmt.Sprintf("198.51.100.%d", i), "race@example.com")
		}()
	}
	wg.Wait()
	close(results)
	accepted := 0
	for err := range results {
		if err == nil {
			accepted++
		} else if !limitedFor(err, 1, 1200) {
			t.Fatal(err)
		}
	}
	if accepted != 3 {
		t.Fatalf("accepted %d concurrent requests, want 3", accepted)
	}

	var raw int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM registration_quotas WHERE subject LIKE '%example%' OR subject LIKE '%.%'`).Scan(&raw); err != nil || raw != 0 {
		t.Fatalf("raw subjects stored: %d %v", raw, err)
	}
	// Expired rows are deleted by the next request.
	if _, err := pool.Exec(ctx, `UPDATE registration_quotas SET full_at=statement_timestamp()-interval '1 second'`); err != nil {
		t.Fatal(err)
	}
	if err := quota.Consume(ctx, "203.0.113.9", "fresh@example.com"); err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM registration_quotas`).Scan(&rows); err != nil || rows != 2 {
		t.Fatalf("rows after cleanup: %d %v", rows, err)
	}
}

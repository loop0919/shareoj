package httpapi

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/tags"
)

// deadlineTracer records the queries that run without a deadline of at most ten seconds.
type deadlineTracer struct {
	mu      sync.Mutex
	queries int
	missing []string
}

func (d *deadlineTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queries++
	if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 10*time.Second {
		d.missing = append(d.missing, data.SQL)
	}
	return ctx
}

func (d *deadlineTracer) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (d *deadlineTracer) reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.queries, d.missing = 0, nil
}

// Every public route queries the database within the same ten seconds as the routes that require an account.
func TestPublicRoutesHaveDeadlinePostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close(ctx) })
	schema := fmt.Sprintf("test_public_deadline_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	})
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	cfg, err := pgxpool.ParseConfig(u.String())
	if err != nil {
		t.Fatal(err)
	}
	tracer := &deadlineTracer{}
	cfg.ConnConfig.Tracer = tracer
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	store := problems.New(pool)
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES('alice','alice')`); err != nil {
		t.Fatal(err)
	}
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: pool}, Submissions: &submissions.Store{Pool: pool}, Posts: posts.New(pool), Tags: &tags.Store{Pool: pool}, Profiles: profiles.New(pool), Images: &images.Store{Pool: pool}})
	const id = "11111111-1111-4111-8111-111111111111"
	for _, path := range []string{
		// These handlers had no deadline of their own.
		"/contests",
		"/contests/" + id,
		"/contests/" + id + "/problems/" + id,
		"/contests/" + id + "/problems/" + id + "/submissions",
		"/contests/" + id + "/standings",
		"/contests/" + id + "/submissions",
		"/contests/" + id + "/submissions/" + id,
		"/submissions/" + id,
		"/problems/" + id + "/submissions",
		"/problems/" + id + "/submissions/" + id,
		"/featured",
		"/featured/schedule",
		"/featured/standings?at=2026-10-19T14:00:00Z",
		// These handlers had one; it now comes from the public routes.
		"/problems",
		"/tags",
		"/users/alice",
		"/posts",
		"/contests/" + id + "/editorials",
		"/problems/" + id + "/samples",
		"/images/" + id,
	} {
		tracer.reset()
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		tracer.mu.Lock()
		queries, missing := tracer.queries, tracer.missing
		tracer.mu.Unlock()
		if queries == 0 || len(missing) > 0 {
			t.Errorf("GET %s: %d queries, without a deadline: %q", path, queries, missing)
		}
	}
}

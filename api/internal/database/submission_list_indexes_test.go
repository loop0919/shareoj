package database_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"judge/api/internal/database"
)

// TestSubmissionListStatisticsAfterMigratePostgres applies migration 052 to a table that already holds runs, as a deploy
// does, and checks that its statistics are gathered at once. CREATE STATISTICS gathers nothing by itself, and until
// ANALYZE runs the planner ignores the statistics and sorts every run of a filtered list.
func TestSubmissionListStatisticsAfterMigratePostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_list_statistics_%d", time.Now().UnixNano())
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
	// Take 052 back, fill the table, and apply 052 again.
	if _, err = pool.Exec(ctx, `DROP INDEX submissions_problem_shortest,submissions_problem_fastest,submissions_contest_shortest,submissions_contest_fastest;
		DROP STATISTICS submissions_job_flags,submissions_verdict; DELETE FROM schema_migrations WHERE version=52`); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice');
		INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
		SELECT gen_random_uuid(),'alice','a1000000-0000-4000-8000-000000000001',1,'t','cpp17','',
			jsonb_build_object('easyTest',i%10=0,'generate',i%20=1,'validate',i%20=2),'DONE',jsonb_build_object('verdict',(ARRAY['AC','WA','TLE'])[i%3+1])
		FROM generate_series(1,600) i`); err != nil {
		t.Fatal(err)
	}
	if err = database.Migrate(ctx, pool); err != nil {
		t.Fatal(err)
	}
	rows, err := pool.Query(ctx, `SELECT statistics_name,count(*) FILTER (WHERE most_common_vals IS NOT NULL) FROM pg_stats_ext_exprs
		WHERE schemaname=current_schema() AND tablename='submissions' GROUP BY 1`)
	if err != nil {
		t.Fatal(err)
	}
	gathered := map[string]int{}
	var name string
	var expressions int
	if _, err = pgx.ForEachRow(rows, []any{&name, &expressions}, func() error {
		gathered[name] = expressions
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if gathered["submissions_job_flags"] != 3 || gathered["submissions_verdict"] != 1 {
		t.Fatalf("statistics gathered for %v expressions, want submissions_job_flags 3 and submissions_verdict 1: 052 no longer analyses the table it builds them on", gathered)
	}
}

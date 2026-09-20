// Package database manages the shared PostgreSQL pool and versioned migrations.
package database

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func Open(ctx context.Context, url string) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, errors.New("invalid DATABASE_URL")
	}
	cfg.MaxConns = 4
	cfg.MinConns = 0
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, errors.New("cannot create database pool")
	}
	return pool, nil
}

//go:embed 001_problems.sql
var problemsSchema string

//go:embed 002_profiles.sql
var profilesSchema string

//go:embed 003_publications.sql
var publicationsSchema string

//go:embed 004_submissions.sql
var submissionsSchema string

//go:embed 005_judge_outbox.sql
var judgeOutboxSchema string

//go:embed 006_test_files.sql
var testFilesSchema string

//go:embed 007_multilanguage_dispatch.sql
var multilanguageDispatchSchema string

//go:embed 008_judge_progress.sql
var judgeProgressSchema string

//go:embed 009_generated_test_files.sql
var generatedTestFilesSchema string

//go:embed 010_problem_favorites.sql
var problemFavoritesSchema string

//go:embed 011_sample_case_flags.sql
var sampleCaseFlagsSchema string

//go:embed 012_contests.sql
var contestsSchema string

//go:embed 013_problem_submissions.sql
var problemSubmissionsSchema string

//go:embed 014_tester_invitations.sql
var testerInvitationsSchema string

//go:embed 015_profile_accounts.sql
var profileAccountsSchema string

//go:embed 016_notifications.sql
var notificationsSchema string

//go:embed 017_content_images.sql
var contentImagesSchema string

//go:embed 018_difficulty_votes.sql
var difficultyVotesSchema string

func Migrate(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(709091001)`); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version bigint PRIMARY KEY)`); err != nil {
		return err
	}
	for index, schema := range []string{problemsSchema, profilesSchema, publicationsSchema, submissionsSchema, judgeOutboxSchema, testFilesSchema, multilanguageDispatchSchema, judgeProgressSchema, generatedTestFilesSchema, problemFavoritesSchema, sampleCaseFlagsSchema, contestsSchema, problemSubmissionsSchema, testerInvitationsSchema, profileAccountsSchema, notificationsSchema, contentImagesSchema, difficultyVotesSchema} {
		version := index + 1
		var applied bool
		if err = tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&applied); err != nil {
			return err
		}
		if applied {
			continue
		}
		if _, err = tx.Exec(ctx, schema); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `INSERT INTO schema_migrations VALUES ($1)`, version); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

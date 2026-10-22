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

//go:embed 019_reserved.sql
var reservedSchema string

//go:embed 020_contest_participants.sql
var contestParticipantsSchema string

//go:embed 021_featured_problems.sql
var featuredProblemsSchema string

//go:embed 022_featured_previews.sql
var featuredPreviewsSchema string

//go:embed 023_creation_quotas.sql
var creationQuotasSchema string

//go:embed 024_registration_quotas.sql
var registrationQuotasSchema string

//go:embed 025_contest_drafts.sql
var contestDraftsSchema string

//go:embed 026_account_deletion.sql
var accountDeletionSchema string

//go:embed 027_user_editorials.sql
var userEditorialsSchema string

//go:embed 028_tags.sql
var tagsSchema string

//go:embed 029_scheduled_problems.sql
var scheduledProblemsSchema string

//go:embed 030_rejudges.sql
var rejudgesSchema string

//go:embed 031_contest_roles.sql
var contestRolesSchema string

//go:embed 032_contest_setters.sql
var contestSettersSchema string

//go:embed 033_contest_setter_draft_guard.sql
var contestSetterDraftGuardSchema string

//go:embed 034_contest_freeze.sql
var contestFreezeSchema string

//go:embed 035_contest_teams.sql
var contestTeamsSchema string

//go:embed 036_contest_team_visibility.sql
var contestTeamVisibilitySchema string

//go:embed 037_contest_onsite_kept_open.sql
var contestOnsiteKeptOpenSchema string

//go:embed 038_contest_team_individuals.sql
var contestTeamIndividualsSchema string

//go:embed 039_contest_onsite_drop_kept_open.sql
var contestOnsiteDropKeptOpenSchema string

//go:embed 040_submission_full_score.sql
var submissionFullScoreSchema string

//go:embed 041_contest_clarifications.sql
var contestClarificationsSchema string

//go:embed 042_contest_notifications.sql
var contestNotificationsSchema string

// migrations lists the schemas in version order: version N is the Nth entry, from NNN_*.sql.
// Add a migration at the end under the next number. Deployed databases record the versions they applied, so never
// remove or reorder an entry; a reverted migration keeps its number reserved, like 019_reserved.sql.
func migrations() []string {
	return []string{
		problemsSchema,
		profilesSchema,
		publicationsSchema,
		submissionsSchema,
		judgeOutboxSchema,
		testFilesSchema,
		multilanguageDispatchSchema,
		judgeProgressSchema,
		generatedTestFilesSchema,
		problemFavoritesSchema,
		sampleCaseFlagsSchema,
		contestsSchema,
		problemSubmissionsSchema,
		testerInvitationsSchema,
		profileAccountsSchema,
		notificationsSchema,
		contentImagesSchema,
		difficultyVotesSchema,
		reservedSchema,
		contestParticipantsSchema,
		featuredProblemsSchema,
		featuredPreviewsSchema,
		creationQuotasSchema,
		registrationQuotasSchema,
		contestDraftsSchema,
		accountDeletionSchema,
		userEditorialsSchema,
		tagsSchema,
		scheduledProblemsSchema,
		rejudgesSchema,
		contestRolesSchema,
		contestSettersSchema,
		contestSetterDraftGuardSchema,
		contestFreezeSchema,
		contestTeamsSchema,
		contestTeamVisibilitySchema,
		contestOnsiteKeptOpenSchema,
		contestTeamIndividualsSchema,
		contestOnsiteDropKeptOpenSchema,
		submissionFullScoreSchema,
		contestClarificationsSchema,
		contestNotificationsSchema,
	}
}

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
	for index, schema := range migrations() {
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

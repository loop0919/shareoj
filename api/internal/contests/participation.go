package contests

import (
	"context"
	"crypto/subtle"
	"errors"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
)

var ErrParticipationUnavailable = errors.New("contest participation unavailable")

// ErrParticipantProblem refuses a save that adds a problem a registered participant authors or tests, so the editor
// can name that cause instead of every reason a save may fail (ADR 0022).
var ErrParticipantProblem = errors.New("a participant authors or tests the problem")

// participantProblem reports whether a registered participant authors or tests a problem that a save would add.
// Adding it would make them staff while they compete (ADR 0022).
func participantProblem(ctx context.Context, q querier, id string, ids []string) (bool, error) {
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM problem_drafts d JOIN contest_participants p ON p.contest_id=$1
 WHERE d.id=ANY($2::uuid[]) AND NOT EXISTS(SELECT 1 FROM contest_problems cp WHERE cp.contest_id=$1 AND cp.problem_id=d.id)
 AND (p.owner_id=d.owner_id OR EXISTS(SELECT 1 FROM problem_testers t WHERE t.problem_id=d.id AND t.owner_id=p.owner_id)))`, id, ids).Scan(&found)
	return found, err
}

// JoinInput is the body of POST /my/contests/{id}/participation (ADR 0024). An empty body registers an individual,
// open participant, so clients from before teams keep working. A team contest needs Team. A registered participant
// who sends only OnsitePassphrase becomes onsite, with their team in a team contest.
type JoinInput struct {
	Team             *TeamAction `json:"team"`
	OnsitePassphrase *string     `json:"onsitePassphrase"`
}

// TeamAction creates a team named Name, or joins the team whose join code is Code.
type TeamAction struct {
	Action string `json:"action"`
	Name   string `json:"name"`
	Code   string `json:"code"`
}

// ErrInvalidJoin and ErrInvalidTeamName reject a body the client should never send.
var (
	ErrInvalidJoin     = errors.New("invalid participation request")
	ErrInvalidTeamName = errors.New("invalid team name")
)

// ValidJoin checks the shape of a join request. Codes and passphrases are only bounded here; a wrong one is an
// attempt, counted toward the limit.
func ValidJoin(in JoinInput) error {
	if p := in.OnsitePassphrase; p != nil && (utf8.RuneCountInString(*p) > 1000 || !utf8.ValidString(*p)) {
		return ErrInvalidJoin
	}
	if in.Team == nil {
		return nil
	}
	switch in.Team.Action {
	case "create":
		if !ValidTeamName(in.Team.Name) {
			return ErrInvalidTeamName
		}
	case "join":
		if code := NormalizeJoinCode(in.Team.Code); code == "" || utf8.RuneCountInString(code) > 64 {
			return ErrInvalidJoin
		}
	default:
		return ErrInvalidJoin
	}
	return nil
}

// checkAttempts refuses another guess while the account has attemptLimit wrong attempts on the contest within the
// window. The caller holds the account's lock on the contest, so parallel guesses cannot pass together.
func checkAttempts(ctx context.Context, tx pgx.Tx, id, owner string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contest_attempt_failures WHERE owner_id=$1 AND contest_id=$2
 AND failed_at<=clock_timestamp()-make_interval(mins => $3)`, owner, id, attemptWindowMinutes); err != nil {
		return err
	}
	var retryAfter int
	if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT GREATEST(1,CEIL(EXTRACT(EPOCH FROM f.failed_at+make_interval(mins => $3)-clock_timestamp())))::int
 FROM contest_attempt_failures f WHERE f.owner_id=$1 AND f.contest_id=$2 ORDER BY f.failed_at DESC OFFSET $4 LIMIT 1),0)`, owner, id, attemptWindowMinutes, attemptLimit-1).Scan(&retryAfter); err != nil {
		return err
	}
	if retryAfter > 0 {
		return &AttemptsError{RetryAfter: retryAfter}
	}
	return nil
}

// failAttempt records a wrong code or passphrase and commits only that, so the request changes nothing else.
func failAttempt(ctx context.Context, tx pgx.Tx, id, owner string, cause error) error {
	if _, err := tx.Exec(ctx, `INSERT INTO contest_attempt_failures(contest_id,owner_id) VALUES($1,$2)`, id, owner); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	return cause
}

// samePassphrase compares normalized passphrases in constant time.
func samePassphrase(sent, stored string) bool {
	return stored != "" && subtle.ConstantTimeCompare([]byte(sent), []byte(stored)) == 1
}

// Join registers the actor (ADR 0024). Staff, former staff and, after the end, newcomers are refused as before.
// In a team contest the actor creates a team or joins one by its code; one person is in one team. A correct venue
// passphrase makes the participant, or the whole team, onsite until the end, unless a setter moved them to open;
// a wrong one registers nothing.
func (s *Store) Join(ctx context.Context, id, owner string, in JoinInput) error {
	passphrase := ""
	if in.OnsitePassphrase != nil {
		passphrase = NormalizePassphrase(*in.OnsitePassphrase)
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// Serialize against schedule and problem-list edits and tester acceptance before checking eligibility.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR SHARE`, id).Scan(&locked); err != nil {
		return err
	}
	// One account's requests to one contest take turns, so a repeated request creates one team and parallel guesses
	// cannot pass the attempt limit together.
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1::text||' '||$2::text,709091024))`, owner, id); err != nil {
		return err
	}
	var st struct {
		competitor, ended, teamMode, division, registered bool
		size                                              int
		stored, code                                      string
		team                                              *string
	}
	if err = tx.QueryRow(ctx, `SELECT contest_competitor(c,$2),c.ends_at<=clock_timestamp(),c.team_mode,COALESCE(c.max_team_size,$3),
 c.onsite_division,COALESCE(c.onsite_passphrase,''),p.owner_id IS NOT NULL,p.team_id,COALESCE(t.join_code,'')
 FROM contests c LEFT JOIN contest_participants p ON p.contest_id=c.id AND p.owner_id=$2 LEFT JOIN contest_teams t ON t.id=p.team_id
 WHERE c.id=$1`, id, owner, MaxTeamMembers).Scan(&st.competitor, &st.ended, &st.teamMode, &st.size, &st.division, &st.stored,
		&st.registered, &st.team, &st.code); err != nil {
		return err
	}
	// A registered participant who sends the passphrase alone asks to become onsite, even when it is blank: answering
	// a blank one with success would tell them they are onsite while they stay open (ADR 0024).
	upgrading := st.registered && in.Team == nil && in.OnsitePassphrase != nil
	switch {
	case !st.competitor || st.ended && !st.registered:
		return ErrParticipationUnavailable
	case in.Team != nil && !st.teamMode:
		return ErrTeamUnavailable
	case (passphrase != "" || upgrading) && !st.division:
		return ErrOnsiteUnavailable
	}
	// Sending the code of one's own team again changes nothing, so a repeated request succeeds.
	if in.Team != nil && st.team != nil && (in.Team.Action != "join" || NormalizeJoinCode(in.Team.Code) != st.code) {
		return ErrAlreadyInTeam
	}
	entering := in.Team != nil && st.team == nil
	// A registration without a team can only come from an API before teams; it may still enter a team.
	if st.teamMode && st.team == nil && !entering && !(st.registered && in.OnsitePassphrase == nil) {
		return ErrTeamRequired
	}
	if st.ended && (entering || passphrase != "" || upgrading) {
		return ErrParticipationUnavailable
	}
	// A blank passphrase is no guess, so it is not counted.
	if upgrading && passphrase == "" {
		return ErrIncorrectPassphrase
	}
	if !entering && passphrase == "" {
		if !st.registered {
			if _, err = tx.Exec(ctx, `INSERT INTO contest_participants(contest_id,owner_id) VALUES($1,$2)`, id, owner); err != nil {
				return err
			}
		}
		return tx.Commit(ctx)
	}
	creating, joining := entering && in.Team.Action == "create", entering && in.Team.Action == "join"
	// A taken name is not a guess, so it is refused before the limit is checked.
	if creating {
		if taken, err := nameTaken(ctx, tx, id, "", in.Team.Name); err != nil || taken {
			if err == nil {
				err = ErrTeamNameTaken
			}
			return err
		}
	}
	if joining || passphrase != "" {
		if err = checkAttempts(ctx, tx, id, owner); err != nil {
			return err
		}
	}
	var team string
	// Whether a setter moved the team, or the participant, to open. Their switch updates the same row this locks.
	var kept bool
	switch {
	case joining:
		err = tx.QueryRow(ctx, `SELECT id,kept_open FROM contest_teams WHERE contest_id=$1 AND join_code=$2 FOR UPDATE`, id, NormalizeJoinCode(in.Team.Code)).Scan(&team, &kept)
		if errors.Is(err, pgx.ErrNoRows) {
			return failAttempt(ctx, tx, id, owner, ErrIncorrectTeamCode)
		}
		if err != nil {
			return err
		}
	case st.team != nil:
		// A member may be removed, or leave, meanwhile; the team's lock orders that, and membership is read again.
		var member bool
		err = tx.QueryRow(ctx, `SELECT id,kept_open FROM contest_teams WHERE id=$1 FOR UPDATE`, *st.team).Scan(&team, &kept)
		if err == nil {
			err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id=$1 AND owner_id=$2 AND team_id=$3)`, id, owner, team).Scan(&member)
		}
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !member {
			err = ErrConflict
		}
		if err != nil {
			return err
		}
	case st.registered && passphrase != "":
		err = tx.QueryRow(ctx, `SELECT kept_open FROM contest_participants WHERE contest_id=$1 AND owner_id=$2 FOR UPDATE`, id, owner).Scan(&kept)
		if err != nil {
			return err
		}
	}
	// The setter's decision stands, whoever holds the passphrase; it is not a guess, so nothing is counted.
	if passphrase != "" && kept {
		return ErrOnsiteKeptOpen
	}
	if passphrase != "" && !samePassphrase(passphrase, st.stored) {
		return failAttempt(ctx, tx, id, owner, ErrIncorrectPassphrase)
	}
	if joining {
		var size int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM contest_participants WHERE team_id=$1`, team).Scan(&size); err != nil {
			return err
		}
		if size >= st.size {
			return ErrTeamFull
		}
	}
	if creating {
		if team, err = createTeam(ctx, tx, id, in.Team.Name); err != nil {
			return err
		}
	}
	var teamID *string
	if team != "" {
		teamID = &team
	}
	switch {
	case !st.registered:
		_, err = tx.Exec(ctx, `INSERT INTO contest_participants(contest_id,owner_id,team_id) VALUES($1,$2,$3)`, id, owner, teamID)
	case entering:
		_, err = tx.Exec(ctx, `UPDATE contest_participants SET team_id=$3 WHERE contest_id=$1 AND owner_id=$2 AND team_id IS NULL`, id, owner, teamID)
	}
	if err != nil {
		return err
	}
	if passphrase != "" {
		if st.teamMode {
			_, err = tx.Exec(ctx, `UPDATE contest_teams SET onsite_at=clock_timestamp(),onsite_via='passphrase' WHERE id=$1 AND onsite_at IS NULL`, team)
		} else {
			_, err = tx.Exec(ctx, `UPDATE contest_participants SET onsite_at=clock_timestamp(),onsite_via='passphrase'
 WHERE contest_id=$1 AND owner_id=$2 AND onsite_at IS NULL`, id, owner)
		}
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

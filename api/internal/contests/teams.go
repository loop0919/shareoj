package contests

import (
	"context"
	"crypto/rand"
	"errors"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"golang.org/x/text/unicode/norm"
)

const (
	// MaxTeamMembers caps every team, also in contests without a limit of their own. Rate limits apply per account,
	// so a larger team would submit to one standings row, and load the judges, like that many individuals (ADR 0024).
	MaxTeamMembers = 50
	// MinTeamSize is the smallest limit a setter may choose; one person may still form a team of one, or enter alone.
	MinTeamSize   = 2
	maxTeamName   = 40
	minPassphrase = 4
	maxPassphrase = 64
	// Wrong join codes and venue passphrases share one limit per account and contest.
	attemptLimit         = 10
	attemptWindowMinutes = 10
	// Join codes leave out 0, O, 1, I, L and U, which are easy to misread when read out or written down.
	codeAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
	codeLength   = 10
)

var (
	// ErrTeamUnavailable means a team was named in an individual contest.
	ErrTeamUnavailable = errors.New("the contest is not a team contest")
	// ErrIndividualLocked refuses a team to someone who entered a team contest alone, once it has started: their
	// submissions so far are their own and must never count for a team (ADR 0024).
	ErrIndividualLocked = errors.New("an individual participant cannot enter a team after the start")
	// ErrOnsiteUnavailable means a venue passphrase, or an onsite change, was sent for a contest without the division.
	ErrOnsiteUnavailable = errors.New("the contest has no onsite division")
	// ErrOnsiteIrreversible refuses a setter's switch to open: participants and teams only ever become onsite.
	ErrOnsiteIrreversible = errors.New("onsite participants stay onsite")
	// ErrIncorrectTeamCode and ErrIncorrectPassphrase count toward the attempt limit.
	ErrIncorrectTeamCode   = errors.New("no team has this join code")
	ErrIncorrectPassphrase = errors.New("the venue passphrase is incorrect")
	ErrTeamFull            = errors.New("the team is full")
	ErrTeamNameTaken       = errors.New("another team has this name")
	// ErrAlreadyInTeam means the actor already belongs to a team of the contest; one person joins one team.
	ErrAlreadyInTeam = errors.New("already in a team of this contest")
	// ErrTeamNotFound means the actor is in no team of the contest, or a setter named no team of it.
	ErrTeamNotFound = errors.New("team not found")
	// ErrTeamMemberNotFound means the handle names nobody in the actor's team.
	ErrTeamMemberNotFound = errors.New("team member not found")
	// ErrTeamLocked refuses a member's change outside its period: members change only before the start, and the
	// name and join code until the end.
	ErrTeamLocked = errors.New("the team can no longer be changed")
	// ErrParticipantNotFound means a setter named nobody registered for the individual contest.
	ErrParticipantNotFound = errors.New("participant not found")
	// ErrTeamModeLocked and ErrTeamSizeLocked refuse settings that would split the registrations (ADR 0024).
	ErrTeamModeLocked = errors.New("registrations fix the team mode")
	ErrTeamSizeLocked = errors.New("a team is larger than the new limit")
)

// AttemptsError means the account sent too many wrong join codes or venue passphrases to the contest recently.
type AttemptsError struct{ RetryAfter int }

func (e *AttemptsError) Error() string { return "too many wrong attempts" }

// TeamSummary is a team as anyone may see it: standings show team names and members from before the start.
type TeamSummary struct {
	ID      string   `json:"id"`
	Name    string   `json:"name"`
	Members []string `json:"members"`
	Onsite  bool     `json:"onsite"`
}

// Team is what members see of their own team, including the join code that lets others in.
type Team struct {
	TeamSummary
	Code string `json:"code"`
}

// Entrant is one row of the setters' participant list: a team, or a participant who entered alone, as in the standings.
// OnsiteVia tells how they became onsite, "passphrase" or "setter", and is empty while they are open.
type Entrant struct {
	Handle    string       `json:"handle"`
	Team      *TeamSummary `json:"team"`
	Onsite    bool         `json:"onsite"`
	OnsiteAt  *time.Time   `json:"onsiteAt"`
	OnsiteVia string       `json:"onsiteVia"`
	CreatedAt time.Time    `json:"createdAt"`
}

// Venue is the onsite division and its passphrase, for setters only.
type Venue struct {
	OnsiteDivision bool    `json:"onsiteDivision"`
	Passphrase     *string `json:"passphrase"`
}

// NormalizeTeamName folds width variants with NFKC and collapses runs of spaces, as tags do (ADR 0018), so a name
// cannot copy another team's by width or spacing alone.
func NormalizeTeamName(name string) string {
	return strings.Join(strings.Fields(norm.NFKC.String(name)), " ")
}

// teamKey compares names regardless of letter case.
func teamKey(name string) string { return strings.ToLower(NormalizeTeamName(name)) }

// hidden reports characters that do not show as themselves: control and format characters (zero-width characters,
// direction controls), the other default-ignorable ones (variation selectors, the combining grapheme joiner, Hangul
// fillers), the blank Braille pattern, line and paragraph separators, and private-use characters. Each could make a
// name that looks like another team's yet has another key (ADR 0024).
func hidden(r rune) bool {
	return r == '\u2800' || unicode.In(r, unicode.Cc, unicode.Cf, unicode.Co, unicode.Zl, unicode.Zp, unicode.Variation_Selector,
		unicode.Other_Default_Ignorable_Code_Point)
}

// visible reports a character that shows on its own, so a text of combining marks alone is not a name.
func visible(r rune) bool { return unicode.In(r, unicode.L, unicode.N, unicode.P, unicode.S) }

// shown accepts normalized text that has a visible character and no hidden one.
func shown(s string) bool {
	return strings.IndexFunc(s, hidden) < 0 && strings.IndexFunc(s, visible) >= 0
}

// ValidTeamName accepts 1 to 40 characters after normalization, without hidden characters.
func ValidTeamName(name string) bool {
	n := utf8.RuneCountInString(NormalizeTeamName(name))
	return utf8.ValidString(name) && shown(norm.NFKC.String(name)) && n >= 1 && n <= maxTeamName
}

// NormalizePassphrase trims and folds width variants: the venue passphrase is read out or handed out on paper, so
// full-width input or a stray space must not fail (ADR 0024).
func NormalizePassphrase(passphrase string) string {
	return strings.TrimSpace(norm.NFKC.String(passphrase))
}

// ValidPassphrase accepts 4 to 64 ASCII letters, digits and underscores after normalization, so full-width input
// still counts. Other characters are hard to read out or type on a phone at the venue, and some look alike.
func ValidPassphrase(passphrase string) bool {
	p := NormalizePassphrase(passphrase)
	return len(p) >= minPassphrase && len(p) <= maxPassphrase && strings.IndexFunc(p, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_')
	}) < 0
}

// passphrase is the normalized passphrase to store: only when the save turns the division on, which requires one.
func (in Input) passphrase() *string {
	if in.OnsiteDivision == nil || !*in.OnsiteDivision || in.OnsitePassphrase == nil {
		return nil
	}
	p := NormalizePassphrase(*in.OnsitePassphrase)
	return &p
}

// NormalizeJoinCode accepts a code as people copy or type it: in full width or lower case, with spaces or hyphens.
func NormalizeJoinCode(code string) string {
	return strings.Map(func(r rune) rune {
		if r == '-' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, norm.NFKC.String(code))
}

// newJoinCode draws each character uniformly from codeAlphabet: about 49 bits, beyond guessing under the attempt limit.
func newJoinCode() string {
	code := make([]byte, 0, codeLength)
	var random [32]byte
	limit := 256 - 256%len(codeAlphabet)
	for len(code) < codeLength {
		_, _ = rand.Read(random[:])
		for _, b := range random {
			if int(b) < limit && len(code) < codeLength {
				code = append(code, codeAlphabet[int(b)%len(codeAlphabet)])
			}
		}
	}
	return string(code)
}

// members lists the handles in the team aliased t, in the order they joined.
func members(t string) string {
	return `ARRAY(SELECT mu.handle FROM contest_participants m JOIN user_profiles mu ON mu.owner_id=m.owner_id
 WHERE m.team_id=` + t + `.id ORDER BY m.created_at,m.owner_id)`
}

// keepRegistrations refuses settings that would split the registrations (ADR 0024): the team mode cannot change once
// someone has registered, and the maximum team size cannot drop below the largest team. Joining holds the contest row
// in share mode, so the registrations read here stay until the save commits. Dropping the onsite division before the
// start makes everyone open again, so a division turned on later starts afresh.
func keepRegistrations(ctx context.Context, tx pgx.Tx, id string, teamMode bool, in Input) error {
	var registered bool
	var largest int
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_participants WHERE contest_id=$1),
 COALESCE((SELECT max(n) FROM (SELECT count(*) n FROM contest_participants WHERE contest_id=$1 AND team_id IS NOT NULL GROUP BY team_id) x),0)`, id).Scan(&registered, &largest); err != nil {
		return err
	}
	if in.TeamMode != nil && *in.TeamMode != teamMode && registered {
		return ErrTeamModeLocked
	}
	if size := in.MaxTeamSize.Value; size != nil && *size < largest {
		return ErrTeamSizeLocked
	}
	if in.OnsiteDivision != nil && !*in.OnsiteDivision {
		for _, table := range []string{"contest_participants", "contest_teams"} {
			if _, err := tx.Exec(ctx, `UPDATE `+table+` SET onsite_at=NULL,onsite_via=NULL WHERE contest_id=$1 AND onsite_at IS NOT NULL`, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// createTeam adds a team with a fresh join code. Another team may take the name meanwhile; a code collision draws again.
func createTeam(ctx context.Context, tx pgx.Tx, id, name string) (string, error) {
	for range 5 {
		var team string
		err := tx.QueryRow(ctx, `INSERT INTO contest_teams(id,contest_id,name,name_key,join_code) VALUES(gen_random_uuid(),$1,$2,$3,$4)
 ON CONFLICT DO NOTHING RETURNING id`, id, NormalizeTeamName(name), teamKey(name), newJoinCode()).Scan(&team)
		if !errors.Is(err, pgx.ErrNoRows) {
			return team, err
		}
		if taken, err := nameTaken(ctx, tx, id, "", name); err != nil || taken {
			if err == nil {
				err = ErrTeamNameTaken
			}
			return "", err
		}
	}
	return "", ErrConflict
}

// nameTaken reports whether a team of the contest other than except has the name, regardless of case.
func nameTaken(ctx context.Context, q querier, id, except, name string) (bool, error) {
	var taken bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM contest_teams WHERE contest_id=$1 AND name_key=$2 AND id::text<>$3)`, id, teamKey(name), except).Scan(&taken)
	return taken, err
}

// renameTeam renames a team the caller has locked.
func renameTeam(ctx context.Context, tx pgx.Tx, id, team, name string) error {
	taken, err := nameTaken(ctx, tx, id, team, name)
	if err != nil {
		return err
	}
	if taken {
		return ErrTeamNameTaken
	}
	_, err = tx.Exec(ctx, `UPDATE contest_teams SET name=$2,name_key=$3 WHERE id=$1`, team, NormalizeTeamName(name), teamKey(name))
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" {
		return ErrTeamNameTaken
	}
	return err
}

// renewCode gives a team a new join code; the old one stops working.
func renewCode(ctx context.Context, tx pgx.Tx, id, team string) error {
	for range 5 {
		tag, err := tx.Exec(ctx, `UPDATE contest_teams SET join_code=$3 WHERE id=$1
 AND NOT EXISTS(SELECT 1 FROM contest_teams WHERE contest_id=$2 AND join_code=$3)`, team, id, newJoinCode())
		if err != nil || tag.RowsAffected() == 1 {
			return err
		}
	}
	return ErrConflict
}

// RenewTeamCodes gives each of the teams that still exists a new join code. Account deletion takes the account out of
// teams the caller has locked, and the person behind it knows their codes.
func RenewTeamCodes(ctx context.Context, tx pgx.Tx, teams []string) error {
	rows, err := tx.Query(ctx, `SELECT contest_id::text,id::text FROM contest_teams WHERE id=ANY($1::uuid[]) ORDER BY id`, teams)
	if err != nil {
		return err
	}
	left, err := pgx.CollectRows(rows, pgx.RowToStructByPos[struct{ Contest, Team string }])
	if err != nil {
		return err
	}
	for _, t := range left {
		if err = renewCode(ctx, tx, t.Contest, t.Team); err != nil {
			return err
		}
	}
	return nil
}

var teamColumns = `t.id,t.name,c.onsite_division AND t.onsite_at IS NOT NULL,` + members("t") + `,t.join_code`

// MyTeam returns the actor's team with its join code; only members see the code.
func (s *Store) MyTeam(ctx context.Context, id, actor string) (Team, error) {
	return myTeam(ctx, s.Pool, id, actor)
}

func myTeam(ctx context.Context, q querier, id, actor string) (Team, error) {
	var t Team
	err := q.QueryRow(ctx, `SELECT `+teamColumns+` FROM contest_participants p JOIN contest_teams t ON t.id=p.team_id
 JOIN contests c ON c.id=p.contest_id WHERE p.contest_id=$1 AND p.owner_id=$2`, id, actor).Scan(&t.ID, &t.Name, &t.Onsite, &t.Members, &t.Code)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrTeamNotFound
	}
	return t, err
}

type lockedTeam struct {
	id             string
	started, ended bool
}

// lockTeam locks the actor's team, in the order Join uses: the contest in share mode, then the team. Membership is read
// again after the wait, since another member may have removed the actor meanwhile.
func lockTeam(ctx context.Context, tx pgx.Tx, id, actor string) (lockedTeam, error) {
	var t lockedTeam
	var locked string
	err := tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR SHARE`, id).Scan(&locked)
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT t.id FROM contest_participants p JOIN contest_teams t ON t.id=p.team_id
 WHERE p.contest_id=$1 AND p.owner_id=$2 FOR UPDATE OF t`, id, actor).Scan(&t.id)
	}
	var member bool
	if err == nil {
		err = tx.QueryRow(ctx, `SELECT c.starts_at<=clock_timestamp(),c.ends_at<=clock_timestamp(),
 EXISTS(SELECT 1 FROM contest_participants p WHERE p.contest_id=c.id AND p.owner_id=$2 AND p.team_id=$3)
 FROM contests c WHERE c.id=$1`, id, actor, t.id).Scan(&t.started, &t.ended, &member)
	}
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !member {
		err = ErrTeamNotFound
	}
	return t, err
}

// changeTeam runs change on the actor's locked team until the end and returns the team as it is then.
func (s *Store) changeTeam(ctx context.Context, id, actor string, change func(pgx.Tx, string) error) (Team, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Team{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	t, err := lockTeam(ctx, tx, id, actor)
	if err != nil {
		return Team{}, err
	}
	if t.ended {
		return Team{}, ErrTeamLocked
	}
	if err = change(tx, t.id); err != nil {
		return Team{}, err
	}
	team, err := myTeam(ctx, tx, id, actor)
	if err != nil {
		return team, err
	}
	return team, tx.Commit(ctx)
}

// RenewTeamCode replaces the join code of the actor's team until the end; the old code stops letting anyone in.
func (s *Store) RenewTeamCode(ctx context.Context, id, actor string) (Team, error) {
	return s.changeTeam(ctx, id, actor, func(tx pgx.Tx, team string) error { return renewCode(ctx, tx, id, team) })
}

// RenameTeam renames the actor's team until the end. The caller has checked the name with ValidTeamName.
func (s *Store) RenameTeam(ctx context.Context, id, actor, name string) (Team, error) {
	return s.changeTeam(ctx, id, actor, func(tx pgx.Tx, team string) error { return renameTeam(ctx, tx, id, team, name) })
}

// RemoveTeamMember takes a member off the actor's team before the start: the actor leaves when handle is their own.
// The member's registration goes with them, and the code is renewed whoever goes: one who left could otherwise come
// back, or send in another account that nobody can remove after the start. The last member to leave deletes the team
// (ADR 0024).
func (s *Store) RemoveTeamMember(ctx context.Context, id, actor, handle string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	t, err := lockTeam(ctx, tx, id, actor)
	if err != nil {
		return err
	}
	var member string
	err = tx.QueryRow(ctx, `SELECT p.owner_id FROM contest_participants p JOIN user_profiles u ON u.owner_id=p.owner_id
 WHERE p.contest_id=$1 AND p.team_id=$2 AND u.handle=$3`, id, t.id, handle).Scan(&member)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrTeamMemberNotFound
	}
	if err != nil {
		return err
	}
	if t.started {
		return ErrTeamLocked
	}
	if err = renewCode(ctx, tx, id, t.id); err != nil {
		return err
	}
	if err = dropMember(ctx, tx, id, t.id, member); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// dropMember removes a registration from a locked team, and the team once nobody is left in it.
func dropMember(ctx context.Context, tx pgx.Tx, id, team, member string) error {
	if _, err := tx.Exec(ctx, `DELETE FROM contest_participants WHERE contest_id=$1 AND owner_id=$2`, id, member); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM contest_teams t WHERE t.id=$1 AND NOT EXISTS(SELECT 1 FROM contest_participants p WHERE p.team_id=t.id)`, team)
	return err
}

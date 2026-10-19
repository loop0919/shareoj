package contests

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Venue returns the onsite division and its passphrase to a setter. Testers and problem authors are staff but never
// see the passphrase (ADR 0024).
func (s *Store) Venue(ctx context.Context, id, actor string) (Venue, error) {
	var v Venue
	err := s.Pool.QueryRow(ctx, `SELECT onsite_division,onsite_passphrase FROM contests WHERE id=$1 AND contest_setter(id,$2)`, id, actor).Scan(&v.OnsiteDivision, &v.Passphrase)
	return v, err
}

// RotatePassphrase replaces the venue passphrase until the end, in case it leaks; it cannot be cleared, since the
// division is fixed at the start. It counts as an edit, so an editor holding the old passphrase cannot save it back.
func (s *Store) RotatePassphrase(ctx context.Context, id, actor, passphrase string) (Venue, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return Venue{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	// The lock orders the change against joining, which reads the passphrase under a share lock.
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR NO KEY UPDATE`, id).Scan(&locked); err != nil {
		return Venue{}, err
	}
	var setter, division, ended bool
	if err = tx.QueryRow(ctx, `SELECT contest_setter(id,$2),onsite_division,ends_at<=clock_timestamp() FROM contests WHERE id=$1`, id, actor).Scan(&setter, &division, &ended); err != nil {
		return Venue{}, err
	}
	switch {
	case !setter:
		return Venue{}, pgx.ErrNoRows
	case !division:
		return Venue{}, ErrOnsiteUnavailable
	case ended:
		return Venue{}, ErrConflict
	}
	p := NormalizePassphrase(passphrase)
	if _, err = tx.Exec(ctx, `UPDATE contests SET onsite_passphrase=$2,version=version+1 WHERE id=$1`, id, p); err != nil {
		return Venue{}, err
	}
	return Venue{OnsiteDivision: true, Passphrase: &p}, tx.Commit(ctx)
}

// entrants reads the rows of the setters' list, oldest first: the teams, and the participants in no team, whom the
// standings show as rows of their own. $2 narrows it to one team, $3 to one participant. Participants left out of
// the standings are left out here too.
var entrants = `SELECT * FROM (
 SELECT '' handle,t.id::text team,t.name,` + members("t") + ` members,c.onsite_division AND t.onsite_at IS NOT NULL onsite,t.onsite_at,
  COALESCE(t.onsite_via,'') via,t.created_at
 FROM contest_teams t JOIN contests c ON c.id=t.contest_id WHERE c.id=$1 AND ($2='' AND $3='' OR t.id::text=$2)
 UNION ALL
 SELECT u.handle,NULL,'',ARRAY[]::text[],c.onsite_division AND p.onsite_at IS NOT NULL,p.onsite_at,COALESCE(p.onsite_via,''),p.created_at
 FROM contest_participants p JOIN contests c ON c.id=p.contest_id JOIN user_profiles u ON u.owner_id=p.owner_id
 WHERE c.id=$1 AND p.team_id IS NULL AND contest_competitor(c,p.owner_id) AND ($2='' AND $3='' OR u.handle=$3)) e
 ORDER BY e.created_at,e.handle,e.team`

func readEntrants(ctx context.Context, q querier, id, team, handle string) ([]Entrant, error) {
	rows, err := q.Query(ctx, entrants, id, team, handle)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Entrant, error) {
		var e Entrant
		var t TeamSummary
		var teamID *string
		err := row.Scan(&e.Handle, &teamID, &t.Name, &t.Members, &e.Onsite, &e.OnsiteAt, &e.OnsiteVia, &e.CreatedAt)
		if teamID != nil {
			t.ID, t.Onsite = *teamID, e.Onsite
			e.Team = &t
		}
		return e, err
	})
}

// Entrants lists, for setters, the teams and the participants who entered alone, with when and how each became
// onsite, so a late switch by passphrase stands out.
func (s *Store) Entrants(ctx context.Context, id, actor string) ([]Entrant, error) {
	var found int
	if err := s.Pool.QueryRow(ctx, `SELECT 1 FROM contests WHERE id=$1 AND contest_setter(id,$2)`, id, actor).Scan(&found); err != nil {
		return nil, err
	}
	return readEntrants(ctx, s.Pool, id, "", "")
}

// changeEntrant runs change for a setter until the results are final, after locking the contest as joining does, so
// the onsite division cannot be dropped meanwhile. teamMode tells whether the contest has teams.
func (s *Store) changeEntrant(ctx context.Context, id, actor string, change func(tx pgx.Tx, teamMode, division bool) error) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var locked string
	if err = tx.QueryRow(ctx, `SELECT id FROM contests WHERE id=$1 FOR SHARE`, id).Scan(&locked); err != nil {
		return err
	}
	var setter, teamMode, division, final bool
	if err = tx.QueryRow(ctx, `SELECT contest_setter(c.id,$2),c.team_mode,c.onsite_division,COALESCE(contest_results_final_at(c)<=clock_timestamp(),false)
 FROM contests c WHERE c.id=$1`, id, actor).Scan(&setter, &teamMode, &division, &final); err != nil {
		return err
	}
	switch {
	case !setter:
		return pgx.ErrNoRows
	case final:
		return ErrConflict
	}
	if err = change(tx, teamMode, division); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// one reads the single entrant a change touched.
func one(ctx context.Context, tx pgx.Tx, id, team, handle string) (Entrant, error) {
	list, err := readEntrants(ctx, tx, id, team, handle)
	if err == nil && len(list) == 0 {
		err = notFound(team)
	}
	if err != nil {
		return Entrant{}, err
	}
	return list[0], nil
}

// SetOnsite makes a team (by id) or a participant who entered alone (by handle) onsite until the results are final,
// for someone at the venue who missed the passphrase (ADR 0024). A switch keeps the first time and route. Nobody is
// moved back to open: the setters decided that no path back is needed.
func (s *Store) SetOnsite(ctx context.Context, id, actor, handle, team string, onsite bool) (Entrant, error) {
	var e Entrant
	err := s.changeEntrant(ctx, id, actor, func(tx pgx.Tx, _, division bool) error {
		var tag pgconn.CommandTag
		var err error
		switch {
		case !onsite:
			return ErrOnsiteIrreversible
		case !division:
			return ErrOnsiteUnavailable
		case team != "":
			tag, err = tx.Exec(ctx, `UPDATE contest_teams SET onsite_at=COALESCE(onsite_at,clock_timestamp()),onsite_via=COALESCE(onsite_via,'setter')
 WHERE contest_id=$1 AND id::text=$2`, id, team)
		default:
			// A member's division is their team's, so a member's handle names no entrant.
			tag, err = tx.Exec(ctx, `UPDATE contest_participants p SET onsite_at=COALESCE(p.onsite_at,clock_timestamp()),onsite_via=COALESCE(p.onsite_via,'setter')
 FROM contests c, user_profiles u WHERE c.id=p.contest_id AND u.owner_id=p.owner_id AND p.contest_id=$1 AND u.handle=$2
 AND p.team_id IS NULL AND contest_competitor(c,p.owner_id)`, id, handle)
		}
		if err == nil && tag.RowsAffected() == 0 {
			err = notFound(team)
		}
		if err != nil {
			return err
		}
		e, err = one(ctx, tx, id, team, handle)
		return err
	})
	return e, err
}

// RenameEntrantTeam lets a setter rename a team until the results are final, against unfit names or impersonation.
// The caller has checked the name with ValidTeamName.
func (s *Store) RenameEntrantTeam(ctx context.Context, id, actor, team, name string) (Entrant, error) {
	var e Entrant
	err := s.changeEntrant(ctx, id, actor, func(tx pgx.Tx, teamMode, _ bool) error {
		var locked string
		err := tx.QueryRow(ctx, `SELECT id FROM contest_teams WHERE contest_id=$1 AND id::text=$2 FOR UPDATE`, id, team).Scan(&locked)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !teamMode {
			err = ErrTeamNotFound
		}
		if err == nil {
			err = renameTeam(ctx, tx, id, team, name)
		}
		if err == nil {
			e, err = one(ctx, tx, id, team, "")
		}
		return err
	})
	return e, err
}

func notFound(team string) error {
	if team != "" {
		return ErrTeamNotFound
	}
	return ErrParticipantNotFound
}

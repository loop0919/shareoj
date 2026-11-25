// Package folders keeps the folders a user makes to sort their problems and posts on the my page. A folder is its
// owner's alone: nobody else reads it, and it changes no visibility or permission.
package folders

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const (
	MaxFolders = 100
	MaxDepth   = 3
	MaxName    = 40
	MaxItems   = 100
)

var (
	ErrNotFound  = errors.New("folder not found")
	ErrInvalid   = errors.New("invalid folder")
	ErrLimit     = errors.New("too many folders")
	ErrDepth     = errors.New("folders nested too deep")
	ErrNameTaken = errors.New("folder name already used")
	ErrCycle     = errors.New("folder moved into itself")
)

// Folder is a folder of the owner. ParentID is nil at the top.
type Folder struct {
	ID       string  `json:"id"`
	ParentID *string `json:"parentId"`
	Name     string  `json:"name"`
}

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool} }

var folderID = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// cleanName trims the name and reports whether it is 1 to MaxName characters without control characters.
func cleanName(name string) (string, bool) {
	name = strings.TrimSpace(name)
	n := utf8.RuneCountInString(name)
	return name, utf8.ValidString(name) && n >= 1 && n <= MaxName && strings.IndexFunc(name, unicode.IsControl) < 0
}

func validParent(parent *string) bool { return parent == nil || folderID.MatchString(*parent) }

const columns = `id::text,parent_id::text,name`

func scan(row pgx.Row) (Folder, error) {
	var f Folder
	err := row.Scan(&f.ID, &f.ParentID, &f.Name)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "content_folders_sibling_name" {
		err = ErrNameTaken
	}
	return f, err
}

// Folders lists the owner's folders by name, then id.
func (s *Store) Folders(ctx context.Context, owner string) ([]Folder, error) {
	rows, err := s.pool.Query(ctx, `SELECT `+columns+` FROM content_folders WHERE owner_id=$1 ORDER BY name,id`, owner)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Folder, error) { return scan(row) })
}

// Create makes a folder at the top, or under parent.
func (s *Store) Create(ctx context.Context, owner, name string, parent *string) (Folder, error) {
	name, ok := cleanName(name)
	if !ok || !validParent(parent) {
		return Folder{}, ErrInvalid
	}
	tx, t, err := s.begin(ctx, owner)
	if err != nil {
		return Folder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if !t.has(parent) {
		return Folder{}, ErrNotFound
	}
	if len(t) >= MaxFolders {
		return Folder{}, ErrLimit
	}
	if t.depth(parent)+1 > MaxDepth {
		return Folder{}, ErrDepth
	}
	f, err := scan(tx.QueryRow(ctx, `INSERT INTO content_folders(id,owner_id,parent_id,name) VALUES(gen_random_uuid(),$1,$2,$3) RETURNING `+columns, owner, parent, name))
	if err != nil {
		return Folder{}, err
	}
	return f, tx.Commit(ctx)
}

// Update renames the folder and moves it, with the folders below it, to the top or under parent.
func (s *Store) Update(ctx context.Context, owner, id, name string, parent *string) (Folder, error) {
	name, ok := cleanName(name)
	if !ok || !folderID.MatchString(id) || !validParent(parent) {
		return Folder{}, ErrInvalid
	}
	tx, t, err := s.begin(ctx, owner)
	if err != nil {
		return Folder{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if !t.has(&id) || !t.has(parent) {
		return Folder{}, ErrNotFound
	}
	if parent != nil && t.within(*parent, id) {
		return Folder{}, ErrCycle
	}
	if t.depth(parent)+t.height(id) > MaxDepth {
		return Folder{}, ErrDepth
	}
	f, err := scan(tx.QueryRow(ctx, `UPDATE content_folders SET name=$3,parent_id=$4 WHERE owner_id=$1 AND id=$2 RETURNING `+columns, owner, id, name, parent))
	if err != nil {
		return Folder{}, err
	}
	return f, tx.Commit(ctx)
}

// Delete deletes the folder and the folders below it. What they held becomes unsorted.
func (s *Store) Delete(ctx context.Context, owner, id string) error {
	if !folderID.MatchString(id) {
		return ErrInvalid
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err = lock(ctx, tx, owner); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `DELETE FROM content_folders WHERE owner_id=$1 AND id=$2`, owner, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return tx.Commit(ctx)
}

// lock takes the owner's profile row, so that the owner's writes run one at a time: nothing else of the owner's
// changes between counting the folders and writing.
func lock(ctx context.Context, tx pgx.Tx, owner string) error {
	err := tx.QueryRow(ctx, `SELECT 1 FROM user_profiles WHERE owner_id=$1 AND deleted_at IS NULL FOR UPDATE`, owner).Scan(new(int))
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

// begin starts a write under the owner's lock and reads the owner's folders to check it against.
func (s *Store) begin(ctx context.Context, owner string) (pgx.Tx, tree, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, nil, err
	}
	var t tree
	if err = lock(ctx, tx, owner); err == nil {
		t, err = read(ctx, tx, owner)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, nil, err
	}
	return tx, t, nil
}

// read maps each folder of the owner to its parent.
func read(ctx context.Context, tx pgx.Tx, owner string) (tree, error) {
	rows, err := tx.Query(ctx, `SELECT `+columns+` FROM content_folders WHERE owner_id=$1`, owner)
	if err != nil {
		return nil, err
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Folder, error) { return scan(row) })
	if err != nil {
		return nil, err
	}
	t := make(tree, len(list))
	for _, f := range list {
		t[f.ID] = f.ParentID
	}
	return t, nil
}

// tree maps each folder of an owner to its parent, nil at the top. Walks stop after len(t) steps, so that they end
// even on a cycle, which writes never make.
type tree map[string]*string

// has reports whether the folder is one of the owner's; no folder, the top, is always there.
func (t tree) has(id *string) bool {
	if id == nil {
		return true
	}
	_, ok := t[*id]
	return ok
}

// depth is the level of the folder: 1 at the top, and 0 for no folder.
func (t tree) depth(id *string) int {
	n := 0
	for ; id != nil && n <= len(t); id = t[*id] {
		n++
	}
	return n
}

// within reports whether folder f is the folder id or below it.
func (t tree) within(f, id string) bool {
	for range len(t) + 1 {
		if f == id {
			return true
		}
		parent := t[f]
		if parent == nil {
			return false
		}
		f = *parent
	}
	return false
}

// height counts the levels from the folder down to the deepest folder below it: 1 for a folder with none.
func (t tree) height(id string) int {
	top := t.depth(&id)
	h := 0
	for f := range t {
		if t.within(f, id) {
			h = max(h, t.depth(&f)-top+1)
		}
	}
	return h
}

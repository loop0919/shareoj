// Package tags validates the tags of problems and posts and lists the tags in public use.
package tags

import (
	"context"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/text/unicode/norm"
)

const (
	// MaxPerItem bounds the tags of one problem or post.
	MaxPerItem = 5
	// MaxLength counts runes after normalization.
	MaxLength = 30
)

// Normalize folds width variants with NFKC and collapses runs of spaces, so 「ＤＰ」 and 「DP」 are one tag.
// The web editor applies the same rule before saving.
func Normalize(tag string) string {
	return strings.Join(strings.Fields(norm.NFKC.String(tag)), " ")
}

// Key compares tags regardless of letter case.
func Key(tag string) string { return strings.ToLower(Normalize(tag)) }

// Valid accepts normalized tags without commas or control characters, distinct regardless of case.
// The comma is reserved because the editor uses it to separate tags.
func Valid(list []string) bool {
	if len(list) > MaxPerItem {
		return false
	}
	seen := map[string]bool{}
	for _, tag := range list {
		if tag == "" || tag != Normalize(tag) || utf8.RuneCountInString(tag) > MaxLength || strings.ContainsRune(tag, ',') || strings.IndexFunc(tag, unicode.IsControl) >= 0 || seen[Key(tag)] {
			return false
		}
		seen[Key(tag)] = true
	}
	return true
}

// Tag is a tag in public use, with how many public problems and posts carry it.
type Tag struct {
	Name         string `json:"name"`
	ProblemCount int64  `json:"problemCount"`
	PostCount    int64  `json:"postCount"`
}

// Limit bounds the list the editor loads once for its suggestions.
const Limit = 1000

type Store struct{ Pool *pgxpool.Pool }

// List returns the tags of public problems and posts, most used first.
// A problem in a 定期便 round in progress does not count, since its tags are hidden.
func (s *Store) List(ctx context.Context) ([]Tag, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tag,count(*) FILTER (WHERE kind='problem'),count(*) FILTER (WHERE kind='post') FROM (
  SELECT jsonb_array_elements_text(d.published_draft->'tags') tag,'problem' kind FROM problem_drafts d
  WHERE d.published_draft IS NOT NULL AND NOT problem_tags_hidden(d.id)
  UNION ALL SELECT unnest(b.published_tags),'post' FROM blog_posts b WHERE b.published_title IS NOT NULL
 ) t GROUP BY tag ORDER BY count(*) DESC,tag LIMIT $1`, Limit)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (Tag, error) {
		var t Tag
		err := row.Scan(&t.Name, &t.ProblemCount, &t.PostCount)
		return t, err
	})
}

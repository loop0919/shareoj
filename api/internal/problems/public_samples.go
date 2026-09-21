package problems

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
)

// PublicSamples reads the published snapshot, never the editable draft.
func (s *Store) PublicSamples(ctx context.Context, id string) ([]TestCase, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, `SELECT published_draft FROM problem_drafts WHERE id=$1 AND published_draft IS NOT NULL`, id).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	var draft Draft
	if err = json.Unmarshal(data, &draft); err != nil {
		return nil, err
	}
	samples := make([]TestCase, 0)
	for _, sample := range draft.TestCases {
		if sample.IsSample {
			samples = append(samples, sample)
		}
	}
	return samples, nil
}

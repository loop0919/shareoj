package httpapi

import (
	"judge/api/internal/problems"
	"judge/api/internal/submissions"
)

func validDraft(d problems.Draft) bool { return problems.ValidDraft(d, submissions.RuntimeIDs()) }

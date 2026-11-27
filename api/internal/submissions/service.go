package submissions

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"judge/api/internal/problems"
)

var (
	ErrInvalid        = errors.New("invalid submission")
	ErrMaintenance    = errors.New("judge maintenance")
	ErrUnavailable    = errors.New("judging unavailable")
	ErrProblemStorage = errors.New("problem storage unavailable")
)

type Service struct {
	Config
	Submissions   *Store
	Problems      *problems.Store
	DispatchJudge func(context.Context) error
}
type CreateInput struct {
	ContestID  string      `json:"contestId"`
	EasyTest   bool        `json:"easyTest"`
	ProblemID  string      `json:"problemId"`
	Runtime    string      `json:"runtime"`
	Source     string      `json:"source"`
	Generation *Generation `json:"generation"`
}

func (s Service) Create(ctx context.Context, owner, id string, input CreateInput) (Submission, error) {
	if s.Submissions == nil {
		return Submission{}, ErrUnavailable
	}
	if err := s.Ready(); err != nil {
		return Submission{}, err
	}
	selected := s.storedRuntime(input.Runtime)
	if ((input.EasyTest || input.ContestID != "") && input.Generation != nil) ||
		(input.Generation != nil && !problems.JudgeCodeRuntime(selected)) ||
		(input.ContestID != "" && !problems.ValidID(input.ContestID)) || !problems.ValidID(input.ProblemID) || selected == "" ||
		!validSource(selected, input.Source) {
		return Submission{}, ErrInvalid
	}
	params := RunInput{
		Owner: owner, ID: id, ProblemID: input.ProblemID, Source: input.Source, Image: s.JudgeImage, Runtime: selected,
		EasyTest: input.EasyTest, ContestID: input.ContestID, CheckerRuntimes: s.RuntimeIDs(),
	}
	var item Submission
	var err error
	if input.Generation == nil {
		item, err = s.Submissions.CreateRun(ctx, params)
	} else {
		if s.Problems == nil {
			return Submission{}, ErrProblemStorage
		}
		if !input.Generation.valid() {
			return Submission{}, ErrInvalid
		}
		problem, getErr := s.Problems.Get(ctx, owner, input.ProblemID)
		if getErr != nil {
			return Submission{}, getErr
		}
		job, buildErr := generationJob(owner, input.ProblemID, s.JudgeImage, problem, *input.Generation)
		if buildErr != nil {
			return Submission{}, buildErr
		}
		item, err = s.Submissions.CreateGeneration(ctx, GenerationInput{RunInput: params, ProblemVersion: problem.Version, Job: job})
	}
	if err != nil {
		return Submission{}, err
	}
	// The committed outbox and periodic dispatcher recover a failed wake-up.
	s.wake(ctx, "immediate judge dispatch unavailable; periodic recovery retained", "submissionId", item.ID)
	return item, nil
}

// validSource checks a source to run with a stored runtime: not blank, at most 128 KiB, and UTF-8 without NUL.
func validSource(runtime, source string) bool {
	return problems.HasSource(runtime, source) && len(source) <= 128<<10 && utf8.ValidString(source) && !strings.ContainsRune(source, 0)
}

// wake asks the dispatcher to send now and waits at most 2 seconds. A failure is only logged, as warning with the ID
// under key, because the periodic dispatcher sends what is left.
func (s Service) wake(ctx context.Context, warning, key, id string) {
	if s.local() || s.DispatchJudge == nil {
		return
	}
	wakeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	err := s.DispatchJudge(wakeCtx)
	cancel()
	if err != nil {
		slog.Warn(warning, key, id)
	}
}

// runtimeSuffix is what a stored runtime ID adds to the ID of an available runtime.
func (s Service) runtimeSuffix() string {
	if s.local() {
		return "-local"
	}
	return "-isolate"
}

// storedRuntime maps a requested runtime ID to the runtime stored with a run, or "" when it is not available now.
func (s Service) storedRuntime(id string) string {
	suffix := s.runtimeSuffix()
	for _, available := range s.AvailableRuntimes() {
		if id == available.ID || id == available.ID+suffix {
			return available.ID + suffix
		}
	}
	return ""
}

// submissionRuntimes lists the stored runtime IDs that new submissions may use now.
func (s Service) submissionRuntimes() []string {
	suffix := s.runtimeSuffix()
	ids := []string{}
	for _, rt := range s.AvailableRuntimes() {
		ids = append(ids, rt.ID+suffix)
	}
	return ids
}

// Rejudge requeues finished submissions of a problem the actor manages, or any problem for an operator.
func (s Service) Rejudge(ctx context.Context, actor, problemID string, submissionIDs []string, operator bool) (int64, error) {
	if s.Submissions == nil {
		return 0, ErrUnavailable
	}
	if err := s.Ready(); err != nil {
		return 0, err
	}
	if !problems.ValidID(problemID) || len(submissionIDs) > MaxRejudgeTargets {
		return 0, ErrInvalid
	}
	for _, id := range submissionIDs {
		if !problems.ValidID(id) {
			return 0, ErrInvalid
		}
	}
	queued, err := s.Submissions.Rejudge(ctx, RejudgeInput{
		Actor: actor, Operator: operator, ProblemID: problemID, SubmissionIDs: submissionIDs,
		Image: s.JudgeImage, Runtimes: s.submissionRuntimes(), CheckerRuntimes: s.RuntimeIDs(),
	})
	if err == nil && queued > 0 {
		s.wake(ctx, "immediate judge dispatch unavailable for rejudge; periodic recovery retained", "problemId", problemID)
	}
	return queued, err
}

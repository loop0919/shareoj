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
	Problems      problems.Repository
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
	runtime := s.JudgeRuntime
	if runtime == "" {
		runtime = "cpp17-local"
	}
	selected := ""
	for _, available := range s.AvailableRuntimes() {
		if input.Runtime == available.ID || input.Runtime == available.ID+"-isolate" || (runtime == "cpp17-local" && input.Runtime == runtime) {
			selected = available.ID + "-isolate"
			if runtime == "cpp17-local" {
				selected = runtime
			}
			break
		}
	}
	if ((input.EasyTest || input.ContestID != "") && input.Generation != nil) ||
		(input.ContestID != "" && !problems.ValidID(input.ContestID)) || !problems.ValidID(input.ProblemID) || selected == "" ||
		strings.TrimSpace(input.Source) == "" || len(input.Source) > 64<<10 || !utf8.ValidString(input.Source) || strings.ContainsRune(input.Source, 0) {
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
	if selected != "cpp17-local" && s.DispatchJudge != nil {
		// The committed outbox and periodic dispatcher recover a failed wake-up.
		wakeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		err := s.DispatchJudge(wakeCtx)
		cancel()
		if err != nil {
			slog.Warn("immediate judge dispatch unavailable; periodic recovery retained", "submissionId", item.ID)
		}
	}
	return item, nil
}

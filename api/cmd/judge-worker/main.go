// judge-worker processes the local development queue. Do not run inside the API Lambda.
package main

import (
	"context"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5"
	"judge/api/internal/database"
	"judge/api/internal/problems"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL is required")
	}
	pool, err := database.Open(ctx, os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatal(err)
	}
	defer pool.Close()
	store := submissions.Store{Pool: pool}
	bucket := os.Getenv("TEST_DATA_BUCKET")
	var objects testfiles.FileObjects
	if bucket != "" {
		sdk, err := config.LoadDefaultConfig(ctx)
		if err != nil {
			log.Fatal("AWS configuration unavailable")
		}
		objects = s3.NewFromConfig(sdk)
	}
	if bucket == "" && os.Getenv("TEST_DATA_DIR") != "" {
		objects, err = testfiles.NewLocalObjects(os.Getenv("TEST_DATA_DIR"))
		if err != nil {
			log.Fatal("Local test file storage unavailable: ", err)
		}
		bucket = "local"
	}
	log.Print("Local Docker judge worker started")
	for ctx.Err() == nil {
		item, job, err := store.Claim(ctx)
		if err == nil {
			jobCtx, cancel := context.WithTimeout(ctx, submissions.JudgeTimeout)
			var result submissions.Result
			if err := store.ResolveTestFiles(jobCtx, &job); err != nil {
				result = submissions.Result{Verdict: "JE"}
			} else {
				job.LoadFile = func(ctx context.Context, file *problems.TestFile) (string, error) {
					if objects == nil {
						return "", testfiles.ErrUnavailable
					}
					return testfiles.ReadFile(ctx, objects, bucket, file)
				}
				job.SaveOutput = func(ctx context.Context, data []byte) (*problems.TestFile, error) {
					if objects == nil {
						return nil, testfiles.ErrUnavailable
					}
					return testfiles.WriteGenerated(ctx, objects, bucket, job.GenerationPrefix, data)
				}
				result = submissions.Judge(jobCtx, item.Source, job)
			}
			cancel()
			saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
			if err := store.Finish(saveCtx, item.ID, result); err != nil {
				log.Print("Could not persist judge result")
			}
			saveCancel()
			continue
		}
		if errors.Is(err, pgx.ErrNoRows) {
			run, claimErr := store.ClaimCodeRun(ctx)
			if claimErr == nil {
				jobCtx, cancel := context.WithTimeout(ctx, submissions.JudgeTimeout)
				result := submissions.RunCodeLocally(jobCtx, run)
				cancel()
				saveCtx, saveCancel := context.WithTimeout(context.Background(), 10*time.Second)
				if err := store.FinishCodeRun(saveCtx, run.ID, "", result); err != nil {
					log.Print("Could not persist code run result")
				}
				saveCancel()
				continue
			}
			err = claimErr
		}
		if !errors.Is(err, pgx.ErrNoRows) && ctx.Err() == nil {
			log.Print("Judge queue unavailable")
		}
		select {
		case <-ctx.Done():
		case <-time.After(time.Second):
		}
	}
}

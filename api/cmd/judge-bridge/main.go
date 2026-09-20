// judge-bridge dispatches the durable DB outbox and applies SQS results.
// The Lightsail runner has no database credentials.
package main

import (
	"context"
	"log/slog"
	"os"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/sqs"

	"judge/api/internal/database"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
	ctx := context.Background()
	db, err := database.OpenConfigured(ctx, os.Getenv)
	if err != nil {
		panic("database configuration unavailable")
	}
	sdk, err := config.LoadDefaultConfig(ctx)
	if err != nil {
		panic("AWS configuration unavailable")
	}
	b := bridge{db: db, objects: s3.NewFromConfig(sdk), queue: sqs.NewFromConfig(sdk), bucket: os.Getenv("JUDGE_JOB_BUCKET"), queueURL: os.Getenv("JUDGE_REQUEST_QUEUE_URL"), runtime: os.Getenv("JUDGE_RUNTIME_DIGEST")}
	lambda.Start(b.handle)
}

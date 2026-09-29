// judge-bridge dispatches the durable DB outbox, applies SQS results and keeps
// the EC2 judge pool sized for contests. The judge hosts have no database credentials.
package main

import (
	"context"
	"log/slog"
	"os"
	"strconv"

	"github.com/aws/aws-lambda-go/lambda"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
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
	if os.Getenv("JUDGE_CAPACITY_ENABLED") == "true" {
		minimum, err := strconv.Atoi(os.Getenv("JUDGE_BURST_MIN_PARTICIPANTS"))
		if err != nil || minimum < 1 || os.Getenv("JUDGE_POOL") == "" {
			panic("capacity configuration invalid")
		}
		b.capacity = &capacity{fleet: ec2Fleet{client: ec2.NewFromConfig(sdk), pool: os.Getenv("JUDGE_POOL")}, minParticipants: minimum}
	}
	lambda.Start(b.handle)
}

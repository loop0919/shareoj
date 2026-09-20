package httpapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/lambda"
	lambdatypes "github.com/aws/aws-sdk-go-v2/service/lambda/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"

	"judge/api/internal/contests"
	"judge/api/internal/database"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
)

// NewConfiguredHandlerは環境変数から認証設定を読み込む。
// 未設定の場合も起動できるが、認証エンドポイントは503を返す。
func NewConfiguredHandler(getenv func(string) string) (http.Handler, error) {
	id, secret := getenv("COGNITO_CLIENT_ID"), getenv("COGNITO_CLIENT_SECRET")
	if id == "" {
		if secret != "" {
			return nil, fmt.Errorf("COGNITO_CLIENT_ID is required with COGNITO_CLIENT_SECRET")
		}
		return configuredStorage(getenv, AuthConfig{}, "")
	}
	region := getenv("AWS_REGION")
	if region == "" {
		region = getenv("AWS_DEFAULT_REGION")
	}
	if region == "" {
		return nil, fmt.Errorf("AWS_REGION is required with COGNITO_CLIENT_ID")
	}
	client := cognitoidentityprovider.New(cognitoidentityprovider.Options{
		Region:           region,
		HTTPClient:       &http.Client{Timeout: 5 * time.Second},
		RetryMaxAttempts: 1,
	})
	return configuredStorage(getenv, AuthConfig{Client: client, ClientID: id, ClientSecret: secret}, region)
}

type configuredHandler struct {
	http.Handler
	pool *pgxpool.Pool
}

func (h *configuredHandler) Close() error {
	if h.pool != nil {
		h.pool.Close()
	}
	return nil
}

func configuredStorage(getenv func(string) string, auth AuthConfig, region string) (http.Handler, error) {
	private := handlerDependencies{Operators: make(map[string]bool)}
	for _, subject := range strings.Split(getenv("OPERATOR_SUBJECTS"), ",") {
		if subject = strings.TrimSpace(subject); subject != "" {
			private.Operators[subject] = true
		}
	}
	poolID := getenv("COGNITO_USER_POOL_ID")
	if auth.ClientID != "" && poolID != "" {
		if !regexp.MustCompile(`^[a-z0-9-]+_[A-Za-z0-9]+$`).MatchString(poolID) || !strings.HasPrefix(poolID, region+"_") {
			return nil, errors.New("invalid COGNITO_USER_POOL_ID")
		}
		private.Verifier = newCognitoVerifier("https://cognito-idp."+region+".amazonaws.com/"+poolID, auth.ClientID)
	}
	var pool *pgxpool.Pool
	if getenv("DATABASE_URL") != "" || getenv("DATABASE_SECRET_ARN") != "" {
		if auth.ClientID != "" && private.Verifier == nil {
			return nil, errors.New("COGNITO_USER_POOL_ID is required with DATABASE_URL and COGNITO_CLIENT_ID")
		}
		var err error
		pool, err = database.OpenConfigured(context.Background(), getenv)
		if err != nil {
			return nil, err
		}
		store := problems.New(pool)
		private.Store = store
		private.Contests = &contests.Store{Pool: pool}
		private.Images = &images.Store{Pool: pool}
		private.Notifications = &notifications.Store{Pool: pool}
		private.Profiles = profiles.New(pool)
		private.Posts = posts.New(pool)
		private.Submissions = &submissions.Store{Pool: pool}
		private.JudgeImage = getenv("JUDGE_CPP_IMAGE")
		private.JudgeRuntime = getenv("JUDGE_RUNTIME")
		private.JudgeEnabledRuntimes = getenv("JUDGE_ENABLED_RUNTIMES")
		if function := getenv("JUDGE_DISPATCH_FUNCTION"); function != "" {
			sdk, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
			if err != nil {
				pool.Close()
				return nil, errors.New("judge dispatch configuration unavailable")
			}
			client := lambda.NewFromConfig(sdk, func(o *lambda.Options) { o.RetryMaxAttempts = 1 })
			private.DispatchJudge = func(ctx context.Context) error {
				_, err := client.Invoke(ctx, &lambda.InvokeInput{
					FunctionName: aws.String(function), InvocationType: lambdatypes.InvocationTypeEvent, Payload: []byte(`{}`),
				})
				return err
			}
		}
		if bucket := getenv("TEST_DATA_BUCKET"); bucket != "" {
			sdk, err := awsconfig.LoadDefaultConfig(context.Background(), awsconfig.WithRegion(region))
			if err != nil {
				pool.Close()
				return nil, errors.New("test file storage unavailable")
			}
			private.Files = testfiles.New(pool, s3.NewFromConfig(sdk), bucket)
		}
	}
	return &configuredHandler{Handler: newHandler(auth, private), pool: pool}, nil
}

package httpapi

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider/types"
	"github.com/jackc/pgx/v5"

	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

// This identity provider exists only in the Go test binary, never in the API build.
type browserCognito struct {
	t      *testing.T
	signer signingFixture
	mu     sync.Mutex
	users  map[string]*browserUser
}

type browserUser struct {
	password  string
	confirmed bool
}

func (c *browserCognito) InitiateAuth(_ context.Context, in *cognitoidentityprovider.InitiateAuthInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.InitiateAuthOutput, error) {
	name := in.AuthParameters["USERNAME"]
	c.mu.Lock()
	defer c.mu.Unlock()
	if name == "alice@example.test" || name == "bob@example.test" || name == "test_cases@example.test" {
		if in.AuthParameters["PASSWORD"] != "test-password" {
			return nil, &types.NotAuthorizedException{}
		}
	} else {
		user := c.users[name]
		if user == nil || user.password != in.AuthParameters["PASSWORD"] {
			return nil, &types.NotAuthorizedException{}
		}
		if !user.confirmed {
			return nil, &types.UserNotConfirmedException{}
		}
	}
	return &cognitoidentityprovider.InitiateAuthOutput{AuthenticationResult: &types.AuthenticationResultType{
		AccessToken: aws.String(c.signer.token(c.t, name, nil)), IdToken: aws.String("unused-id-token"), ExpiresIn: 3600, TokenType: aws.String("Bearer"),
	}}, nil
}

func (c *browserCognito) RespondToAuthChallenge(context.Context, *cognitoidentityprovider.RespondToAuthChallengeInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.RespondToAuthChallengeOutput, error) {
	return nil, &types.NotAuthorizedException{}
}

func TestBrowserFixture(t *testing.T) {
	if os.Getenv("OPENOJ_BROWSER_TEST") != "1" {
		t.Skip("browser fixture disabled")
	}
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	schema := fmt.Sprintf("test_browser_%d", time.Now().UnixNano())
	if _, err := conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`) }()
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	profileStore := profiles.New(store.Pool())
	for _, name := range []string{"alice", "bob", "test_cases"} {
		if _, err := profileStore.Save(ctx, name+"@example.test", name, "", 0, profiles.Accounts{}); err != nil {
			t.Fatal(err)
		}
	}
	f := newSigningFixture(t)
	handler := newHandler(AuthConfig{Client: &browserCognito{t: t, signer: f, users: make(map[string]*browserUser)}, ClientID: "client"}, handlerDependencies{Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profileStore, Posts: posts.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Operators: map[string]bool{"alice@example.test": true}, Verifier: newCognitoVerifier(f.server.URL, "client")})
	address := os.Getenv("OPENOJ_BROWSER_API_ADDR")
	if address == "" {
		address = "127.0.0.1:18082"
	}
	server := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second}
	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("browser API ready")
	stopCtx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case err := <-done:
		t.Fatal(err)
	case <-stopCtx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		t.Error(err)
	}
}

func (c *browserCognito) SignUp(_ context.Context, in *cognitoidentityprovider.SignUpInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.SignUpOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	name := aws.ToString(in.Username)
	if c.users[name] != nil {
		return nil, &types.UsernameExistsException{}
	}
	c.users[name] = &browserUser{password: aws.ToString(in.Password)}
	return &cognitoidentityprovider.SignUpOutput{UserConfirmed: false}, nil
}

func (c *browserCognito) ConfirmSignUp(_ context.Context, in *cognitoidentityprovider.ConfirmSignUpInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ConfirmSignUpOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	user := c.users[aws.ToString(in.Username)]
	if user == nil {
		return nil, &types.UserNotFoundException{}
	}
	if aws.ToString(in.ConfirmationCode) != "123456" {
		return nil, &types.CodeMismatchException{}
	}
	user.confirmed = true
	return &cognitoidentityprovider.ConfirmSignUpOutput{}, nil
}

func (c *browserCognito) ResendConfirmationCode(_ context.Context, in *cognitoidentityprovider.ResendConfirmationCodeInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ResendConfirmationCodeOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	user := c.users[aws.ToString(in.Username)]
	if user == nil {
		return nil, &types.UserNotFoundException{}
	}
	if user.confirmed {
		return nil, &types.NotAuthorizedException{}
	}
	return &cognitoidentityprovider.ResendConfirmationCodeOutput{}, nil
}

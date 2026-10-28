package httpapi

import (
	"context"
	"encoding/json"
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

	"judge/api/internal/accounts"
	"judge/api/internal/contests"
	"judge/api/internal/images"
	"judge/api/internal/notifications"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
	"judge/api/internal/testfiles"
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
	user := c.users[name]
	if user == nil || user.password != in.AuthParameters["PASSWORD"] {
		return nil, &types.NotAuthorizedException{}
	}
	if !user.confirmed {
		return nil, &types.UserNotConfirmedException{}
	}
	return &cognitoidentityprovider.InitiateAuthOutput{AuthenticationResult: &types.AuthenticationResultType{
		AccessToken: aws.String(c.signer.token(c.t, name, map[string]any{"username": name})), IdToken: aws.String("unused-id-token"), ExpiresIn: 3600, TokenType: aws.String("Bearer"),
	}}, nil
}

func (c *browserCognito) AdminGetUser(_ context.Context, in *cognitoidentityprovider.AdminGetUserInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminGetUserOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.users[aws.ToString(in.Username)] == nil {
		return nil, &types.UserNotFoundException{}
	}
	return &cognitoidentityprovider.AdminGetUserOutput{Username: in.Username, UserAttributes: []types.AttributeType{{Name: aws.String("email"), Value: in.Username}}}, nil
}

func (c *browserCognito) AdminDeleteUser(_ context.Context, in *cognitoidentityprovider.AdminDeleteUserInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminDeleteUserOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.users, aws.ToString(in.Username))
	return &cognitoidentityprovider.AdminDeleteUserOutput{}, nil
}

func (c *browserCognito) ChangePassword(_ context.Context, in *cognitoidentityprovider.ChangePasswordInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ChangePasswordOutput, error) {
	_, name := bearer(&http.Request{Header: http.Header{"Authorization": {"Bearer " + aws.ToString(in.AccessToken)}}})
	c.mu.Lock()
	defer c.mu.Unlock()
	user := c.users[name]
	if user == nil || user.password != aws.ToString(in.PreviousPassword) {
		return nil, &types.NotAuthorizedException{}
	}
	if len(aws.ToString(in.ProposedPassword)) < 12 {
		return nil, &types.InvalidPasswordException{}
	}
	user.password = aws.ToString(in.ProposedPassword)
	return &cognitoidentityprovider.ChangePasswordOutput{}, nil
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
	f := newSigningFixture(t)
	cognito := &browserCognito{t: t, signer: f, users: make(map[string]*browserUser)}
	// leaver exists so the account deletion test cannot take a shared user with it; carol and dave join contests
	// as further co-setters and participants, and erin and frank as members of other teams.
	for _, name := range []string{"alice", "bob", "carol", "dave", "erin", "frank", "test_cases", "shared_submission", "leaver"} {
		if _, err := profileStore.Save(ctx, name+"@example.test", name, "", 0, profiles.Accounts{}); err != nil {
			t.Fatal(err)
		}
		cognito.users[name+"@example.test"] = &browserUser{password: "test-password", confirmed: true}
	}
	dependencies := handlerDependencies{Accounts: &accounts.Store{Pool: store.Pool()}, CognitoUsers: cognito, UserPoolID: "local_browser", Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profileStore, Posts: posts.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", Operators: map[string]bool{"alice@example.test": true}, Verifier: newCognitoVerifier(f.server.URL, "client")}
	address := os.Getenv("OPENOJ_BROWSER_API_ADDR")
	if address == "" {
		address = "127.0.0.1:18082"
	}
	if image := os.Getenv("BROWSER_JUDGE_IMAGE"); image != "" {
		dependencies.JudgeImage = image
		dir := t.TempDir()
		dependencies.Files, dependencies.LocalFiles, err = testfiles.NewLocal(store.Pool(), dir, "http://"+address+"/local-test-files", "http://127.0.0.1:13002")
		if err != nil {
			t.Fatal(err)
		}
		objects, err := testfiles.NewLocalObjects(dir)
		if err != nil {
			t.Fatal(err)
		}
		workerCtx, cancel := context.WithCancel(ctx)
		finished := make(chan struct{})
		defer func() { cancel(); <-finished }()
		go func() {
			defer close(finished)
			for workerCtx.Err() == nil {
				item, job, err := dependencies.Submissions.Claim(workerCtx)
				if err == nil {
					if err = dependencies.Submissions.ResolveTestFiles(workerCtx, &job); err != nil {
						t.Error(err)
						return
					}
					job.LoadFile = func(ctx context.Context, file *problems.TestFile) (string, error) {
						return testfiles.ReadFile(ctx, objects, "local", file)
					}
					job.SaveOutput = func(ctx context.Context, data []byte) (*problems.TestFile, error) {
						return testfiles.WriteGenerated(ctx, objects, "local", job.GenerationPrefix, data)
					}
					result := submissions.Judge(workerCtx, item.Source, job)
					if err = dependencies.Submissions.Finish(workerCtx, item.ID, result); err != nil && workerCtx.Err() == nil {
						t.Error(err)
						return
					}
				} else if err != pgx.ErrNoRows && workerCtx.Err() == nil {
					t.Error(err)
					return
				}
				select {
				case <-workerCtx.Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
		}()
	}
	handler := http.NewServeMux()
	handler.Handle("/", newHandler(AuthConfig{Client: cognito, ClientID: "client"}, dependencies))
	// Browser tests move a contest's schedule instead of waiting for it. The API decides a contest's
	// state from these times, so a moved contest goes through the same code as one that waited.
	handler.HandleFunc("POST /__test/contests/{id}/schedule", func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			StartsIn *float64 `json:"startsIn"`
			EndsIn   *float64 `json:"endsIn"`
		}
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		tag, err := store.Pool().Exec(r.Context(), `UPDATE contests SET
 starts_at=COALESCE(clock_timestamp()+make_interval(secs=>$2::float8),starts_at),
 ends_at=COALESCE(clock_timestamp()+make_interval(secs=>$3::float8),ends_at) WHERE id=$1`, r.PathValue("id"), in.StartsIn, in.EndsIn)
		if err != nil || tag.RowsAffected() != 1 {
			http.Error(w, fmt.Sprint("contest not moved: ", err), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	})
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

package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
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
)

type accountCognito struct {
	deleted    []string
	failDelete bool
	identities string
	password   string
}

func (c *accountCognito) AdminGetUser(_ context.Context, in *cognitoidentityprovider.AdminGetUserInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminGetUserOutput, error) {
	attributes := []types.AttributeType{{Name: aws.String("email"), Value: aws.String(aws.ToString(in.Username) + "@example.test")}}
	if c.identities != "" {
		attributes = append(attributes, types.AttributeType{Name: aws.String("identities"), Value: aws.String(c.identities)})
	}
	return &cognitoidentityprovider.AdminGetUserOutput{UserAttributes: attributes}, nil
}

func (c *accountCognito) AdminDeleteUser(_ context.Context, in *cognitoidentityprovider.AdminDeleteUserInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminDeleteUserOutput, error) {
	if c.failDelete {
		return nil, errors.New("cognito unavailable")
	}
	c.deleted = append(c.deleted, aws.ToString(in.Username))
	return &cognitoidentityprovider.AdminDeleteUserOutput{}, nil
}

func (c *accountCognito) ChangePassword(_ context.Context, in *cognitoidentityprovider.ChangePasswordInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ChangePasswordOutput, error) {
	if aws.ToString(in.PreviousPassword) != c.password {
		return nil, &types.NotAuthorizedException{}
	}
	if len(aws.ToString(in.ProposedPassword)) < 12 {
		return nil, &types.InvalidPasswordException{}
	}
	c.password = aws.ToString(in.ProposedPassword)
	return &cognitoidentityprovider.ChangePasswordOutput{}, nil
}

func TestAccountPostgres(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	schema := fmt.Sprintf("test_accounts_%d", time.Now().UnixNano())
	if _, err = conn.Exec(ctx, `CREATE SCHEMA `+schema); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := conn.Exec(ctx, `DROP SCHEMA `+schema+` CASCADE`); err != nil {
			t.Error(err)
		}
	}()
	u, _ := url.Parse(dsn)
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	store, err := problems.Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err = store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	exec := func(query string, args ...any) {
		t.Helper()
		if _, e := store.Pool().Exec(ctx, query, args...); e != nil {
			t.Fatal(e)
		}
	}
	count := func(query string, args ...any) int {
		t.Helper()
		var n int
		if e := store.Pool().QueryRow(ctx, query, args...).Scan(&n); e != nil {
			t.Fatal(e)
		}
		return n
	}
	exec(`INSERT INTO user_profiles(owner_id,handle,avatar,accounts) VALUES ('alice-sub','alice','data:image/png;base64,AAAA','{"atcoder":"alice"}'),('bob-sub','bob','','{}')`)
	const draft = "11111111-1111-4111-8111-111111111111"
	const public = "22222222-2222-4222-8222-222222222222"
	const bobs = "33333333-3333-4333-8333-333333333333"
	const contestID = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const keptImage = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
	const privateImage = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
	for id, owner := range map[string]string{draft: "alice-sub", public: "alice-sub", bobs: "bob-sub"} {
		if _, err = store.Save(ctx, owner, id, 0, problems.Draft{Title: "Problem " + id[:1], Markdown: "statement"}); err != nil {
			t.Fatal(err)
		}
	}
	exec(`UPDATE problem_drafts SET published_draft=jsonb_set(draft,'{markdown}',to_jsonb('![](/api/images/`+keptImage+`)'::text)),published_version=1,published_at=now() WHERE id=$1`, public)
	exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{markdown}','"unpublished edit ![](/api/images/`+privateImage+`)"') WHERE id=$1`, public)
	exec(`INSERT INTO content_images(id,owner_id,digest,media_type,data,size) SELECT id,'alice-sub',decode(md5(id::text),'hex'),'image/png','\x00'::bytea,1 FROM unnest($1::uuid[]) id`, []string{keptImage, privateImage})
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'alice-sub'),($2,'bob-sub')`, bobs, draft)
	exec(`INSERT INTO problem_favorites(problem_id,owner_id) VALUES($1,'alice-sub'),($2,'bob-sub')`, bobs, public)
	exec(`INSERT INTO blog_posts(id,owner_id,title,markdown) VALUES ('44444444-4444-4444-8444-444444444444','alice-sub','Draft post','secret')`)
	exec(`INSERT INTO blog_posts(id,owner_id,title,markdown,published_title,published_markdown,published_version,published_at) VALUES ('55555555-5555-4555-8555-555555555555','alice-sub','Edited','edited','Public post','public',1,now())`)
	exec(`INSERT INTO contest_drafts(id,owner_id,draft) VALUES ('66666666-6666-4666-8666-666666666666','alice-sub','{}')`)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at) VALUES ($1,'alice-sub','Upcoming','',now()+interval '1 hour',now()+interval '2 hours')`, contestID)

	cognito := &accountCognito{password: "old-password-1"}
	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Accounts: &accounts.Store{Pool: store.Pool()}, CognitoUsers: cognito, UserPoolID: "local_pool", Store: store, Contests: &contests.Store{Pool: store.Pool()}, Images: &images.Store{Pool: store.Pool()}, Notifications: &notifications.Store{Pool: store.Pool()}, Profiles: profiles.New(store.Pool()), Posts: posts.New(store.Pool()), Submissions: &submissions.Store{Pool: store.Pool()}, JudgeImage: "sha256:" + strings.Repeat("a", 64), JudgeRuntime: "cpp17-isolate", JudgeEnabledRuntimes: "cpp17", Verifier: newCognitoVerifier(f.server.URL, "client")})
	request := func(method, path, owner, username string, body any, want int) string {
		t.Helper()
		raw := ""
		if body != nil {
			data, e := json.Marshal(body)
			if e != nil {
				t.Fatal(e)
			}
			raw = string(data)
		}
		r := httptest.NewRequest(method, path, strings.NewReader(raw))
		r.Header.Set("Content-Type", "application/json")
		if owner != "" {
			r.Header.Set("Authorization", "Bearer "+f.token(t, owner, map[string]any{"username": username}))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
		}
		return w.Body.String()
	}

	// The sign-in method comes from Cognito: Google users carry an identities attribute.
	if body := request("GET", "/my/account", "alice-sub", "alice-sub", nil, 200); !strings.Contains(body, `"email":"alice-sub@example.test"`) || !strings.Contains(body, `"provider":"password"`) {
		t.Fatal(body)
	}
	cognito.identities = `[{"providerName":"Google","userId":"1"}]`
	if body := request("GET", "/my/account", "alice-sub", "google_1", nil, 200); !strings.Contains(body, `"provider":"google"`) {
		t.Fatal(body)
	}
	cognito.identities = ""

	// Only password users change passwords; Cognito checks the current one and the policy.
	request("POST", "/my/password", "alice-sub", "google_1", map[string]string{"current": "x", "proposed": "new-password-12"}, 409)
	request("POST", "/my/password", "alice-sub", "alice-sub", map[string]string{"current": "wrong", "proposed": "new-password-12"}, 400)
	request("POST", "/my/password", "alice-sub", "alice-sub", map[string]string{"current": "old-password-1", "proposed": "short"}, 400)
	request("POST", "/my/password", "alice-sub", "alice-sub", map[string]string{"current": "old-password-1", "proposed": "new-password-12"}, 204)
	request("POST", "/my/password", "alice-sub", "alice-sub", map[string]string{"current": "", "proposed": "new-password-12"}, 400)
	if cognito.password != "new-password-12" {
		t.Fatal("password not changed")
	}

	// Nobody can pick the placeholder that deleted accounts get.
	request("PUT", "/my/profile", "bob-sub", "bob-sub", map[string]any{"handle": "deleted_0123456789a", "avatar": "", "accounts": map[string]string{}, "version": 1}, 400)

	// The typed handle must match, and a contest that has not ended keeps its organizer.
	request("DELETE", "/my/account", "alice-sub", "alice-sub", map[string]string{"handle": "bob"}, 400)
	if body := request("DELETE", "/my/account", "alice-sub", "alice-sub", map[string]string{"handle": "alice"}, 409); !strings.Contains(body, "account_has_active_contest") {
		t.Fatal(body)
	}
	exec(`UPDATE contests SET starts_at=now()-interval '2 hours',ends_at=now()-interval '1 hour',released=true WHERE id=$1`, contestID)

	// A Cognito failure rolls everything back.
	cognito.failDelete = true
	request("DELETE", "/my/account", "alice-sub", "alice-sub", map[string]string{"handle": "alice"}, 502)
	if count(`SELECT count(*) FROM problem_drafts WHERE id=$1`, draft) != 1 || count(`SELECT count(*) FROM user_profiles WHERE handle='alice'`) != 1 {
		t.Fatal("failed deletion changed data")
	}
	cognito.failDelete = false
	request("DELETE", "/my/account", "alice-sub", "alice-sub", map[string]string{"handle": "alice"}, 204)
	if len(cognito.deleted) != 1 || cognito.deleted[0] != "alice-sub" {
		t.Fatal("cognito user not deleted", cognito.deleted)
	}

	// Private data is gone.
	for query, want := range map[string]int{
		`SELECT count(*) FROM problem_drafts WHERE id='` + draft + `'`:                                       0,
		`SELECT count(*) FROM blog_posts WHERE owner_id='alice-sub' AND published_title IS NULL`:             0,
		`SELECT count(*) FROM contest_drafts WHERE owner_id='alice-sub'`:                                     0,
		`SELECT count(*) FROM problem_favorites WHERE owner_id='alice-sub'`:                                  0,
		`SELECT count(*) FROM content_images WHERE id='` + privateImage + `'`:                                0,
		`SELECT count(*) FROM problem_drafts WHERE id='` + public + `' AND draft::text LIKE '%unpublished%'`: 0,
		`SELECT count(*) FROM blog_posts WHERE owner_id='alice-sub' AND title='Public post'`:                 1,
		`SELECT count(*) FROM content_images WHERE id='` + keptImage + `'`:                                   1,
		`SELECT count(*) FROM problem_favorites WHERE owner_id='bob-sub' AND problem_id='` + public + `'`:    1,
		`SELECT count(*) FROM problem_testers WHERE owner_id='alice-sub'`:                                    1,
		`SELECT count(*) FROM contests WHERE id='` + contestID + `'`:                                         1,
		`SELECT count(*) FROM user_profiles WHERE owner_id='alice-sub' AND avatar='' AND accounts='{}'`:      1,
	} {
		if got := count(query); got != want {
			t.Fatalf("%s: got %d want %d", query, got, want)
		}
	}
	// Public work stays under a placeholder, the old handle is free, and leftover tokens stop working.
	var problem problems.PublicProblem
	if err := json.Unmarshal([]byte(request("GET", "/problems/"+public, "", "", nil, 200)), &problem); err != nil {
		t.Fatal(err)
	}
	if !profiles.DeletedHandle.MatchString(problem.Author) {
		t.Fatal("author not anonymized", problem.Author)
	}
	request("GET", "/users/alice", "", "", nil, 404)
	request("GET", "/users/"+problem.Author, "", "", nil, 404)
	request("GET", "/my/profile", "alice-sub", "alice-sub", nil, 401)
	request("GET", "/my/problems", "alice-sub", "alice-sub", nil, 401)
	request("DELETE", "/my/account", "alice-sub", "alice-sub", map[string]string{"handle": problem.Author}, 401)
	request("PUT", "/my/profile", "bob-sub", "bob-sub", map[string]any{"handle": "alice", "avatar": "", "accounts": map[string]string{}, "version": 1}, 200)
}

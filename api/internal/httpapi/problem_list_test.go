package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/tags"
)

func TestProblemListConditionsPostgres(t *testing.T) {
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
	schema := fmt.Sprintf("test_problem_list_%d", time.Now().UnixNano())
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
	exec(`INSERT INTO user_profiles(owner_id,handle) VALUES ('alice','alice'),('bob','bob'),('carol','carol')`)

	f := newSigningFixture(t)
	h := newHandler(AuthConfig{}, handlerDependencies{Store: store, Posts: posts.New(store.Pool()), Profiles: profiles.New(store.Pool()), Tags: &tags.Store{Pool: store.Pool()}, Verifier: newCognitoVerifier(f.server.URL, "client")})
	// getAs signs in as actor; "" sends no session.
	getAs := func(path, actor string, want int) string {
		t.Helper()
		r := httptest.NewRequest("GET", path, nil)
		if actor != "" {
			r.Header.Set("Authorization", "Bearer "+f.token(t, actor, nil))
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != want {
			t.Fatalf("GET %s: got %d want %d: %s", path, w.Code, want, w.Body.String())
		}
		return w.Body.String()
	}
	get := func(path string, want int) string {
		t.Helper()
		return getAs(path, "", want)
	}
	type page struct {
		Items      []struct{ ID string }
		NextCursor string
		Total      int
	}
	fetchAs := func(path, actor string) page {
		t.Helper()
		var p page
		if err := json.Unmarshal([]byte(getAs(path, actor, 200)), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	fetch := func(path string) page {
		t.Helper()
		return fetchAs(path, "")
	}
	idsOf := func(p page) []string {
		ids := []string{}
		for _, item := range p.Items {
			ids = append(ids, item.ID)
		}
		return ids
	}
	listed := func(path string) []string {
		t.Helper()
		return idsOf(fetch(path))
	}

	const (
		a = "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa"
		b = "bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb"
		c = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		d = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		e = "eeeeeeee-eeee-4eee-8eee-eeeeeeeeeeee"
	)
	level := func(n int) *int { return &n }
	for _, p := range []struct {
		id         string
		days       int
		difficulty *int
	}{{a, 5, level(3)}, {b, 4, nil}, {c, 3, level(7)}, {d, 2, level(3)}, {e, 1, level(10)}} {
		draft := problems.Draft{Difficulty: p.difficulty, Title: "Problem " + p.id[:1], Markdown: "statement", Editorial: "official", TimeLimitMS: "1000", MemoryLimitMB: "256", TestCases: []problems.TestCase{{Input: "1", Output: "2", IsSample: true}}}
		if _, err = store.Save(ctx, "alice", p.id, 0, draft); err != nil {
			t.Fatal(err)
		}
		exec(`UPDATE problem_drafts SET published_draft=draft,published_version=version,published_at=now()-make_interval(days=>$2) WHERE id=$1`, p.id, p.days)
	}
	for _, s := range []struct{ owner, problem string }{{"bob", a}, {"bob", c}, {"carol", a}, {"carol", c}, {"carol", d}} {
		exec(`INSERT INTO submissions (id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
			VALUES (gen_random_uuid(),$1,$2,1,'test','cpp17','','{"privateDraft":false}','DONE','{"verdict":"AC"}')`, s.owner, s.problem)
	}
	for _, fav := range []struct{ owner, problem string }{{"bob", b}, {"carol", b}, {"bob", c}} {
		exec(`INSERT INTO problem_favorites(problem_id,owner_id) VALUES ($1,$2)`, fav.problem, fav.owner)
	}

	for _, tc := range []struct {
		query string
		want  []string
	}{
		{"", []string{e, d, c, b, a}},
		{"sort=", []string{e, d, c, b, a}},
		{"sort=newest", []string{e, d, c, b, a}},
		{"sort=oldest", []string{a, b, c, d, e}},
		{"sort=solvers-desc", []string{c, a, d, e, b}},
		{"sort=solvers-asc", []string{e, b, d, c, a}},
		{"sort=favorites-desc", []string{b, c, e, d, a}},
		{"sort=difficulty-desc", []string{e, c, d, a, b}},
		{"sort=difficulty-asc", []string{d, a, c, e, b}},
		{"difficulty=3-7", []string{d, c, a}},
		{"difficulty=1-10", []string{e, d, c, a}},
		{"difficulty=10-10&sort=oldest", []string{e}},
		{"difficulty=3-7&contest=exclude&sort=solvers-desc", []string{c, a, d}},
		// The public route does not read solved.
		{"solved=unsolved", []string{e, d, c, b, a}},
	} {
		p := fetch("/problems?" + tc.query)
		got := []string{}
		for _, item := range p.Items {
			got = append(got, item.ID)
		}
		if !slices.Equal(got, tc.want) || p.Total != len(tc.want) {
			t.Errorf("%q: got %v total %d, want %v", tc.query, got, p.Total, tc.want)
		}
	}

	for _, bad := range []string{"sort=popular", "difficulty=0-3", "difficulty=6-3", "difficulty=3", "difficulty=3-11", "difficulty=a-b"} {
		body := get("/problems?"+bad, 400)
		want := "invalid_difficulty"
		if strings.HasPrefix(bad, "sort=") {
			want = "invalid_sort"
		}
		if !strings.Contains(body, want) {
			t.Errorf("%q: got %s, want %s", bad, body, want)
		}
	}

	// Offset pages of a non-initial order list every problem exactly once.
	var all []string
	for _, offset := range []int{0, 2, 4} {
		all = append(all, listed(fmt.Sprintf("/problems?limit=2&sort=solvers-desc&offset=%d", offset))...)
	}
	if !slices.Equal(all, []string{c, a, d, e, b}) {
		t.Errorf("offset pages: %v", all)
	}

	// A cursor continues only the initial order.
	first := fetch("/problems?limit=2")
	if first.NextCursor == "" {
		t.Fatal("missing nextCursor")
	}
	if got := listed("/problems?limit=2&cursor=" + first.NextCursor); !slices.Equal(got, []string{c, b}) {
		t.Errorf("cursor page: %v", got)
	}
	if body := get("/problems?sort=oldest&cursor="+first.NextCursor, 400); !strings.Contains(body, "invalid_request") {
		t.Errorf("cursor with sort: %s", body)
	}

	// The signed-in route adds the viewer's solved state to the same list.
	for _, tc := range []struct {
		query, actor string
		want         []string
	}{
		{"", "bob", []string{e, d, c, b, a}},
		{"?solved=", "bob", []string{e, d, c, b, a}},
		{"?solved=unsolved", "bob", []string{e, d, b}},
		{"?solved=solved", "bob", []string{c, a}},
		{"?solved=solved&sort=oldest&difficulty=3-7", "bob", []string{a, c}},
		{"?solved=solved", "alice", []string{}},
	} {
		p := fetchAs("/my/public-problems"+tc.query, tc.actor)
		if got := idsOf(p); !slices.Equal(got, tc.want) || p.Total != len(tc.want) {
			t.Errorf("%q as %s: got %v total %d, want %v", tc.query, tc.actor, got, p.Total, tc.want)
		}
	}
	// The solved filter agrees with the rows the list paints green.
	var marks struct{ Items []string }
	if err := json.Unmarshal([]byte(getAs("/my/solved-problems", "bob", 200)), &marks); err != nil {
		t.Fatal(err)
	}
	filtered := idsOf(fetchAs("/my/public-problems?solved=solved", "bob"))
	slices.Sort(marks.Items)
	slices.Sort(filtered)
	if !slices.Equal(marks.Items, filtered) {
		t.Errorf("solved filter %v differs from /my/solved-problems %v", filtered, marks.Items)
	}
	// A run that /my/solved-problems does not count is not solved here either.
	exec(`INSERT INTO submissions (id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
		VALUES (gen_random_uuid(),'alice',$1,1,'test','cpp17','','{"privateDraft":true}','DONE','{"verdict":"AC"}')`, e)
	if got := idsOf(fetchAs("/my/public-problems?solved=solved", "alice")); len(got) != 0 {
		t.Errorf("private draft run counted as solved: %v", got)
	}
	if body := getAs("/my/public-problems?solved=maybe", "bob", 400); !strings.Contains(body, "invalid_solved") {
		t.Errorf("solved=maybe: %s", body)
	}
	getAs("/my/public-problems", "", 401)
}

// TestProblemListSpeedPostgres times every sort of the public list far above production size. It only runs with
// OPENOJ_LIST_SPEED=1 so that the ordinary go test and CI stay fast.
func TestProblemListSpeedPostgres(t *testing.T) {
	if os.Getenv("OPENOJ_LIST_SPEED") != "1" {
		t.Skip("OPENOJ_LIST_SPEED=1 required")
	}
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
	schema := fmt.Sprintf("test_problem_speed_%d", time.Now().UnixNano())
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
	exec := func(query string) {
		t.Helper()
		if _, e := store.Pool().Exec(ctx, query); e != nil {
			t.Fatal(e)
		}
	}
	// 200 users, 1000 published problems (difficulty 1-10, every 11th unset), 30000 accepted runs and about 5000
	// favorites. Owners and problems are drawn from independent hashes so that counts differ between problems.
	exec(`INSERT INTO user_profiles(owner_id,handle) SELECT 'u'||i,'user'||i FROM generate_series(1,200) i`)
	exec(`INSERT INTO problem_drafts(id,owner_id,draft,version,published_draft,published_version,published_at)
		SELECT gen_random_uuid(),'u1',d,1,d,1,now()-make_interval(mins=>i)
		FROM generate_series(1,1000) i,
		LATERAL (SELECT jsonb_build_object('title','Problem '||i,'timeLimitMs','1000','memoryLimitMb','256')||CASE WHEN i%11=0 THEN '{}'::jsonb ELSE jsonb_build_object('difficulty',i%10+1) END AS d) x`)
	exec(`CREATE TABLE speed_problems AS SELECT id,row_number() OVER (ORDER BY id) AS n FROM problem_drafts`)
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
		SELECT gen_random_uuid(),'u'||((hashtext('o'||i)&2147483647)%200+1),p.id,1,'t','cpp17','','{"privateDraft":false}','DONE','{"verdict":"AC"}'
		FROM generate_series(1,30000) i JOIN speed_problems p ON p.n=(hashtext('p'||i)&2147483647)%1000+1`)
	// The viewer u1 has solved 300 problems, so the unsolved filter removes a real share of the list.
	exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
		SELECT gen_random_uuid(),'u1',p.id,1,'t','cpp17','','{"privateDraft":false}','DONE','{"verdict":"AC"}'
		FROM speed_problems p WHERE p.n<=300`)
	exec(`INSERT INTO problem_favorites(problem_id,owner_id)
		SELECT p.id,'u'||k FROM speed_problems p, generate_series(1,200) k WHERE (hashtext(p.id::text||'f'||k)&2147483647)%40=0`)
	exec(`ANALYZE`)
	// Guard against a seed that degenerates into tied sort keys.
	var solverKinds, favoriteKinds, favoriteRows int
	if err = store.Pool().QueryRow(ctx, `SELECT
		(SELECT count(DISTINCT c) FROM (SELECT count(DISTINCT owner_id) c FROM submissions GROUP BY problem_id) x),
		(SELECT count(DISTINCT c) FROM (SELECT count(*) c FROM problem_favorites GROUP BY problem_id) y),
		(SELECT count(*) FROM problem_favorites)`).Scan(&solverKinds, &favoriteKinds, &favoriteRows); err != nil {
		t.Fatal(err)
	}
	if solverKinds < 10 || favoriteKinds < 5 || favoriteRows < 4000 || favoriteRows > 6000 {
		t.Fatalf("seed too uniform: %d distinct solver counts, %d distinct favorite counts, %d favorites", solverKinds, favoriteKinds, favoriteRows)
	}
	t.Logf("seed: %d distinct solver counts, %d distinct favorite counts, %d favorites", solverKinds, favoriteKinds, favoriteRows)

	sorts := []string{problems.SortNewest, problems.SortOldest, problems.SortSolversDesc, problems.SortSolversAsc, problems.SortFavoritesDesc, problems.SortDifficultyDesc, problems.SortDifficultyAsc}
	for _, solved := range []string{"", problems.UnsolvedOnly} {
		for _, order := range sorts {
			filter := problems.ListFilter{Sort: order, Solved: solved}
			if solved != "" {
				filter.Viewer = "u1"
			}
			best := time.Duration(1<<62 - 1)
			for range 3 {
				start := time.Now()
				items, e := store.PublicList(ctx, nil, 0, 50, filter)
				if e != nil {
					t.Fatal(e)
				}
				total, e := store.PublicCount(ctx, filter)
				if e != nil {
					t.Fatal(e)
				}
				if elapsed := time.Since(start); elapsed < best {
					best = elapsed
				}
				if len(items) < 50 || total == 0 {
					t.Fatalf("sort %s solved %q: got %d items, total %d", order, solved, len(items), total)
				}
			}
			t.Logf("sort=%s solved=%q best=%dms", order, solved, best.Milliseconds())
			if best > 200*time.Millisecond {
				t.Errorf("sort=%s solved=%q took %v, want at most 200ms", order, solved, best)
			}
		}
	}
}

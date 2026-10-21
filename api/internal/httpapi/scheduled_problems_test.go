package httpapi

import (
	"context"
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"judge/api/internal/problems"
)

func scheduledProblem(t *testing.T, raw string) problems.Problem {
	t.Helper()
	var p problems.Problem
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestScheduledProblemPrivacyAndSavedEditsPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	path := "/my/problems/" + p.ID
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	p = scheduledProblem(t, f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 200))
	if p.ScheduledAt == nil || !p.ScheduledAt.Equal(at) || p.PublishedVersion != 0 || p.EverPublished {
		t.Fatal(p)
	}
	f.request("GET", "/problems/"+p.ID, "", nil, 404)
	f.request("GET", "/problems/"+p.ID+"/samples", "", nil, 404)
	f.request("POST", "/my/submissions", "bob", map[string]any{"problemId": p.ID, "runtime": "cpp17", "source": "int main() {}"}, 409)
	if strings.Contains(f.request("GET", "/problems", "", nil, 200), p.ID) {
		t.Fatal("reservation leaked into public list")
	}
	if !strings.Contains(f.request("GET", "/my/problems", "alice", nil, 200), "scheduledAt") {
		t.Fatal("reservation missing from author list")
	}
	// Saved edits update the reservation without moving its publication time.
	p.Draft.Markdown = "Updated statement"
	p.Draft.Editorial = "UPDATED EDITORIAL"
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": p.Draft}, 200))
	if p.ScheduledAt == nil || !p.ScheduledAt.Equal(at) || p.PublishedVersion != 0 {
		t.Fatal(p)
	}
	f.request("GET", "/problems/"+p.ID, "", nil, 404)
	due := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	f.exec(`UPDATE problem_drafts SET scheduled_at=$2 WHERE id=$1`, p.ID, due)
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			if err := f.store.ReleaseScheduled(context.Background()); err != nil {
				t.Error(err)
			}
		})
	}
	wg.Wait()
	var public problems.PublicProblem
	if err := json.Unmarshal([]byte(f.request("GET", "/problems/"+p.ID, "", nil, 200)), &public); err != nil {
		t.Fatal(err)
	}
	if public.Markdown != p.Draft.Markdown || public.Editorial != p.Draft.Editorial || !public.PublishedAt.Equal(due) {
		t.Fatal(public)
	}
	after := scheduledProblem(t, f.request("GET", path, "alice", nil, 200))
	if after.ScheduledAt != nil || after.Version != p.Version+1 || after.PublishedVersion != p.Version || !after.EverPublished || after.Draft.Markdown != p.Draft.Markdown {
		t.Fatal(after)
	}
}

func TestScheduledProblemRejectedEditsKeepReservationPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	path := "/my/problems/" + p.ID
	p = scheduledProblem(t, f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": time.Now().Add(time.Hour)}, 200))
	for _, field := range []string{"markdown", "editorial", "testCases"} {
		d := p.Draft
		switch field {
		case "markdown":
			d.Markdown = ""
		case "editorial":
			d.Editorial = ""
		case "testCases":
			d.TestCases = nil
		}
		body := f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": d}, 400)
		if !strings.Contains(body, "incomplete_problem") {
			t.Fatal(body)
		}
	}
	d := p.Draft
	d.Editorial = "REJECTED EDITORIAL"
	f.request("PUT", path, "alice", map[string]any{"version": p.Version - 1, "draft": d}, 409)
	f.request("PUT", path, "bob", map[string]any{"version": p.Version, "draft": d}, 404)
	after := scheduledProblem(t, f.request("GET", path, "alice", nil, 200))
	if after.Version != p.Version || !reflect.DeepEqual(after.Draft, p.Draft) || after.ScheduledAt == nil || !after.ScheduledAt.Equal(*p.ScheduledAt) {
		t.Fatal(after)
	}
	f.exec(`UPDATE problem_drafts SET scheduled_at=clock_timestamp()-interval '1 second' WHERE id=$1`, p.ID)
	var public problems.PublicProblem
	if err := json.Unmarshal([]byte(f.request("GET", "/problems/"+p.ID, "", nil, 200)), &public); err != nil {
		t.Fatal(err)
	}
	if public.Markdown != p.Draft.Markdown || public.Editorial != p.Draft.Editorial {
		t.Fatal(public)
	}
}

func TestScheduledProblemValidationAndCancellationPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	path := "/my/problems/" + p.ID + "/publication"
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, p.ID)
	for _, actor := range []string{"bob", "tester"} {
		f.request("PUT", path, actor, map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 404)
	}
	for _, value := range []any{"invalid", time.Now().Add(-time.Hour), at.Format("2006-01-02T15:04:05")} {
		f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": value}, 400)
	}
	f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": false, "scheduledAt": at}, 400)
	f.request("PUT", path, "alice", map[string]any{"version": p.Version + 1, "publish": true, "scheduledAt": at}, 409)
	f.apply(p, "soon", 204)
	f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 400)
	f.apply(p, "", 204)
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 200))
	f.apply(p, "soon", 400)
	cid := newSubmissionID()
	f.request("PUT", "/my/contests/"+cid, "alice", map[string]any{"version": 0, "title": "Contest", "description": "", "startsAt": at, "endsAt": at.Add(time.Hour), "penaltyMinutes": 5, "problems": []map[string]any{{"id": p.ID, "points": 100}}}, 409)
	at = at.Add(time.Hour)
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 200))
	if p.ScheduledAt == nil || !p.ScheduledAt.Equal(at) {
		t.Fatal(p)
	}
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": false}, 200))
	if p.ScheduledAt != nil || p.PublishedVersion != 0 {
		t.Fatal(p)
	}
	if err := f.store.ReleaseScheduled(context.Background()); err != nil {
		t.Fatal(err)
	}
	f.request("GET", "/problems/"+p.ID, "", nil, 404)
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 200))
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true}, 200))
	if p.ScheduledAt != nil || p.PublishedVersion == 0 {
		t.Fatal(p)
	}
	f.request("PUT", path, "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 400)
}

// Every write answers with the problem as the next read shows it. The read and the answers select the same columns,
// so the fixed values (id, author, versions, reservation) are what catch a column that reads the wrong thing, and so is
// updated_at moving past the create's once the draft is saved (publication writes leave it alone). The contest stays
// empty through these writes; TestProblemReadAnswersContestPostgres checks a non-empty one.
func TestProblemWriteResponsesMatchReadPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	level := 3
	d := problems.Draft{Title: "Featured problem", Markdown: "Statement", Editorial: "SECRET EDITORIAL", TimeLimitMS: "1000", MemoryLimitMB: "256", Difficulty: &level, TestCases: []problems.TestCase{{Input: "1", Output: "2"}}}
	id := newSubmissionID()
	path := "/my/problems/" + id
	var created time.Time
	read := func(step, answer string) problems.Problem {
		t.Helper()
		got := scheduledProblem(t, answer)
		want := scheduledProblem(t, f.request("GET", path, "alice", nil, 200))
		if got.ID != id || got.ID != want.ID || got.Version != want.Version || !got.UpdatedAt.Equal(want.UpdatedAt) || !reflect.DeepEqual(got.Draft, want.Draft) ||
			got.PublishedVersion != want.PublishedVersion || got.Author != "alice" || want.Author != "alice" || got.ContestID != "" || want.ContestID != "" ||
			(got.ScheduledAt == nil) != (want.ScheduledAt == nil) || (got.ScheduledAt != nil && !got.ScheduledAt.Equal(*want.ScheduledAt)) {
			t.Fatalf("%s answered %+v, read %+v", step, got, want)
		}
		if !created.IsZero() && !got.UpdatedAt.After(created) {
			t.Fatalf("%s answered updated_at %v, not after the create's %v", step, got.UpdatedAt, created)
		}
		return got
	}
	p := read("create", f.request("PUT", path, "alice", map[string]any{"version": 0, "draft": d}, 200))
	created = p.UpdatedAt
	if p.Version != 1 || p.PublishedVersion != 0 || p.ScheduledAt != nil {
		t.Fatal(p)
	}
	d.Markdown = "Saved statement"
	p = read("save", f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": d}, 200))
	if p.Version != 2 || p.Draft.Markdown != "Saved statement" {
		t.Fatal(p)
	}
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	p = read("schedule", f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": at}, 200))
	if p.Version != 3 || p.PublishedVersion != 0 || p.ScheduledAt == nil || !p.ScheduledAt.Equal(at) {
		t.Fatal(p)
	}
	p = read("publish", f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true}, 200))
	if p.Version != 4 || p.PublishedVersion != 4 || p.ScheduledAt != nil {
		t.Fatal(p)
	}
	p = read("unpublish", f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": false}, 200))
	if p.Version != 5 || p.PublishedVersion != 0 || p.ScheduledAt != nil {
		t.Fatal(p)
	}
}

// The problem read names the contest that holds the problem in its own contestId, apart from the contest editors.
func TestProblemReadAnswersContestPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	cid := newSubmissionID()
	at := time.Now().UTC().Add(time.Hour).Truncate(time.Second)
	f.request("PUT", "/my/contests/"+cid, "alice", map[string]any{"version": 0, "title": "Contest", "description": "", "startsAt": at, "endsAt": at.Add(time.Hour), "penaltyMinutes": 5, "problems": []map[string]any{{"id": p.ID, "points": 100}}}, 200)
	if got := scheduledProblem(t, f.request("GET", "/my/problems/"+p.ID, "alice", nil, 200)); got.ContestID != cid {
		t.Fatalf("contestId %q, want %q", got.ContestID, cid)
	}
}

func TestScheduledProblemImagesAndReleaseOnReadPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	imageID := newSubmissionID()
	oldImageID := newSubmissionID()
	f.exec(`INSERT INTO content_images(id,owner_id,digest,media_type,data,size) VALUES($1,'alice','x','image/png','x',1)`, imageID)
	f.exec(`INSERT INTO content_images(id,owner_id,digest,media_type,data,size) VALUES($1,'alice','y','image/png','y',1)`, oldImageID)
	f.exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{editorial}',to_jsonb('![](/api/images/'||$2::text||')')) WHERE id=$1`, p.ID, oldImageID)
	path := "/my/problems/" + p.ID
	p = scheduledProblem(t, f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": time.Now().Add(time.Hour)}, 200))
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, p.ID)
	p.Draft.Editorial = "![](/api/images/" + imageID + ")"
	p = scheduledProblem(t, f.request("PUT", path, "tester", map[string]any{"version": p.Version, "draft": p.Draft}, 200))
	// A tester's saved edit replaces the reserved image, which stays private until publication.
	f.request("GET", "/images/"+imageID, "", nil, 404)
	f.request("GET", "/my/images/"+imageID, "bob", nil, 404)
	f.request("DELETE", "/my/images/"+imageID, "alice", nil, 409)
	f.request("DELETE", "/my/images/"+oldImageID, "alice", nil, 204)
	f.exec(`UPDATE problem_drafts SET scheduled_at=clock_timestamp()-interval '1 second' WHERE id=$1`, p.ID)
	// An image may be the first request after the deadline, before any problem page or minute tick.
	f.request("GET", "/images/"+imageID, "", nil, 200)
	if !strings.Contains(f.request("GET", "/problems/"+p.ID, "", nil, 200), imageID) {
		t.Fatal("reading a due problem did not release the reserved editorial")
	}
	f.request("GET", "/images/"+imageID, "", nil, 200)
	f.request("POST", "/my/submissions", "bob", map[string]any{"problemId": p.ID, "runtime": "cpp17", "source": "int main() {}"}, 202)
}

func TestScheduledProblemsReleaseAllDueOnFirstReadPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	// More than a background batch is due, but the first reader must see all of it.
	f.exec(`INSERT INTO problem_drafts(id,owner_id,draft,scheduled_draft,scheduled_version,scheduled_at)
 SELECT gen_random_uuid(),owner_id,draft,draft,version,clock_timestamp()-interval '1 second'
 FROM problem_drafts CROSS JOIN generate_series(1,101) WHERE id=$1`, p.ID)
	f.request("GET", "/problems", "", nil, 200)
	var published, pending int
	if err := f.store.Pool().QueryRow(context.Background(), `SELECT count(*) FILTER (WHERE published_draft IS NOT NULL),
 count(*) FILTER (WHERE scheduled_at IS NOT NULL) FROM problem_drafts`).Scan(&published, &pending); err != nil {
		t.Fatal(err)
	}
	if published != 101 || pending != 0 {
		t.Fatalf("first read published %d problems, left %d pending", published, pending)
	}
}

func TestScheduledProblemWaitsForConcurrentUpdatePostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	f.exec(`UPDATE problem_drafts SET scheduled_draft=draft,scheduled_version=version,scheduled_at=clock_timestamp()-interval '1 second' WHERE id=$1`, p.ID)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	tx, err := f.store.Pool().Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	if _, err = tx.Exec(ctx, `SELECT id FROM problem_drafts WHERE id=$1 FOR UPDATE`, p.ID); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- f.store.ReleaseScheduled(ctx) }()
	select {
	case err := <-done:
		t.Fatalf("release returned before the due problem could be published: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-done; err != nil {
		t.Fatal(err)
	}
	if _, err = f.store.PublicGet(ctx, p.ID, ""); err != nil {
		t.Fatalf("due problem still unavailable after release: %v", err)
	}
}

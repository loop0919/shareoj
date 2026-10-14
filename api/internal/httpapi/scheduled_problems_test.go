package httpapi

import (
	"context"
	"encoding/json"
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

func TestScheduledProblemPrivacyAndSnapshotPostgres(t *testing.T) {
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
	// Saving even an incomplete draft leaves the complete reserved snapshot intact.
	p.Draft.Markdown = ""
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": p.Draft}, 200))
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
	if public.Markdown != "Statement" || public.Editorial != "SECRET EDITORIAL" || !public.PublishedAt.Equal(due) {
		t.Fatal(public)
	}
	after := scheduledProblem(t, f.request("GET", path, "alice", nil, 200))
	if after.ScheduledAt != nil || after.Version != p.Version+1 || after.PublishedVersion != 2 || !after.EverPublished || after.Draft.Markdown != "" {
		t.Fatal(after)
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

func TestScheduledProblemImagesAndReleaseOnReadPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	imageID := newSubmissionID()
	f.exec(`INSERT INTO content_images(id,owner_id,digest,media_type,data,size) VALUES($1,'alice','x','image/png','x',1)`, imageID)
	f.exec(`UPDATE problem_drafts SET draft=jsonb_set(draft,'{editorial}',to_jsonb('![](/api/images/'||$2::text||')')) WHERE id=$1`, p.ID, imageID)
	path := "/my/problems/" + p.ID
	p = scheduledProblem(t, f.request("PUT", path+"/publication", "alice", map[string]any{"version": p.Version, "publish": true, "scheduledAt": time.Now().Add(time.Hour)}, 200))
	p.Draft.Editorial = ""
	p = scheduledProblem(t, f.request("PUT", path, "alice", map[string]any{"version": p.Version, "draft": p.Draft}, 200))
	// Only the reservation still references the image. It stays private and cannot be deleted.
	f.request("GET", "/images/"+imageID, "", nil, 404)
	f.request("GET", "/my/images/"+imageID, "bob", nil, 404)
	f.request("DELETE", "/my/images/"+imageID, "alice", nil, 409)
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

package httpapi

import (
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"judge/api/internal/contests"
)

func TestClarificationAnnouncementsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "writer", "tester", "cos", "bob", "outsider")
	const cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	const other = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
	const p1 = "11111111-1111-4111-8111-111111111111"
	const p2 = "22222222-2222-4222-8222-222222222222"
	f.problem(p1)
	f.problem(p2)
	f.create(cid, p1, nil)
	f.create(other, p2, nil)
	f.exec(`UPDATE problem_drafts SET owner_id='writer' WHERE id=$1`, p1)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, p1)
	f.exec(`INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,'cos')`, cid)
	f.join(cid, "bob", nil)
	base := "/my/contests/" + cid + "/clarifications"
	body := map[string]any{"answer": "スタッフからのお知らせ"}
	f.request("POST", base, "", body, 401)
	f.request("POST", base, "noprofile", body, 403)
	for _, actor := range []string{"bob", "outsider"} {
		f.request("POST", base, actor, body, 404)
	}
	for _, bad := range []map[string]any{
		{"answer": ""}, {"answer": "　\n"}, {"answer": strings.Repeat("質", 4001)}, {"answer": "bad\x00text"},
		{"answer": "回答", "question": "質問"}, {"answer": "回答", "problemId": "bad"},
	} {
		f.request("POST", base, "alice", bad, 400)
	}
	f.request("POST", base, "alice", map[string]any{"answer": "回答", "problemId": p2}, 404)
	for i, actor := range []string{"alice", "cos", "writer", "tester"} {
		problem := ""
		if i%2 == 1 {
			problem = p1
		}
		switch i {
		case 1:
			f.schedule(cid, "running")
		case 3:
			f.schedule(cid, "ended")
		}
		f.request("POST", base, actor, map[string]any{"answer": "お知らせ", "problemId": problem}, 204)
		for _, viewer := range []string{"alice", "cos", "writer", "tester", "bob", "outsider", "noprofile"} {
			var page offsetResponse[contests.Clarification]
			f.decode(f.request("GET", base, viewer, nil, 200), &page)
			if len(page.Items) != i+1 {
				t.Fatalf("%s: %+v", viewer, page)
			}
			q := page.Items[0]
			staff := viewer != "bob" && viewer != "outsider" && viewer != "noprofile"
			if q.Author != "" || q.Question != "" || q.ProblemID != problem || q.Answer != "お知らせ" || q.AnsweredBy != actor || q.AnsweredAt == nil || q.PublishedAt == nil || q.CanManage != staff {
				t.Fatalf("%s: %+v", viewer, q)
			}
		}
	}
	var page offsetResponse[contests.Clarification]
	f.decode(f.request("GET", base, "bob", nil, 200), &page)
	path := base + "/" + page.Items[0].ID
	f.request("PUT", path, "bob", map[string]any{"answer": "改変", "version": 1}, 404)
	f.request("PUT", path, "cos", map[string]any{"answer": "補足", "version": 1}, 204)
	f.fail("PUT", path, "writer", map[string]any{"answer": "競合", "version": 1}, 409, "clarification_conflict")
	f.exec(`DELETE FROM problem_testers WHERE problem_id=$1 AND owner_id='tester'`, p1)
	f.request("POST", base, "tester", body, 404)
	f.request("PUT", path, "tester", map[string]any{"answer": "改変", "version": 2}, 404)
	f.decode(f.request("GET", base, "outsider", nil, 200), &page)
	if q := page.Items[0]; q.Answer != "補足" || q.AnsweredBy != "cos" || q.PublishedAt == nil || q.Version != 2 {
		t.Fatal(q)
	}
	if n := f.count(`SELECT count(*) FROM contest_clarifications WHERE owner_id IS NULL`); n != 4 {
		t.Fatalf("announcements: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM notifications WHERE kind='clarification_published' AND owner_id='bob'`); n != 4 {
		t.Fatalf("publication notifications: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM notifications WHERE kind IN ('clarification_question','clarification_answer')`); n != 0 {
		t.Fatalf("unexpected question/answer notifications: %d", n)
	}
}

func TestClarificationsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "writer", "tester", "otherwriter", "othertester", "cos", "bob", "carol", "outsider")
	const (
		cid   = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		other = "dddddddd-dddd-4ddd-8ddd-dddddddddddd"
		p1    = "11111111-1111-4111-8111-111111111111"
		p2    = "22222222-2222-4222-8222-222222222222"
		p3    = "33333333-3333-4333-8333-333333333333"
	)
	for _, id := range []string{p1, p2, p3} {
		f.problem(id)
	}
	f.create(cid, p1, map[string]any{"teamMode": true, "problems": []map[string]any{{"id": p1, "points": 100}, {"id": p2, "points": 100}}})
	f.create(other, p3, nil)
	f.exec(`UPDATE problem_drafts SET owner_id='writer' WHERE id=$1`, p1)
	f.exec(`UPDATE problem_drafts SET owner_id='otherwriter' WHERE id=$1`, p2)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester'),($2,'othertester')`, p1, p2)
	f.exec(`INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,'cos')`, cid)
	f.join(cid, "bob", create("Team"))
	f.join(cid, "carol", joinCode(f.team(cid, "bob").Code))
	base := "/my/contests/" + cid + "/clarifications"
	body := map[string]any{"problemId": p1, "question": "非公開の質問 <script>"}
	f.request("GET", base, "", nil, 401)
	f.request("POST", base, "", body, 401)
	f.fail("POST", base, "bob", body, 409, "clarification_unavailable")
	f.schedule(cid, "running")
	for _, actor := range []string{"alice", "writer", "tester", "cos", "outsider"} {
		f.request("POST", base, actor, body, 409)
	}
	for _, question := range []string{"", "　\n", strings.Repeat("質", 4001), "bad\x00text"} {
		f.request("POST", base, "bob", map[string]any{"problemId": p1, "question": question}, 400)
	}
	f.request("POST", base, "bob", map[string]any{"problemId": "bad", "question": "質問"}, 400)
	f.request("POST", base, "bob", map[string]any{"problemId": p3, "question": "別コンテスト"}, 409)
	f.request("POST", base, "bob", body, 204)
	list := func(actor string) []contests.Clarification {
		t.Helper()
		var page offsetResponse[contests.Clarification]
		w := f.do(f.newRequest("GET", base, actor, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("list: %d %s", w.Code, w.Body.String())
		}
		f.decode(w.Body.String(), &page)
		return page.Items
	}
	for _, actor := range []string{"alice", "cos", "writer", "tester", "bob"} {
		items := list(actor)
		if len(items) != 1 || items[0].CanManage != (actor != "bob") {
			t.Fatalf("%s: %+v", actor, items)
		}
	}
	q := list("bob")[0]
	path := base + "/" + q.ID
	for _, actor := range []string{"carol", "otherwriter", "othertester", "outsider", "noprofile"} {
		if items := list(actor); len(items) != 0 {
			t.Fatalf("leaked to %s: %+v", actor, items)
		}
		status := 404
		if actor == "noprofile" {
			status = 403
		}
		f.request("PUT", path, actor, map[string]any{"answer": "attack", "version": 1}, status)
		f.request("PUT", path, actor, map[string]any{"publish": true, "version": 1}, status)
	}
	f.request("PUT", path, "bob", map[string]any{"answer": "self", "version": 1}, 404)
	f.request("PUT", "/my/contests/"+other+"/clarifications/"+q.ID, "alice", map[string]any{"publish": true, "version": 1}, 404)
	for _, body := range []map[string]any{{"answer": " ", "version": 1}, {"publish": true, "version": 0}, {"version": 1}, {"answer": strings.Repeat("a", 4001), "version": 1}} {
		f.request("PUT", path, "tester", body, 400)
	}
	f.request("PUT", path, "writer", map[string]any{"answer": "回答1", "version": 1}, 204)
	f.fail("PUT", path, "tester", map[string]any{"publish": true, "version": 1}, 409, "clarification_conflict")
	f.fail("PUT", path, "cos", map[string]any{"answer": "古い版", "version": 1}, 409, "clarification_conflict")
	q = list("bob")[0]
	if q.Answer != "回答1" || q.AnsweredBy != "writer" || q.AnsweredAt == nil || q.PublishedAt != nil || q.Version != 2 {
		t.Fatal(q)
	}
	f.request("PUT", path, "tester", map[string]any{"answer": "回答2", "version": 2}, 204)
	f.exec(`DELETE FROM problem_testers WHERE problem_id=$1 AND owner_id='tester'`, p1)
	if len(list("tester")) != 0 {
		t.Fatal("former tester kept access")
	}
	f.request("PUT", path, "tester", map[string]any{"publish": true, "version": 3}, 404)
	f.exec(`UPDATE contest_setters SET left_at=clock_timestamp() WHERE contest_id=$1 AND owner_id='cos'`, cid)
	if len(list("cos")) != 0 {
		t.Fatal("former co-setter kept access")
	}
	f.request("PUT", path, "cos", map[string]any{"publish": true, "version": 3}, 404)
	f.request("PUT", path, "alice", map[string]any{"publish": true, "version": 3}, 204)
	for _, actor := range []string{"carol", "otherwriter", "othertester", "cos", "outsider", "tester", "noprofile"} {
		items := list(actor)
		if len(items) != 1 || items[0].Answer != "回答2" || items[0].PublishedAt == nil || items[0].CanManage {
			t.Fatalf("public %s: %+v", actor, items)
		}
		status := 404
		if actor == "noprofile" {
			status = 403
		}
		f.request("PUT", path, actor, map[string]any{"answer": "attack", "version": 4}, status)
	}
	f.request("GET", base, "", nil, 401)
	f.request("GET", "/contests/"+cid+"/clarifications", "", nil, 404)
	f.schedule(cid, "ended")
	f.request("POST", base, "bob", body, 409)
	// A later answer keeps the publication and cannot be replaced by a stale manager.
	f.request("PUT", path, "writer", map[string]any{"answer": "終了後の補足", "version": 4, "publish": false}, 204)
	q = list("outsider")[0]
	if q.Answer != "終了後の補足" || q.PublishedAt == nil {
		t.Fatal(q)
	}
	// Two writers using the same version cannot both succeed.
	requests := []*httptest.ResponseRecorder{httptest.NewRecorder(), httptest.NewRecorder()}
	r1 := f.newRequest("PUT", path, "writer", map[string]any{"answer": "concurrent writer", "version": 5})
	r2 := f.newRequest("PUT", path, "alice", map[string]any{"answer": "concurrent owner", "version": 5})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); f.handler.ServeHTTP(requests[0], r1) }()
	go func() { defer wg.Done(); f.handler.ServeHTTP(requests[1], r2) }()
	wg.Wait()
	if (requests[0].Code != 204 || requests[1].Code != 409) && (requests[0].Code != 409 || requests[1].Code != 204) {
		t.Fatalf("concurrent: %d %d", requests[0].Code, requests[1].Code)
	}
	// Filtering happens before pagination, and the count never includes hidden questions.
	f.exec(`INSERT INTO contest_clarifications(contest_id,problem_id,owner_id,question) SELECT $1,$2,'bob','hidden' FROM generate_series(1,51)`, cid, p1)
	var page offsetResponse[contests.Clarification]
	f.decode(f.request("GET", base, "alice", nil, 200), &page)
	if len(page.Items) != 50 || !page.HasMore {
		t.Fatal("first page", page)
	}
	f.decode(f.request("GET", base+"?offset=50", "alice", nil, 200), &page)
	if len(page.Items) != 2 || page.HasMore {
		t.Fatal("second page", page)
	}
	if len(list("outsider")) != 1 {
		t.Fatal("private pagination leak")
	}
	f.request("GET", base+"?offset=-1", "alice", nil, 400)
	// Both directly assigned roles may publish, even without an answer yet.
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester')`, p1)
	for _, actor := range []string{"writer", "tester"} {
		private := list(actor)[0]
		f.request("PUT", base+"/"+private.ID, actor, map[string]any{"publish": true, "version": private.Version}, 204)
	}
	f.exec(`UPDATE user_profiles SET deleted_at=clock_timestamp() WHERE owner_id='outsider'`)
	f.request("GET", base, "outsider", nil, 401)
}

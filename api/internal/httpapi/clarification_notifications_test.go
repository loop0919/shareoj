package httpapi

import (
	"slices"
	"testing"
)

func TestClarificationNotificationsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "writer", "tester", "otherwriter", "othertester", "cos", "bob", "carol")
	const (
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
		p1  = "11111111-1111-4111-8111-111111111111"
		p2  = "22222222-2222-4222-8222-222222222222"
	)
	for _, id := range []string{p1, p2} {
		f.problem(id)
	}
	f.create(cid, p1, map[string]any{"teamMode": true, "problems": []map[string]any{{"id": p1, "points": 100}, {"id": p2, "points": 100}}})
	f.exec(`UPDATE problem_drafts SET owner_id='writer' WHERE id=$1`, p1)
	f.exec(`UPDATE problem_drafts SET owner_id='otherwriter' WHERE id=$1`, p2)
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'tester'),($2,'othertester')`, p1, p2)
	f.exec(`INSERT INTO contest_setters(contest_id,owner_id) VALUES($1,'cos')`, cid)
	f.join(cid, "bob", create("Team"))
	f.join(cid, "carol", joinCode(f.team(cid, "bob").Code))
	f.schedule(cid, "running")
	base := "/my/contests/" + cid + "/clarifications"
	// The bell lists newest first, so compare without order.
	want := func(actor string, kinds ...string) {
		t.Helper()
		var page struct {
			Notifications []struct{ Kind, Actor, ContestID, ProblemID string }
		}
		f.decode(f.request("GET", "/my/notifications?history=1", actor, nil, 200), &page)
		got := []string{}
		for _, item := range page.Notifications {
			if item.ProblemID != "" { // the tester invitations of the fixture
				continue
			}
			if item.ContestID != cid {
				t.Fatalf("%s: %+v", actor, item)
			}
			got = append(got, item.Kind+":"+item.Actor)
		}
		slices.Sort(got)
		slices.Sort(kinds)
		if !slices.Equal(got, kinds) && len(got)+len(kinds) != 0 {
			t.Fatalf("%s: got %v, want %v", actor, got, kinds)
		}
	}
	f.request("POST", base, "bob", map[string]any{"problemId": p1, "question": "質問"}, 204)
	want("alice", "clarification_question:bob")
	want("writer", "clarification_question:bob")
	want("tester", "clarification_question:bob")
	for _, u := range []string{"cos", "otherwriter", "othertester", "carol", "bob"} {
		want(u)
	}
	var page struct{ Items []struct{ ID string } }
	f.decode(f.request("GET", base, "bob", nil, 200), &page)
	path := base + "/" + page.Items[0].ID
	f.request("PUT", path, "writer", map[string]any{"answer": "回答", "version": 1}, 204)
	want("bob", "clarification_answer:writer")
	f.request("PUT", path, "alice", map[string]any{"publish": true, "version": 2}, 204)
	want("bob", "clarification_answer:writer", "clarification_published:alice")
	want("carol", "clarification_published:alice")
	want("writer", "clarification_question:bob")
	// Publishing again, or answering a published question, adds no publication notice.
	f.request("PUT", path, "alice", map[string]any{"publish": true, "version": 3}, 204)
	want("carol", "clarification_published:alice")
	f.request("POST", base, "bob", map[string]any{"problemId": p2, "question": "質問2"}, 204)
	want("otherwriter", "clarification_question:bob")
	want("othertester", "clarification_question:bob")
	want("writer", "clarification_question:bob")
}

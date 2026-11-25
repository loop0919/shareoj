package httpapi

import (
	"bytes"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"judge/api/internal/folders"
	"judge/api/internal/problems"
)

// TestFoldersPostgres pins the folders of the my page: their names, depth and number, who can reach them, and moving a
// folder together with the folders below it.
func TestFoldersPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	var top, child, leaf folders.Folder
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": " コンテスト用 ", "parentId": nil}, 201), &top)
	if top.Name != "コンテスト用" || top.ParentID != nil {
		t.Fatalf("top folder: %+v", top)
	}
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "第3回", "parentId": top.ID}, 201), &child)
	if child.ParentID == nil || *child.ParentID != top.ID {
		t.Fatalf("child folder: %+v", child)
	}
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "予備", "parentId": child.ID}, 201), &leaf)
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "4段目", "parentId": leaf.ID}, 409, "folder_depth")
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "第3回", "parentId": top.ID}, 409, "folder_name_taken")
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "コンテスト用", "parentId": nil}, 409, "folder_name_taken")
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "  ", "parentId": nil}, 400, "invalid_folder")
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": strings.Repeat("あ", 41), "parentId": nil}, 400, "invalid_folder")
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "改\n行", "parentId": nil}, 400, "invalid_folder")
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "親", "parentId": "top"}, 400, "invalid_folder")
	f.fail("DELETE", "/my/folders/top", "alice", nil, 400, "invalid_folder")
	f.fail("PUT", "/my/folders/top", "alice", map[string]any{"name": "親", "parentId": nil}, 400, "invalid_folder")
	f.request("POST", "/my/folders", "alice", map[string]any{"name": strings.Repeat("あ", 40), "parentId": top.ID}, 201)
	f.fail("PUT", "/my/folders/"+top.ID, "alice", map[string]any{"name": "コンテスト用", "parentId": leaf.ID}, 409, "folder_cycle")
	f.fail("PUT", "/my/folders/"+top.ID, "alice", map[string]any{"name": "コンテスト用", "parentId": top.ID}, 409, "folder_cycle")
	// A move names both the name and the parent, so that leaving one out cannot move a folder to the top.
	f.fail("PUT", "/my/folders/"+child.ID, "alice", map[string]any{"name": "第3回"}, 400, "invalid_request")
	f.fail("PUT", "/my/folders/"+child.ID, "alice", map[string]any{"parentId": top.ID}, 400, "invalid_request")

	// Folders are their owner's alone.
	if body := f.request("GET", "/my/folders", "bob", nil, 200); !strings.Contains(body, `"folders":[]`) || !strings.Contains(body, `"items":[]`) {
		t.Fatalf("bob reads folders: %s", body)
	}
	f.fail("PUT", "/my/folders/"+top.ID, "bob", map[string]any{"name": "奪う", "parentId": nil}, 404, "not_found")
	f.fail("DELETE", "/my/folders/"+top.ID, "bob", nil, 404, "not_found")
	f.fail("POST", "/my/folders", "bob", map[string]any{"name": "中へ", "parentId": top.ID}, 404, "not_found")
	if n := f.count(`SELECT count(*) FROM content_folders WHERE owner_id='bob'`); n != 0 {
		t.Fatalf("bob's folders: %d", n)
	}

	// A move counts the levels below the folder it moves.
	var regular, candidate, moved folders.Folder
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "定期便用", "parentId": nil}, 201), &regular)
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "候補", "parentId": regular.ID}, 201), &candidate)
	f.fail("PUT", "/my/folders/"+regular.ID, "alice", map[string]any{"name": "定期便用", "parentId": child.ID}, 409, "folder_depth")
	f.decode(f.request("PUT", "/my/folders/"+regular.ID, "alice", map[string]any{"name": "定期便用", "parentId": top.ID}, 200), &moved)
	if moved.ID != regular.ID || moved.ParentID == nil || *moved.ParentID != top.ID || moved.Name != "定期便用" {
		t.Fatalf("moved folder: %+v", moved)
	}
	f.fail("POST", "/my/folders", "alice", map[string]any{"name": "4段目", "parentId": candidate.ID}, 409, "folder_depth")
	f.fail("PUT", "/my/folders/"+regular.ID, "alice", map[string]any{"name": "第3回", "parentId": top.ID}, 409, "folder_name_taken")

	// A rename keeps the folder where it is.
	f.request("PUT", "/my/folders/"+child.ID, "alice", map[string]any{"name": " 第4回 ", "parentId": top.ID}, 200)
	var list struct {
		Folders []folders.Folder `json:"folders"`
	}
	f.decode(f.request("GET", "/my/folders", "alice", nil, 200), &list)
	renamed := false
	for _, folder := range list.Folders {
		if folder.ID == child.ID {
			renamed = folder.Name == "第4回" && folder.ParentID != nil && *folder.ParentID == top.ID
		}
	}
	if len(list.Folders) != 6 || !renamed {
		t.Fatalf("alice's folders after the rename: %+v", list.Folders)
	}

	// Deleting a folder deletes the folders below it.
	f.request("DELETE", "/my/folders/"+top.ID, "alice", nil, 204)
	if n := f.count(`SELECT count(*) FROM content_folders WHERE owner_id='alice'`); n != 0 {
		t.Fatalf("alice's folders after the delete: %d", n)
	}
	f.fail("DELETE", "/my/folders/"+top.ID, "alice", nil, 404, "not_found")

	// A creation without a parentId is at the top, and a null parentId moves a folder back there: deleting its old
	// parent leaves it.
	var outer, inner, back folders.Folder
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "あとで"}, 201), &outer)
	if outer.ParentID != nil {
		t.Fatalf("folder made without a parent: %+v", outer)
	}
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "候補", "parentId": outer.ID}, 201), &inner)
	f.decode(f.request("PUT", "/my/folders/"+inner.ID, "alice", map[string]any{"name": "候補", "parentId": nil}, 200), &back)
	if back.ID != inner.ID || back.ParentID != nil {
		t.Fatalf("folder moved to the top: %+v", back)
	}
	f.request("DELETE", "/my/folders/"+outer.ID, "alice", nil, 204)
	f.request("DELETE", "/my/folders/"+inner.ID, "alice", nil, 204)

	// Two creations at once stop at the limit. The test holds alice's profile row until both requests wait behind it,
	// so that they overlap however fast each one runs.
	f.exec(`INSERT INTO content_folders(id,owner_id,name) SELECT gen_random_uuid(),'alice','既存'||n FROM generate_series(1,$1::int) n`, folders.MaxFolders-1)
	requests := []*http.Request{
		f.newRequest("POST", "/my/folders", "alice", map[string]any{"name": "同時A", "parentId": nil}),
		f.newRequest("POST", "/my/folders", "alice", map[string]any{"name": "同時B", "parentId": nil}),
	}
	hold, err := f.store.Pool().Begin(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = hold.Rollback(f.ctx) }()
	var holder int
	if err = hold.QueryRow(f.ctx, `SELECT pg_backend_pid() FROM user_profiles WHERE owner_id='alice' FOR UPDATE`).Scan(&holder); err != nil {
		t.Fatal(err)
	}
	responses := make([]*httptest.ResponseRecorder, len(requests))
	var wg sync.WaitGroup
	for i, r := range requests {
		wg.Add(1)
		go func() {
			defer wg.Done()
			responses[i] = f.do(r)
		}()
	}
	// A second waiter for a row waits behind the first one, so the count follows the chain back to the holder.
	const waiting = `WITH RECURSIVE w(pid) AS (SELECT $1::int UNION SELECT a.pid FROM pg_stat_activity a, w WHERE w.pid=ANY(pg_blocking_pids(a.pid)))
 SELECT count(*)-1 FROM w`
	for deadline := time.Now().Add(10 * time.Second); f.count(waiting, holder) < len(requests); {
		if time.Now().After(deadline) {
			t.Fatal("the creations did not wait for alice's profile row")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err = hold.Commit(f.ctx); err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	created, limited := 0, 0
	for _, w := range responses {
		switch {
		case w.Code == 201:
			created++
		case w.Code == 409 && strings.Contains(w.Body.String(), `"error":"folder_limit"`):
			limited++
		default:
			t.Fatalf("concurrent creation: %d %s", w.Code, w.Body.String())
		}
	}
	if created != 1 || limited != 1 {
		t.Fatalf("concurrent creations: %d created, %d limited", created, limited)
	}
	if n := f.count(`SELECT count(*) FROM content_folders WHERE owner_id='alice'`); n != folders.MaxFolders {
		t.Fatalf("alice's folders at the limit: %d", n)
	}

	// The list is never cached.
	if w := f.do(f.newRequest("GET", "/my/folders", "alice", nil)); w.Code != 200 || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("list headers: %d %v", w.Code, w.Header())
	}
}

// TestFolderItemsPostgres pins sorting problems and posts into folders: each person sorts only what they write or
// manage, an id of anything else goes nowhere and answers like any other, and a placement goes with its folder, its
// problem or post, and the right that let its owner sort it.
func TestFolderItemsPostgres(t *testing.T) {
	f := newTeamTest(t, "alice", "bob")
	const (
		p1      = "f1000000-0000-4000-8000-000000000001"
		p2      = "f1000000-0000-4000-8000-000000000002"
		post1   = "f2000000-0000-4000-8000-000000000001"
		post2   = "f2000000-0000-4000-8000-000000000002"
		missing = "f3000000-0000-4000-8000-000000000001"
		path    = "/my/folder-items"
	)
	f.problem(p1)
	if _, err := f.store.Save(f.ctx, "bob", p2, 0, problems.Draft{Title: "Bob"}); err != nil {
		t.Fatal(err)
	}
	post := map[string]any{"version": 0, "title": "解説", "markdown": "本文", "problemIds": []string{}, "tags": []string{}}
	f.request("PUT", "/my/posts/"+post1, "alice", post, 200)
	f.request("PUT", "/my/posts/"+post2, "bob", post, 200)
	var fa, fa2, fb folders.Folder
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "コンテスト用", "parentId": nil}, 201), &fa)
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "あとで", "parentId": nil}, 201), &fa2)
	f.decode(f.request("POST", "/my/folders", "bob", map[string]any{"name": "テスト中", "parentId": nil}, 201), &fb)

	problem := func(id string) map[string]string { return map[string]string{"kind": "problem", "id": id} }
	article := func(id string) map[string]string { return map[string]string{"kind": "post", "id": id} }
	place := func(folder any, refs ...map[string]string) map[string]any {
		return map[string]any{"folderId": folder, "items": refs}
	}
	type item struct {
		Kind     string `json:"kind"`
		ID       string `json:"id"`
		FolderID string `json:"folderId"`
	}
	// expect compares the placements GET /my/folders shows the owner, in any order.
	expect := func(owner string, want ...item) {
		t.Helper()
		var list struct {
			Items []item `json:"items"`
		}
		f.decode(f.request("GET", "/my/folders", owner, nil, 200), &list)
		order := func(a, b item) int { return strings.Compare(a.Kind+a.ID, b.Kind+b.ID) }
		slices.SortFunc(list.Items, order)
		slices.SortFunc(want, order)
		if !slices.Equal(list.Items, want) {
			t.Fatalf("%s's placements: got %+v want %+v", owner, list.Items, want)
		}
	}
	rows := func(table, owner string) int {
		t.Helper()
		return f.count(`SELECT count(*) FROM `+table+` WHERE owner_id=$1`, owner)
	}

	if body := f.request("PUT", path, "alice", place(fa.ID, problem(p1), article(post1)), 204); body != "" {
		t.Fatalf("placement body: %q", body)
	}
	expect("alice", item{"problem", p1, fa.ID}, item{"post", post1, fa.ID})
	// A problem sits in one folder of each person: placing it again moves it.
	f.request("PUT", path, "alice", place(fa2.ID, problem(p1)), 204)
	if n := rows("problem_folder_items", "alice"); n != 1 {
		t.Fatalf("alice's problem placements after a move: %d", n)
	}
	expect("alice", item{"problem", p1, fa2.ID}, item{"post", post1, fa.ID})
	// A null folder makes it unsorted again.
	f.request("PUT", path, "alice", place(nil, article(post1)), 204)
	if n := rows("post_folder_items", "alice"); n != 0 {
		t.Fatalf("alice's post placements after unsorting: %d", n)
	}
	expect("alice", item{"problem", p1, fa2.ID})

	// Sorting leaves the problem and the post alone: placing them, moving them, unsorting them and deleting the folder
	// that holds them change neither their edit time nor their version.
	content := func() string {
		t.Helper()
		var s string
		if err := f.store.Pool().QueryRow(f.ctx, `SELECT format('%s %s %s %s',d.updated_at,d.version,p.updated_at,p.version)
 FROM problem_drafts d, blog_posts p WHERE d.id=$1 AND p.id=$2`, p1, post1).Scan(&s); err != nil {
			t.Fatal(err)
		}
		return s
	}
	var fc folders.Folder
	f.decode(f.request("POST", "/my/folders", "alice", map[string]any{"name": "消す用", "parentId": nil}, 201), &fc)
	before := content()
	both := []map[string]string{problem(p1), article(post1)}
	for _, step := range []struct {
		name         string
		method, path string
		body         any
		want         int
	}{
		{"placing", "PUT", path, place(fa.ID, both...), 204},
		{"moving", "PUT", path, place(fc.ID, both...), 204},
		{"unsorting", "PUT", path, place(nil, both...), 204},
		{"placing again", "PUT", path, place(fc.ID, both...), 204},
		{"deleting their folder", "DELETE", "/my/folders/" + fc.ID, nil, 204},
	} {
		f.request(step.method, step.path, "alice", step.body, step.want)
		if after := content(); after != before {
			t.Fatalf("problem and post after %s: %s, before %s", step.name, after, before)
		}
	}
	expect("alice")
	f.request("PUT", path, "alice", place(fa2.ID, problem(p1)), 204)

	// Nothing tells alice a problem she cannot open, a post of bob's and an id of nothing apart: each answers the same,
	// into a folder or out of one, and writes nothing.
	var first *httptest.ResponseRecorder
	for _, folder := range []any{fa.ID, nil} {
		for _, ref := range []map[string]string{problem(p2), article(post2), problem(missing), article(missing)} {
			w := f.do(f.newRequest("PUT", path, "alice", place(folder, ref)))
			if w.Code != 204 || w.Body.Len() != 0 {
				t.Fatalf("placing %v in %v: %d %q", ref, folder, w.Code, w.Body.String())
			}
			if first == nil {
				first = w
			} else if !maps.EqualFunc(w.Header(), first.Header(), slices.Equal) {
				t.Fatalf("placing %v in %v: headers %v, first %v", ref, folder, w.Header(), first.Header())
			}
		}
	}
	if n := f.count(`SELECT (SELECT count(*) FROM problem_folder_items)+(SELECT count(*) FROM post_folder_items)`); n != 1 {
		t.Fatalf("placements after ids alice cannot sort: %d", n)
	}
	expect("alice", item{"problem", p1, fa2.ID})
	// Nor does a batch that mixes them with what she may sort: it answers byte for byte like a batch of two she may
	// sort, places the one and writes nothing for bob's problem.
	sortable := f.do(f.newRequest("PUT", path, "alice", place(fa.ID, problem(p1), article(post1))))
	mixed := f.do(f.newRequest("PUT", path, "alice", place(fa2.ID, problem(p1), problem(p2))))
	if mixed.Code != 204 || mixed.Body.Len() != 0 || mixed.Code != sortable.Code || !bytes.Equal(mixed.Body.Bytes(), sortable.Body.Bytes()) || !maps.EqualFunc(mixed.Header(), sortable.Header(), slices.Equal) {
		t.Fatalf("mixed batch: %d %q %v, sortable batch: %d %q %v", mixed.Code, mixed.Body.String(), mixed.Header(), sortable.Code, sortable.Body.String(), sortable.Header())
	}
	if n := f.count(`SELECT count(*) FROM problem_folder_items WHERE problem_id=$1`, p2); n != 0 {
		t.Fatalf("placements of bob's problem after the mixed batch: %d", n)
	}
	expect("alice", item{"problem", p1, fa2.ID}, item{"post", post1, fa.ID})
	f.request("PUT", path, "alice", place(nil, article(post1)), 204)

	// A folder of someone else is not there.
	f.fail("PUT", path, "alice", place(fb.ID, problem(p1)), 404, "not_found")
	f.fail("PUT", path, "alice", place(missing, problem(p1)), 404, "not_found")
	expect("alice", item{"problem", p1, fa2.ID})

	// A tester sorts the problem into a folder of their own, apart from the author.
	f.request("PUT", path, "bob", place(fb.ID, problem(p1)), 204)
	if n := rows("problem_folder_items", "bob"); n != 0 {
		t.Fatalf("bob sorted alice's problem before testing it: %d", n)
	}
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'bob')`, p1)
	f.request("PUT", path, "bob", place(fb.ID, problem(p1)), 204)
	expect("bob", item{"problem", p1, fb.ID})
	expect("alice", item{"problem", p1, fa2.ID})
	// Leaving the testers hides the placement, which comes back in the same folder with the right.
	f.exec(`DELETE FROM problem_testers WHERE problem_id=$1 AND owner_id='bob'`, p1)
	expect("bob")
	f.exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'bob')`, p1)
	expect("bob", item{"problem", p1, fb.ID})

	// A placement goes with its folder, and the problem stays.
	f.request("DELETE", "/my/folders/"+fa2.ID, "alice", nil, 204)
	if n := rows("problem_folder_items", "alice"); n != 0 {
		t.Fatalf("alice's placements after deleting the folder: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM problem_drafts WHERE id=$1`, p1); n != 1 {
		t.Fatalf("problem after deleting its folder: %d", n)
	}
	expect("alice")
	// A placement goes with its problem or post, for everyone who sorted it.
	f.request("PUT", path, "alice", place(fa.ID, problem(p1), article(post1)), 204)
	if n := f.count(`SELECT count(*) FROM problem_folder_items WHERE problem_id=$1`, p1); n != 2 {
		t.Fatalf("placements of the problem by alice and bob: %d", n)
	}
	if n := f.count(`SELECT count(*) FROM post_folder_items WHERE post_id=$1`, post1); n != 1 {
		t.Fatalf("placements of the post: %d", n)
	}
	f.exec(`DELETE FROM problem_drafts WHERE id=$1`, p1)
	if n := f.count(`SELECT count(*) FROM problem_folder_items WHERE problem_id=$1`, p1); n != 0 {
		t.Fatalf("placements of a deleted problem: %d", n)
	}
	f.exec(`DELETE FROM blog_posts WHERE id=$1`, post1)
	if n := f.count(`SELECT count(*) FROM post_folder_items WHERE post_id=$1`, post1); n != 0 {
		t.Fatalf("placements of a deleted post: %d", n)
	}

	// One placement takes 1 to 100 problems and posts with their kinds and ids, into a named folder or none.
	many := make([]map[string]string, folders.MaxItems+1)
	for i := range many {
		many[i] = problem(fmt.Sprintf("f4000000-0000-4000-8000-%012d", i))
	}
	f.request("PUT", path, "alice", place(fa.ID, many[:folders.MaxItems]...), 204)
	for _, body := range []any{
		place(fa.ID, many...),
		map[string]any{"folderId": fa.ID, "items": []any{}},
		map[string]any{"folderId": fa.ID},
		map[string]any{"items": []any{problem(p2)}},
		place(fa.ID, map[string]string{"kind": "contest", "id": p2}),
		place(fa.ID, problem("p2")),
		place(fa.ID, problem(strings.ToUpper(p2))),
		place("top", problem(p2)),
		map[string]any{"folderId": fa.ID, "items": []any{map[string]any{"kind": "problem", "id": p2, "folderId": fb.ID}}},
	} {
		f.fail("PUT", path, "alice", body, 400, "invalid_request")
	}
	if n := f.count(`SELECT (SELECT count(*) FROM problem_folder_items)+(SELECT count(*) FROM post_folder_items)`); n != 0 {
		t.Fatalf("placements left: %d", n)
	}
}

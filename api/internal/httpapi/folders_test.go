package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"judge/api/internal/folders"
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

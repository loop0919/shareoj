package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"judge/api/internal/problems"
)

func imagePNG(t *testing.T, width int) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, width, 2))
	img.Set(0, 0, color.NRGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestContentImageValidation(t *testing.T) {
	data := imagePNG(t, 4)
	clean, media, err := cleanContentImage(append(data, []byte("trailing metadata")...))
	if err != nil || media != "image/png" || bytes.Contains(clean, []byte("trailing metadata")) {
		t.Fatalf("clean: %s %v", media, err)
	}
	var photo bytes.Buffer
	if err := jpeg.Encode(&photo, image.NewRGBA(image.Rect(0, 0, 8, 8)), nil); err != nil {
		t.Fatal(err)
	}
	if _, media, err := cleanContentImage(photo.Bytes()); err != nil || media != "image/jpeg" {
		t.Fatalf("jpeg: %s %v", media, err)
	}
	for _, data := range [][]byte{nil, []byte(`<svg xmlns="http://www.w3.org/2000/svg"/>`), imagePNG(t, 2049), make([]byte, maxContentImage+1), data[:len(data)/2]} {
		if _, _, err := cleanContentImage(data); err == nil {
			t.Fatal("accepted invalid image")
		}
	}
}

// Called by the PostgreSQL integration fixture after Alice and Bob are registered.
func testContentImages(t *testing.T, request func(string, string, string, string) *httptest.ResponseRecorder, store *problems.Store) {
	t.Helper()
	ctx := context.Background()
	body := `{"data":"` + base64.StdEncoding.EncodeToString(imagePNG(t, 4)) + `"}`
	check := func(method, path, owner, body string, status int) *httptest.ResponseRecorder {
		t.Helper()
		w := request(method, path, owner, body)
		if w.Code != status {
			t.Fatalf("image %s %s (%s): %d %s", method, path, owner, w.Code, w.Body.String())
		}
		return w
	}
	check("POST", "/my/images", "", body, 401)
	check("POST", "/my/images", "alice", `{"data":"broken"}`, 400)
	check("POST", "/my/images", "alice", `{"data":"`+strings.Repeat("A", 740000)+`"}`, 413)
	result := check("POST", "/my/images", "alice", body, 201)
	var uploaded struct {
		ID  string `json:"id"`
		URL string `json:"url"`
	}
	if err := json.Unmarshal(result.Body.Bytes(), &uploaded); err != nil || uploaded.ID == "" {
		t.Fatal(err)
	}
	id := uploaded.ID
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			w := request("POST", "/my/images", "alice", body)
			if w.Code != 200 || !strings.Contains(w.Body.String(), id) {
				t.Errorf("dedup: %d %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	check("GET", "/images/"+id, "", "", 404)
	check("GET", "/my/images/"+id, "bob", "", 404)
	check("DELETE", "/my/images/"+id, "bob", "", 404)
	w := check("GET", "/my/images/"+id, "alice", "", 200)
	if w.Header().Get("Content-Type") != "image/png" || w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal(w.Header())
	}
	if _, err := png.Decode(w.Body); err != nil {
		t.Fatal(err)
	}
	check("GET", "/my/images?offset=-1", "alice", "", 400)
	if w := check("GET", "/my/images", "alice", "", 200); !strings.Contains(w.Body.String(), id) {
		t.Fatal(w.Body.String())
	}
	postID := "88888888-8888-4888-8888-888888888888"
	problemID := "99999999-8888-4888-8888-888888888888"
	contestID := "77777777-8888-4888-8888-888888888888"
	url := uploaded.URL
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := store.Pool().Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	// Copying someone else's private URL to a public article grants no access.
	exec(`INSERT INTO blog_posts(id,owner_id,title,markdown,published_title,published_markdown) VALUES($1,'bob','test',$2,'test',$2)`, postID, url)
	check("GET", "/images/"+id, "", "", 404)
	exec(`UPDATE blog_posts SET owner_id='alice',published_markdown=NULL WHERE id=$1`, postID)
	check("GET", "/images/"+id, "", "", 404)
	check("DELETE", "/my/images/"+id, "alice", "", 409)
	exec(`UPDATE blog_posts SET published_markdown=markdown WHERE id=$1`, postID)
	check("GET", "/images/"+id, "", "", 200)
	exec(`UPDATE blog_posts SET markdown='' WHERE id=$1`, postID)
	check("DELETE", "/my/images/"+id, "alice", "", 409)
	exec(`DELETE FROM blog_posts WHERE id=$1`, postID)
	// A contest statement is visible at start; its editorial only at end.
	exec(`INSERT INTO problem_drafts(id,owner_id,draft) VALUES($1,'alice',jsonb_build_object('markdown',$2::text))`, problemID, url)
	exec(`INSERT INTO contests(id,owner_id,title,description,starts_at,ends_at) VALUES($1,'alice','test','',now()+interval '1 hour',now()+interval '2 hours')`, contestID)
	exec(`INSERT INTO contest_problems(contest_id,problem_id,position,points,draft,problem_version) VALUES($1,$2,0,100,jsonb_build_object('markdown',$3::text),1)`, contestID, problemID, url)
	check("GET", "/images/"+id, "", "", 404)
	exec(`INSERT INTO problem_testers(problem_id,owner_id) VALUES($1,'bob')`, problemID)
	check("GET", "/my/images/"+id, "bob", "", 200)
	exec(`UPDATE contests SET starts_at=now()-interval '1 hour' WHERE id=$1`, contestID)
	check("GET", "/images/"+id, "", "", 200)
	exec(`UPDATE contest_problems SET draft=jsonb_build_object('editorial',$2::text) WHERE contest_id=$1`, contestID, url)
	check("GET", "/images/"+id, "", "", 404)
	exec(`UPDATE contests SET ends_at=now()-interval '1 minute',released=true WHERE id=$1`, contestID)
	check("GET", "/images/"+id, "", "", 200)
	check("DELETE", "/my/images/"+id, "alice", "", 409)
	exec(`DELETE FROM contest_problems WHERE contest_id=$1`, contestID)
	exec(`DELETE FROM contests WHERE id=$1`, contestID)
	exec(`DELETE FROM problem_drafts WHERE id=$1`, problemID)
	check("GET", "/images/"+id, "", "", 404)
	check("DELETE", "/my/images/"+id, "alice", "", 204)
	check("GET", "/my/images/"+id, "alice", "", 404)
}

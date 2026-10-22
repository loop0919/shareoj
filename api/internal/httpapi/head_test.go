package httpapi

import (
	"encoding/base64"
	"testing"
)

// A HEAD that matches a GET route serves the GET read without a body, even with a JSON body the write would accept.
func TestHeadServesReadPostgres(t *testing.T) {
	f := newTeamTest(t, "alice")
	const (
		p1  = "11111111-1111-4111-8111-111111111111"
		cid = "cccccccc-cccc-4ccc-8ccc-cccccccccccc"
	)
	f.problem(p1)
	f.create(cid, p1, nil)
	for _, tc := range []struct {
		path, owner string
		body        any
	}{
		{"/my/profile", "alice", map[string]any{"handle": "zed", "avatar": "", "version": 1}},
		{"/my/images", "alice", map[string]any{"data": base64.StdEncoding.EncodeToString(imagePNG(t, 4))}},
		{"/my/submissions", "alice", map[string]any{"problemId": p1, "runtime": "cpp17", "source": "x"}},
		{"/my/problems/" + p1 + "/rejudge", "alice", map[string]any{"submissionIds": []string{}}},
		{"/my/contests/" + cid + "/participants", "alice", map[string]any{"onsite": true}},
		{"/my/contests/" + cid + "/clarifications", "alice", map[string]any{"problemId": p1, "question": "HEAD must not write"}},
		{"/my/problems/" + p1, "alice", nil},
		{"/my/featured", "alice", nil},
		{"/contests/" + cid, "", nil},
	} {
		get := f.do(f.newRequest("GET", tc.path, tc.owner, nil))
		head := f.do(f.newRequest("HEAD", tc.path, tc.owner, tc.body))
		if head.Code != get.Code || head.Body.Len() != 0 || head.Header().Get("Content-Type") != get.Header().Get("Content-Type") {
			t.Errorf("HEAD %s: got %d %q %q, want %d %q with no body", tc.path, head.Code, head.Header().Get("Content-Type"), head.Body.String(), get.Code, get.Header().Get("Content-Type"))
		}
	}
	for _, query := range []string{`SELECT count(*) FROM submissions`, `SELECT count(*) FROM content_images`, `SELECT count(*) FROM user_profiles WHERE handle='zed'`} {
		if n := f.count(query); n != 0 {
			t.Errorf("%s: %d, want 0", query, n)
		}
	}
}

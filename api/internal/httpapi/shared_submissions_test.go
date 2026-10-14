package httpapi

import (
	"strings"
	"testing"
)

func TestSharedSubmissionVisibilityPostgres(t *testing.T) {
	f := newFeaturedTest(t)
	p := f.draft("alice", 3)
	f.request("PUT", "/my/problems/"+p.ID+"/publication", "alice", map[string]any{"version": p.Version, "publish": true}, 200)
	for _, tc := range []struct {
		job  string
		code int
	}{
		{`{"privateDraft":false}`, 200},
		{`{"privateDraft":true}`, 404},
		{`{}`, 404},
		{`{"privateDraft":false,"easyTest":true}`, 404},
		{`{"privateDraft":false,"generate":true}`, 404},
		{`{"privateDraft":false,"validate":true}`, 404},
	} {
		sid := newSubmissionID()
		f.exec(`INSERT INTO submissions(id,owner_id,problem_id,problem_version,problem_title,runtime,source,job,status,result)
          VALUES($1,'bob',$2,2,'Shared','cpp17','public source',$3,'DONE',
          '{"verdict":"WA","passed":0,"total":1,"checkerLog":"SECRET","compileLog":"SECRET","cases":[{"name":"test","verdict":"WA","output":"SECRET"}]}')`, sid, p.ID, tc.job)
		for _, viewer := range []string{"", "carol"} {
			path := "/submissions/" + sid
			if viewer != "" {
				path = "/my" + path + "/public"
			}
			body := f.request("GET", path, viewer, nil, tc.code)
			if strings.Contains(body, "SECRET") || (tc.code == 200 && (!strings.Contains(body, p.ID) || !strings.Contains(body, "public source"))) {
				t.Fatal("wrong shared submission content", body)
			}
		}
		f.request("GET", "/my/submissions/"+sid, "bob", nil, 200)
	}
	f.request("GET", "/submissions/not-an-id", "", nil, 404)
	f.request("GET", "/submissions/"+newSubmissionID(), "", nil, 404)
}

package testfiles

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func TestLocalSignedFiles(t *testing.T) {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)
	defer server.Close()
	store, handler, err := NewLocal(nil, t.TempDir(), server.URL+"/local-test-files", "http://localhost:3000")
	if err != nil {
		t.Fatal(err)
	}
	mux.Handle("/local-test-files", handler)
	key := "test-files/owner/problem/file"
	put, _ := store.presign.PresignPutObject(context.Background(), &s3.PutObjectInput{Key: &key})
	request := func(method, address, body, origin string, want int) string {
		t.Helper()
		r, _ := http.NewRequest(method, address, strings.NewReader(body))
		r.Header.Set("Origin", origin)
		response, err := http.DefaultClient.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, _ := io.ReadAll(response.Body)
		if response.StatusCode != want {
			t.Fatalf("%s: %d %s", method, response.StatusCode, data)
		}
		if origin != "" && want == 200 && response.Header.Get("Access-Control-Allow-Origin") != origin {
			t.Fatal("CORS")
		}
		return string(data)
	}
	request("PUT", put.URL, "hello\n", "http://localhost:3000", 200)
	request("PUT", put.URL, "hello\n", "http://127.0.0.1:3000", 200)
	request("PUT", put.URL, "changed", "", 400)
	get, _ := store.presign.PresignGetObject(context.Background(), &s3.GetObjectInput{Key: &key, VersionId: aws.String("local-v1")})
	if request("GET", get.URL, "", "http://localhost:3000", 200) != "hello\n" {
		t.Fatal("content changed")
	}
	request("PUT", get.URL, "changed", "", 403)
	request("GET", get.URL, "", "https://other.example", 403)
	for _, field := range []string{"key", "expires", "signature"} {
		u, _ := url.Parse(get.URL)
		q := u.Query()
		q.Set(field, "invalid")
		u.RawQuery = q.Encode()
		request("GET", u.String(), "", "", 403)
	}
	local := store.objects.(*LocalObjects)
	file, err := WriteGenerated(context.Background(), local, "local", GenerationPrefix("alice", "11111111-1111-4111-8111-111111111111"), []byte("123\n"))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := ReadFile(context.Background(), local, "local", file); err != nil || got != "123\n" {
		t.Fatal(got, err)
	}
	file.Version = "unknown"
	if _, err := ReadFile(context.Background(), local, "local", file); err == nil {
		t.Fatal("accepted unknown version")
	}
}

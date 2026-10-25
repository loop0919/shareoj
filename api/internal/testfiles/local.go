package testfiles

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/jackc/pgx/v5/pgxpool"
)

// LocalObjects preserves the immutable-object contract without AWS in make dev.
type LocalObjects struct{ dir string }

func NewLocalObjects(dir string) (*LocalObjects, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	return &LocalObjects{dir: dir}, nil
}
func (l *LocalObjects) path(key string) string {
	sum := sha256.Sum256([]byte(key))
	return filepath.Join(l.dir, hex.EncodeToString(sum[:]))
}
func (l *LocalObjects) PutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.Options)) (*s3.PutObjectOutput, error) {
	data, err := io.ReadAll(io.LimitReader(in.Body, MaxSize+1))
	if err != nil || len(data) > MaxSize {
		return nil, ErrInvalid
	}
	// Link the fully written temporary file atomically: readers never see partial uploads.
	f, err := os.CreateTemp(l.dir, ".upload-")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.Remove(f.Name()) }()
	_, err = f.Write(data)
	closeErr := f.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	destination := l.path(aws.ToString(in.Key))
	if err = os.Link(f.Name(), destination); err != nil {
		previous, readErr := os.ReadFile(destination)
		if !errors.Is(err, os.ErrExist) || readErr != nil || !bytes.Equal(previous, data) {
			return nil, ErrInvalid
		}
	}
	return &s3.PutObjectOutput{VersionId: aws.String("local-v1")}, nil
}
func (l *LocalObjects) GetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.Options)) (*s3.GetObjectOutput, error) {
	if in.VersionId != nil && *in.VersionId != "local-v1" {
		return nil, ErrInvalid
	}
	f, err := os.Open(l.path(aws.ToString(in.Key)))
	if err != nil {
		return nil, err
	}
	return &s3.GetObjectOutput{Body: f, VersionId: aws.String("local-v1")}, nil
}
func (l *LocalObjects) PutObjectTagging(context.Context, *s3.PutObjectTaggingInput, ...func(*s3.Options)) (*s3.PutObjectTaggingOutput, error) {
	// Readiness is already persisted in test_files; local files have no pending-object lifecycle.
	return &s3.PutObjectTaggingOutput{}, nil
}

type localURLs struct {
	objects      *LocalObjects
	base, origin string
	secret       [32]byte
}

// NewLocal uses signed URLs like S3, so uploads, generated files and public samples use the same UI.
func NewLocal(pool *pgxpool.Pool, dir, base, origin string) (*Store, http.Handler, error) {
	u, err := url.Parse(base)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" || u.Fragment != "" {
		return nil, nil, ErrInvalid
	}
	objects, err := NewLocalObjects(dir)
	if err != nil {
		return nil, nil, err
	}
	urls := &localURLs{objects: objects, base: base, origin: origin}
	if _, err = rand.Read(urls.secret[:]); err != nil {
		return nil, nil, err
	}
	return &Store{pool: pool, objects: objects, presign: urls, bucket: "local"}, urls, nil
}
func (l *localURLs) signature(method, key, expires string) string {
	mac := hmac.New(sha256.New, l.secret[:])
	_, _ = fmt.Fprintf(mac, "%s\n%s\n%s", method, key, expires)
	return hex.EncodeToString(mac.Sum(nil))
}
func (l *localURLs) signed(method, key string) *v4Request {
	expires := strconv.FormatInt(time.Now().Add(10*time.Minute).Unix(), 10)
	query := url.Values{"key": {key}, "expires": {expires}, "signature": {l.signature(method, key, expires)}}
	return &v4Request{URL: l.base + "?" + query.Encode(), Method: method}
}
func (l *localURLs) PresignPutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.PresignOptions)) (*v4Request, error) {
	return l.signed("PUT", aws.ToString(in.Key)), nil
}
func (l *localURLs) PresignGetObject(_ context.Context, in *s3.GetObjectInput, _ ...func(*s3.PresignOptions)) (*v4Request, error) {
	return l.signed("GET", aws.ToString(in.Key)), nil
}
func (l *localURLs) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if origin := r.Header.Get("Origin"); origin != "" {
		if origin != l.origin && origin != strings.Replace(l.origin, "://localhost:", "://127.0.0.1:", 1) {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", origin)
		w.Header().Set("Vary", "Origin")
	}
	if r.Method == "OPTIONS" {
		w.Header().Set("Access-Control-Allow-Methods", "GET, PUT")
		w.Header().Set("Access-Control-Allow-Headers", "content-type, x-amz-checksum-sha256, x-amz-tagging")
		w.WriteHeader(204)
		return
	}
	q := r.URL.Query()
	key, expires := q.Get("key"), q.Get("expires")
	deadline, err := strconv.ParseInt(expires, 10, 64)
	signature, decodeErr := hex.DecodeString(q.Get("signature"))
	expected, _ := hex.DecodeString(l.signature(r.Method, key, expires))
	if (r.Method != "GET" && r.Method != "PUT") || err != nil || decodeErr != nil || deadline <= time.Now().Unix() || deadline > time.Now().Add(10*time.Minute).Unix() || !strings.HasPrefix(key, "test-files/") || !hmac.Equal(signature, expected) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	if r.Method == "PUT" {
		r.Body = http.MaxBytesReader(w, r.Body, MaxSize)
		_, err = l.objects.PutObject(r.Context(), &s3.PutObjectInput{Key: &key, Body: r.Body})
		if err != nil {
			http.Error(w, "invalid upload", 400)
			return
		}
		w.WriteHeader(200)
		return
	}
	result, err := l.objects.GetObject(r.Context(), &s3.GetObjectInput{Key: &key})
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer func() { _ = result.Body.Close() }()
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = io.Copy(w, result.Body)
}

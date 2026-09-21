// Package testfiles stores validated, immutable test input and output files in S3.
package testfiles

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"judge/api/internal/problems"
)

const MaxSize = 16 << 20

var (
	ErrNotFound    = errors.New("test file not found")
	ErrInvalid     = errors.New("invalid test file")
	ErrUnavailable = errors.New("test file storage unavailable")
)

type Upload struct {
	ID      string            `json:"id"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers"`
}

type Download struct {
	URL    string `json:"url"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type objects interface {
	GetObject(context.Context, *s3.GetObjectInput, ...func(*s3.Options)) (*s3.GetObjectOutput, error)
	PutObjectTagging(context.Context, *s3.PutObjectTaggingInput, ...func(*s3.Options)) (*s3.PutObjectTaggingOutput, error)
}

type presigner interface {
	PresignPutObject(context.Context, *s3.PutObjectInput, ...func(*s3.PresignOptions)) (*v4Request, error)
	PresignGetObject(context.Context, *s3.GetObjectInput, ...func(*s3.PresignOptions)) (*v4Request, error)
}

// v4Request is the subset shared by the SDK's presigned request result.
type v4Request struct {
	URL          string
	Method       string
	SignedHeader http.Header
}

type sdkPresigner struct{ client *s3.PresignClient }

func (p sdkPresigner) PresignPutObject(ctx context.Context, in *s3.PutObjectInput, opt ...func(*s3.PresignOptions)) (*v4Request, error) {
	r, err := p.client.PresignPutObject(ctx, in, opt...)
	if err != nil {
		return nil, err
	}
	return &v4Request{URL: r.URL, Method: r.Method, SignedHeader: r.SignedHeader}, nil
}

func (p sdkPresigner) PresignGetObject(ctx context.Context, in *s3.GetObjectInput, opt ...func(*s3.PresignOptions)) (*v4Request, error) {
	r, err := p.client.PresignGetObject(ctx, in, opt...)
	if err != nil {
		return nil, err
	}
	return &v4Request{URL: r.URL, Method: r.Method, SignedHeader: r.SignedHeader}, nil
}

type Store struct {
	pool    *pgxpool.Pool
	objects objects
	presign presigner
	bucket  string
}

func New(pool *pgxpool.Pool, client *s3.Client, bucket string) *Store {
	return &Store{pool: pool, objects: client, presign: sdkPresigner{s3.NewPresignClient(client)}, bucket: bucket}
}

func (s *Store) Begin(ctx context.Context, owner, problemID, id string, size int64, digest string) (Upload, error) {
	if size <= 0 || size > MaxSize || len(digest) != 64 {
		return Upload{}, ErrInvalid
	}
	rawDigest, err := hex.DecodeString(digest)
	if err != nil {
		return Upload{}, ErrInvalid
	}
	ownerHash := sha256.Sum256([]byte(owner))
	key := fmt.Sprintf("test-files/%x/%s/%s", ownerHash[:16], problemID, id)
	if _, err = s.pool.Exec(ctx, `INSERT INTO test_files(id,owner_id,problem_id,object_key,sha256,size) VALUES ($1,$2,$3,$4,$5,$6)`, id, owner, problemID, key, digest, size); err != nil {
		return Upload{}, ErrUnavailable
	}
	checksum := base64.StdEncoding.EncodeToString(rawDigest)
	contentType, tagging := "text/plain; charset=utf-8", "status=pending"
	request, err := s.presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), ContentLength: aws.Int64(size),
		ContentType: aws.String(contentType), ChecksumSHA256: aws.String(checksum), Tagging: aws.String(tagging),
	}, func(options *s3.PresignOptions) { options.Expires = 10 * time.Minute })
	if err != nil {
		_, _ = s.pool.Exec(ctx, `DELETE FROM test_files WHERE id=$1 AND NOT ready`, id)
		return Upload{}, ErrUnavailable
	}
	return Upload{ID: id, URL: request.URL, Headers: map[string]string{
		"content-type": contentType, "x-amz-checksum-sha256": checksum, "x-amz-tagging": tagging,
	}}, nil
}

func (s *Store) Complete(ctx context.Context, owner, problemID, id string) (problems.TestFile, error) {
	var key, version, digest string
	var uploadVersion *string
	var size int64
	var ready bool
	err := s.pool.QueryRow(ctx, `SELECT object_key,COALESCE(version_id,''),sha256,size,ready,upload_version_id FROM test_files WHERE id=$1 AND (owner_id=$2 OR can_manage_problem(problem_id,$2)) AND problem_id=$3`, id, owner, problemID).Scan(&key, &version, &digest, &size, &ready, &uploadVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return problems.TestFile{}, ErrNotFound
	}
	if err != nil {
		return problems.TestFile{}, ErrUnavailable
	}
	ref := problems.TestFile{ID: id, Size: size, SHA256: digest}
	if ready {
		return ref, nil
	}
	result, err := s.objects.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), VersionId: uploadVersion, ChecksumMode: types.ChecksumModeEnabled})
	if err != nil {
		return problems.TestFile{}, ErrInvalid
	}
	defer func() { _ = result.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(result.Body, MaxSize+1))
	if err != nil || int64(len(data)) != size || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 || result.VersionId == nil || (uploadVersion != nil && *result.VersionId != *uploadVersion) {
		return problems.TestFile{}, ErrInvalid
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != digest {
		return problems.TestFile{}, ErrInvalid
	}
	version = *result.VersionId
	_, err = s.objects.PutObjectTagging(ctx, &s3.PutObjectTaggingInput{
		Bucket: aws.String(s.bucket), Key: aws.String(key), VersionId: aws.String(version),
		Tagging: &types.Tagging{TagSet: []types.Tag{{Key: aws.String("status"), Value: aws.String("ready")}}},
	})
	if err != nil {
		return problems.TestFile{}, ErrUnavailable
	}
	command, err := s.pool.Exec(ctx, `UPDATE test_files SET version_id=$4,ready=true WHERE id=$1 AND (owner_id=$2 OR can_manage_problem(problem_id,$2)) AND problem_id=$3 AND NOT ready`, id, owner, problemID, version)
	if err != nil || command.RowsAffected() != 1 {
		return problems.TestFile{}, ErrUnavailable
	}
	return ref, nil
}

func (s *Store) Download(ctx context.Context, owner, problemID, id string) (Download, error) {
	return s.download(ctx, s.pool.QueryRow(ctx, `SELECT object_key,version_id,sha256,size FROM test_files WHERE id=$1 AND (owner_id=$2 OR can_manage_problem(problem_id,$2)) AND problem_id=$3 AND ready`, id, owner, problemID))
}

// PublicSampleDownload only signs files referenced by a currently published sample.
func (s *Store) PublicSampleDownload(ctx context.Context, problemID, id string) (Download, error) {
	return s.download(ctx, s.pool.QueryRow(ctx, `SELECT f.object_key,f.version_id,f.sha256,f.size FROM test_files f
 JOIN problem_drafts d ON d.id=f.problem_id
 WHERE f.id=$1 AND f.problem_id=$2 AND f.ready AND EXISTS (
 SELECT 1 FROM jsonb_array_elements(d.published_draft->'testCases') c
 WHERE c->'isSample'='true'::jsonb AND (c->'inputFile'->>'id'=f.id::text OR c->'outputFile'->>'id'=f.id::text))`, id, problemID))
}

func (s *Store) download(ctx context.Context, row pgx.Row) (Download, error) {
	var key, version, digest string
	var size int64
	err := row.Scan(&key, &version, &digest, &size)
	if errors.Is(err, pgx.ErrNoRows) {
		return Download{}, ErrNotFound
	}
	if err != nil {
		return Download{}, ErrUnavailable
	}
	request, err := s.presign.PresignGetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(s.bucket), Key: aws.String(key), VersionId: aws.String(version)}, func(options *s3.PresignOptions) { options.Expires = 10 * time.Minute })
	if err != nil {
		return Download{}, ErrUnavailable
	}
	return Download{URL: request.URL, Size: size, SHA256: digest}, nil
}

func IsValidDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

package testfiles

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"
	"regexp"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"judge/api/internal/problems"
)

func GenerationPrefix(owner, problem string) string {
	hash := sha256.Sum256([]byte(owner))
	return fmt.Sprintf("test-files/%x/%s/generated/", hash[:16], problem)
}

var generatedID = regexp.MustCompile(`^[a-f0-9]{8}-[a-f0-9]{4}-4[a-f0-9]{3}-[89ab][a-f0-9]{3}-[a-f0-9]{12}$`)

func ValidGeneratedFile(file *problems.TestFile) bool {
	return file != nil && generatedID.MatchString(file.ID) && file.Size > 0 && file.Size <= MaxSize && IsValidDigest(file.SHA256) && len(file.Version) > 0 && len(file.Version) <= 1024 && len(file.Key) <= 512
}

// WriteGenerated stores one immutable, pending upload. Complete verifies its bytes before use.
func WriteGenerated(ctx context.Context, client *s3.Client, bucket, prefix string, data []byte) (*problems.TestFile, error) {
	if bucket == "" || len(data) == 0 || len(data) > MaxSize || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return nil, ErrInvalid
	}
	token := make([]byte, 16)
	if _, err := rand.Read(token); err != nil {
		return nil, err
	}
	token[6] = (token[6] & 15) | 64
	token[8] = (token[8] & 63) | 128
	id := fmt.Sprintf("%x-%x-%x-%x-%x", token[:4], token[4:6], token[6:8], token[8:10], token[10:])
	hash := sha256.Sum256(data)
	key := prefix + id
	result, err := client.PutObject(ctx, &s3.PutObjectInput{Bucket: aws.String(bucket), Key: aws.String(key), Body: bytes.NewReader(data), ContentType: aws.String("text/plain; charset=utf-8"), Tagging: aws.String("status=pending"), ChecksumSHA256: aws.String(base64.StdEncoding.EncodeToString(hash[:]))})
	if err != nil {
		return nil, err
	}
	if result.VersionId == nil || *result.VersionId == "" {
		return nil, ErrInvalid
	}
	return &problems.TestFile{ID: id, Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:]), Key: key, Version: *result.VersionId}, nil
}

func ReadFile(ctx context.Context, client *s3.Client, bucket string, file *problems.TestFile) (string, error) {
	if file.Size <= 0 || file.Size > MaxSize || file.Version == "" {
		return "", ErrInvalid
	}
	result, err := client.GetObject(ctx, &s3.GetObjectInput{Bucket: aws.String(bucket), Key: aws.String(file.Key), VersionId: aws.String(file.Version)})
	if err != nil {
		return "", err
	}
	defer func() { _ = result.Body.Close() }()
	data, err := io.ReadAll(io.LimitReader(result.Body, MaxSize+1))
	sum := sha256.Sum256(data)
	if err != nil || int64(len(data)) != file.Size || hex.EncodeToString(sum[:]) != file.SHA256 || !utf8.Valid(data) || bytes.IndexByte(data, 0) >= 0 {
		return "", ErrInvalid
	}
	return string(data), nil
}

package httpapi

import (
	_ "embed"
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"judge/api/internal/contests"
	"judge/api/internal/posts"
	"judge/api/internal/problems"
	"judge/api/internal/profiles"
	"judge/api/internal/submissions"
)

// Aliases keep the original response types available beside route-local handlers.
type (
	publicProblemResponse    = problems.PublicProblem
	publicProblemPage        = cursorResponse[problems.PublicProblem]
	publicPostResponse       = posts.Post
	publicPostPage           = cursorResponse[posts.Post]
	publicContestResponse    = contests.Contest
	publicContestPage        = offsetResponse[contests.Contest]
	publicStandingsResponse  = []contests.Standing
	publicSubmissionResponse = submissions.Submission
	publicSubmissionPage     = submissions.ContestSubmissionList
)

// These envelopes are used by both the handlers and the schema generator.
type itemsResponse[T any] struct {
	Items []T `json:"items"`
}

type cursorResponse[T any] struct {
	Items      []T    `json:"items"`
	NextCursor string `json:"nextCursor" doc:"次ページのcursor。空文字なら最後のページ。"`
}

type offsetResponse[T any] struct {
	Items   []T  `json:"items"`
	HasMore bool `json:"hasMore" doc:"trueならoffsetを取得件数分増やして次ページを取得。"`
}

type healthResponse struct {
	Status string `json:"status" example:"ok"`
}

type runtimesResponse struct {
	Items       []submissions.Runtime `json:"items"`
	Maintenance bool                  `json:"maintenance"`
}

type publicProfileResponse struct {
	Handle    string            `json:"handle"`
	Avatar    string            `json:"avatar"`
	CreatedAt time.Time         `json:"createdAt"`
	Accounts  profiles.Accounts `json:"accounts"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Route metadata supplies semantics; Huma derives response schemas from Go types.
type publicOperation struct {
	Summary     string
	Description string
	Response    any
	Parameters  []*huma.Param
	Image       bool
}

func catalogueParameters() []*huma.Param {
	return []*huma.Param{
		{Name: "cursor", In: "query", Description: "前の応答のnextCursor。省略すると先頭から最大50件。", Schema: &huma.Schema{Type: "string", MaxLength: intPointer(512)}},
		{Name: "author", In: "query", Description: "作者のユーザーIDで絞り込み。", Schema: &huma.Schema{Type: "string", Pattern: userHandle.String()}},
	}
}

func offsetParameters() []*huma.Param {
	return []*huma.Param{{Name: "offset", In: "query", Description: "先頭からスキップする件数。1ページ最大50件。", Schema: &huma.Schema{Type: "integer", Minimum: floatPointer(0), Maximum: floatPointer(1000000), Default: 0}}}
}

func intPointer(v int) *int           { return &v }
func floatPointer(v float64) *float64 { return &v }

func newPublicDocumentation() *huma.OpenAPI {
	return &huma.OpenAPI{
		OpenAPI:    "3.1.0",
		Info:       &huma.Info{Title: "ShareOJ Public API", Version: "1.0.0", Description: "認証不要の公開・参照APIです。問題、サンプルケース、コンテスト、公開提出、記事、プロフィールを取得できます。\n\nURLには /api プレフィックスを付けません。公開されていないデータは取得できません。エラー本文は `{\"error\":\"コード\"}` 形式です。仕様はルート登録とGoのレスポンス型から起動時に自動生成します。"},
		Servers:    []*huma.Server{{URL: "/", Description: "このドキュメントを配信しているAPI"}},
		Components: &huma.Components{Schemas: huma.NewMapRegistry("#/components/schemas/", huma.DefaultSchemaNamer)},
	}
}

func documentPublicRoute(spec *huma.OpenAPI, pattern string, doc publicOperation) {
	method, path, _ := strings.Cut(pattern, " ")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	op := &huma.Operation{
		Method: method, Path: path, OperationID: strings.ToLower(method) + "-" + strings.NewReplacer("/", "-", "{", "", "}", "").Replace(strings.Trim(path, "/")),
		Summary: doc.Summary, Description: doc.Description, Tags: []string{parts[0]}, Parameters: doc.Parameters,
		Responses: map[string]*huma.Response{},
	}
	for _, part := range parts {
		if !strings.HasPrefix(part, "{") {
			continue
		}
		name := strings.Trim(part, "{}")
		schema := &huma.Schema{Type: "string", Format: "uuid", Pattern: problemID.String()}
		if name == "handle" {
			schema = &huma.Schema{Type: "string", Pattern: userHandle.String()}
		}
		op.Parameters = append(op.Parameters, &huma.Param{Name: name, In: "path", Required: true, Schema: schema})
	}
	if doc.Image {
		op.Responses["200"] = &huma.Response{Description: "公開コンテンツで参照される画像", Content: map[string]*huma.MediaType{
			"image/png":  {Schema: &huma.Schema{Type: "string", Format: "binary"}},
			"image/jpeg": {Schema: &huma.Schema{Type: "string", Format: "binary"}},
		}}
	} else {
		op.Responses["200"] = &huma.Response{Description: "成功", Content: map[string]*huma.MediaType{
			"application/json": {Schema: spec.Components.Schemas.Schema(reflect.TypeOf(doc.Response), true, "")},
		}}
	}
	if path != "/health" {
		for _, status := range []string{"400", "404", "503"} {
			op.Responses[status] = &huma.Response{Description: map[string]string{"400": "パラメータが不正", "404": "存在しない、または公開されていない", "503": "データベースまたはストレージを利用できない"}[status], Content: map[string]*huma.MediaType{
				"application/json": {Schema: spec.Components.Schemas.Schema(reflect.TypeFor[errorResponse](), true, "")},
			}}
		}
	}
	spec.AddOperation(op)
}

//go:embed swagger.html
var swaggerHTML []byte

func registerDocumentation(mux *http.ServeMux, spec *huma.OpenAPI) {
	// Serialize once after registration. Requests never mutate the schema registry.
	data, err := json.Marshal(spec)
	if err != nil {
		panic(err)
	}
	mux.HandleFunc("GET /openapi.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(data)
	})
	ui := func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		_, _ = w.Write(swaggerHTML)
	}
	mux.HandleFunc("GET /docs", ui)
	mux.HandleFunc("GET /docs/{$}", ui)
}

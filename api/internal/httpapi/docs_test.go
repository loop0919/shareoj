package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"judge/api/internal/contests"
)

func TestPublicDocumentation(t *testing.T) {
	// Documentation must work without database access, including contest release.
	mux := newHandler(AuthConfig{}, handlerDependencies{Contests: &contests.Store{}}).(*http.ServeMux)
	for _, path := range []string{"/docs", "/docs/", "/openapi.json"} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 200 || w.Header().Get("Cache-Control") != "no-cache" {
			t.Fatalf("%s: %d %s", path, w.Code, w.Body.String())
		}
		if path != "/openapi.json" {
			if !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || !strings.Contains(w.Body.String(), "SwaggerUIBundle") || !strings.Contains(w.Body.String(), "url: '/openapi.json'") {
				t.Fatal("missing Swagger UI")
			}
			continue
		}
		var doc struct {
			OpenAPI string `json:"openapi"`
			Paths   map[string]map[string]struct {
				OperationID string `json:"operationId"`
				Parameters  []struct {
					Name, In string
					Required bool
				} `json:"parameters"`
			} `json:"paths"`
			Components struct {
				Schemas map[string]struct {
					Properties map[string]json.RawMessage `json:"properties"`
				} `json:"schemas"`
			} `json:"components"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &doc); err != nil {
			t.Fatal(err)
		}
		if doc.OpenAPI != "3.1.0" || len(doc.Paths) != 18 {
			t.Fatalf("unexpected public operations: %s %d", doc.OpenAPI, len(doc.Paths))
		}
		seen := map[string]bool{}
		for path, methods := range doc.Paths {
			if strings.HasPrefix(path, "/my/") || strings.HasPrefix(path, "/auth/") || len(methods) != 1 {
				t.Fatal("non-public operation", path)
			}
			op, ok := methods["get"]
			if !ok || op.OperationID == "" || seen[op.OperationID] {
				t.Fatal("invalid operation", path)
			}
			seen[op.OperationID] = true
			concrete := path
			for _, param := range op.Parameters {
				if param.In == "path" {
					if !param.Required {
						t.Fatal("optional path parameter", path)
					}
					concrete = strings.ReplaceAll(concrete, "{"+param.Name+"}", "11111111-1111-4111-8111-111111111111")
				}
			}
			_, pattern := mux.Handler(httptest.NewRequest("GET", concrete, nil))
			if pattern != "GET "+path {
				t.Fatalf("undocumented/mismatched route: %s = %s", path, pattern)
			}
		}
		for _, name := range []string{"PublicSample", "PublicProblem", "Download", "Submission"} {
			if len(doc.Components.Schemas[name].Properties) == 0 {
				t.Fatal("missing generated model", name)
			}
		}
		for _, name := range []string{"Result", "CaseResult"} {
			for _, hidden := range []string{"checkerLog", "compileLog", "sampleDetails", "outputFile", "output"} {
				if _, ok := doc.Components.Schemas[name].Properties[hidden]; ok {
					t.Fatal("private field documented", name, hidden)
				}
			}
		}
	}
	for _, tc := range []struct {
		method, path string
		status       int
	}{{"POST", "/docs", 405}, {"POST", "/openapi.json", 405}, {"GET", "/docs/missing", 404}} {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(tc.method, tc.path, nil))
		if w.Code != tc.status {
			t.Fatalf("%s %s: %d", tc.method, tc.path, w.Code)
		}
	}
}

func TestDocumentationUsesResponseTypes(t *testing.T) {
	spec := newPublicDocumentation()
	documentPublicRoute(spec, "GET /samples/{id}", publicOperation{Summary: "Samples", Response: itemsResponse[publicSample]{}})
	schema := spec.Paths["/samples/{id}"].Get.Responses["200"].Content["application/json"].Schema
	value := itemsResponse[publicSample]{Items: []publicSample{{Name: "sample_1", Input: "3 5\n", Output: "8\n"}}}
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var decoded any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatal(err)
	}
	result := &huma.ValidateResult{}
	huma.Validate(spec.Components.Schemas, schema, huma.NewPathBuffer(nil, 0), huma.ModeReadFromServer, decoded, result)
	if len(result.Errors) != 0 {
		t.Fatal(result.Errors)
	}
	// Adding a JSON field to the response type automatically adds it to the schema.
	generated := spec.Components.Schemas.Schema(reflect.TypeFor[publicSample](), false, "")
	for _, field := range reflect.VisibleFields(reflect.TypeFor[publicSample]()) {
		name := strings.Split(field.Tag.Get("json"), ",")[0]
		if _, ok := generated.Properties[name]; !ok {
			t.Fatal("response field absent from schema", name)
		}
	}
}

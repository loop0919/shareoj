package httpapi

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestJSONRequestLimits(t *testing.T) {
	for _, reader := range []struct {
		name  string
		limit int
		read  func(http.ResponseWriter, *http.Request, any) bool
	}{
		{"auth", 16 << 10, readAuthJSON},
		{"content", 700 << 10, contentJSON},
		{"shared", 2 << 10, func(w http.ResponseWriter, r *http.Request, target any) bool {
			return readJSONBody(w, r, target, 2<<10)
		}},
	} {
		t.Run(reader.name, func(t *testing.T) {
			for _, tc := range []struct {
				name, body, contentType string
				status                  int
			}{
				{"valid", `{"Value":"ok"}`, "application/json; charset=utf-8", 200},
				{"empty", "", "application/json", 400},
				{"unknown field", `{"other":1}`, "application/json", 400},
				{"multiple values", `{"Value":"ok"} {}`, "application/json", 400},
				{"malformed tail", `{"Value":"ok"} !`, "application/json", 400},
				{"wrong media type", `{}`, "text/plain", 415},
				{"mixed case", `{"Value":"ok"}`, "Application/JSON", 200},
				{"space before parameter", `{"Value":"ok"}`, "application/json ; charset=utf-8", 200},
				{"missing media type", `{}`, "", 415},
				{"malformed parameter", `{}`, "application/json;garbage", 415},
				{"other subtype", `{}`, "application/jsonx", 415},
				{"at limit", `{"Value":"ok"}` + strings.Repeat(" ", reader.limit-len(`{"Value":"ok"}`)), "application/json", 200},
				{"oversized value", `{"Value":"` + strings.Repeat("x", reader.limit) + `"}`, "application/json", 413},
				{"oversized tail", `{"Value":"ok"}` + strings.Repeat(" ", reader.limit), "application/json", 413},
			} {
				t.Run(tc.name, func(t *testing.T) {
					w := httptest.NewRecorder()
					r := httptest.NewRequest("POST", "/", strings.NewReader(tc.body))
					r.Header.Set("Content-Type", tc.contentType)
					var input struct{ Value string }
					ok := reader.read(w, r, &input)
					status, value := w.Code, input.Value
					if status != tc.status || ok != (tc.status == 200) || (ok && value != "ok") {
						t.Fatalf("ok=%v status=%d value=%q; want status=%d", ok, status, value, tc.status)
					}
				})
			}
		})
	}
}

// Package httpapiは、ジャッジAPIのHTTPインターフェースを提供する。
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// NewHandlerは、APIのルートHTTPハンドラーを返す。
func NewHandler(auth ...AuthConfig) http.Handler {
	var config AuthConfig
	if len(auth) > 0 {
		config = auth[0]
	}
	return newHandler(config, handlerDependencies{})
}

func newHandler(config AuthConfig, private handlerDependencies) http.Handler {
	return headAsGet(newMux(config, private))
}

// headAsGet serves HEAD as the GET read, so handlers that branch on the method never treat it as a write.
// It drops the body here because the Lambda adapter returns whatever the handler writes.
func headAsGet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		get := r.Clone(r.Context())
		get.Method = http.MethodGet
		next.ServeHTTP(headResponse{w}, get)
	})
}

// headResponse keeps the status and headers of the response and discards its body.
type headResponse struct{ http.ResponseWriter }

func (headResponse) Write(p []byte) (int, error) { return len(p), nil }

func newMux(config AuthConfig, private handlerDependencies) *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", health)
	mux.HandleFunc("POST /auth/login", config.login)
	mux.HandleFunc("POST /auth/refresh", config.refresh)
	mux.HandleFunc("POST /auth/challenge", config.challenge)
	mux.HandleFunc("POST /auth/signup", config.registration)
	mux.HandleFunc("POST /auth/confirm-signup", config.registration)
	mux.HandleFunc("POST /auth/resend-confirmation", config.registration)
	registerRoutes(mux, private)

	return mux
}

func health(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	_ = json.NewEncoder(w).Encode(healthResponse{Status: "ok"})
}

// readJSONBody accepts exactly one JSON value, rejects unknown fields, and bounds reads.
func readJSONBody(w http.ResponseWriter, r *http.Request, target any, limit int64) bool {
	r.Body = http.MaxBytesReader(w, r.Body, limit)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(target)
	if err == nil {
		err = decoder.Decode(new(any))
		if err == io.EOF {
			return true
		}
	}
	var large *http.MaxBytesError
	if errors.As(err, &large) {
		authError(w, 413, "request_too_large")
	} else {
		authError(w, 400, "invalid_request")
	}
	return false
}

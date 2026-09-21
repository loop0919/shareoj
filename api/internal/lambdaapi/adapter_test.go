package lambdaapi_test

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/aws/aws-lambda-go/events"

	"judge/api/internal/httpapi"
	"judge/api/internal/lambdaapi"
)

func TestAdapterServesDocumentation(t *testing.T) {
	adapter := lambdaapi.New(httpapi.NewHandler())
	for path, content := range map[string]string{"/docs": "SwaggerUIBundle", "/openapi.json": `"openapi":"3.1.0"`} {
		response, err := adapter.ProxyWithContext(context.Background(), events.APIGatewayV2HTTPRequest{
			RawPath:        path,
			RequestContext: events.APIGatewayV2HTTPRequestContext{HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: "GET"}},
		})
		if err != nil || response.StatusCode != 200 || !strings.Contains(response.Body, content) || response.IsBase64Encoded {
			t.Fatalf("%s: %+v %v", path, response, err)
		}
	}
}

func TestAdapterServesHealthEndpoint(t *testing.T) {
	t.Parallel()

	adapter := lambdaapi.New(httpapi.NewHandler())
	response, err := adapter.ProxyWithContext(context.Background(), events.APIGatewayV2HTTPRequest{
		RawPath: "/health",
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			DomainName: "example.execute-api.ap-northeast-1.amazonaws.com",
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:   http.MethodGet,
				Path:     "/health",
				SourceIP: "192.0.2.1",
			},
		},
	})
	if err != nil {
		t.Fatalf("ProxyWithContext() error = %v", err)
	}

	if response.StatusCode != http.StatusOK {
		t.Errorf("status code = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := response.Headers["Content-Type"]; got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}
	if response.Body != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q, want health response", response.Body)
	}
}

func TestAdapterTranslatesRequestAndResponse(t *testing.T) {
	t.Parallel()

	type contextKey string
	const key contextKey = "request"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("read request body: %v", err)
		}

		if r.Method != http.MethodPost {
			t.Errorf("method = %q, want %q", r.Method, http.MethodPost)
		}
		if r.URL.Path != "/submissions" || r.URL.RawQuery != "page=2" {
			t.Errorf("URL = %q, want %q", r.URL.String(), "/submissions?page=2")
		}
		if r.Host != "api.example.com" {
			t.Errorf("host = %q, want %q", r.Host, "api.example.com")
		}
		if r.Header.Get("X-Request-ID") != "request-id" {
			t.Errorf("X-Request-ID = %q, want %q", r.Header.Get("X-Request-ID"), "request-id")
		}
		if r.Header.Get("Cookie") != "session=abc; theme=dark" {
			t.Errorf("Cookie = %q, want combined cookies", r.Header.Get("Cookie"))
		}
		if string(body) != "request body" {
			t.Errorf("body = %q, want %q", body, "request body")
		}
		if r.Context().Value(key) != "context value" {
			t.Error("request context was not preserved")
		}

		w.Header().Add("Set-Cookie", "one=1")
		w.Header().Add("Set-Cookie", "two=2")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte{0xff, 0x00})
	})
	adapter := lambdaapi.New(handler)

	ctx := context.WithValue(context.Background(), key, "context value")
	response, err := adapter.ProxyWithContext(ctx, events.APIGatewayV2HTTPRequest{
		RawPath:        "/submissions",
		RawQueryString: "page=2",
		Headers: map[string]string{
			"host":         "api.example.com",
			"x-request-id": "request-id",
		},
		Cookies:         []string{"session=abc", "theme=dark"},
		Body:            base64.StdEncoding.EncodeToString([]byte("request body")),
		IsBase64Encoded: true,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{
				Method:   http.MethodPost,
				Path:     "/submissions",
				SourceIP: "192.0.2.1",
			},
		},
	})
	if err != nil {
		t.Fatalf("ProxyWithContext() error = %v", err)
	}

	if response.StatusCode != http.StatusCreated {
		t.Errorf("status code = %d, want %d", response.StatusCode, http.StatusCreated)
	}
	if !response.IsBase64Encoded {
		t.Error("binary response was not base64 encoded")
	}
	if response.Body != base64.StdEncoding.EncodeToString([]byte{0xff, 0x00}) {
		t.Errorf("body = %q, want base64 encoded response", response.Body)
	}
	if len(response.Cookies) != 2 || response.Cookies[0] != "one=1" || response.Cookies[1] != "two=2" {
		t.Errorf("cookies = %#v, want two Set-Cookie values", response.Cookies)
	}
}

func TestAdapterRejectsInvalidBase64Body(t *testing.T) {
	t.Parallel()

	adapter := lambdaapi.New(http.NotFoundHandler())
	_, err := adapter.ProxyWithContext(context.Background(), events.APIGatewayV2HTTPRequest{
		Body:            "not base64!",
		IsBase64Encoded: true,
		RequestContext: events.APIGatewayV2HTTPRequestContext{
			HTTP: events.APIGatewayV2HTTPRequestContextHTTPDescription{Method: http.MethodPost},
		},
	})
	if err == nil {
		t.Fatal("ProxyWithContext() error = nil, want an error")
	}
}

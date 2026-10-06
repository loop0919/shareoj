package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

func TestBrowserLogin(t *testing.T) {
	for _, mode := range []string{"success", "cancelled", "wrong-api"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
			t.Setenv("SHAREOJ_ACCESS_TOKEN", "must-not-use-this-token")
			challenge := ""
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api/auth/providers":
					if r.Header.Get("Authorization") != "" {
						t.Error("token sent to public capability check")
					}
					fmt.Fprint(w, `{"google":true,"googleCLI":true}`)
				case "/api/auth/cli/exchange":
					if r.Header.Get("Authorization") != "" {
						t.Error("ambient token sent to exchange")
					}
					var body map[string]string
					_ = json.NewDecoder(r.Body).Decode(&body)
					digest := sha256.Sum256([]byte(body["verifier"]))
					if body["ticket"] != "signed-ticket" || base64.RawURLEncoding.EncodeToString(digest[:]) != challenge {
						t.Error("PKCE mismatch")
					}
					fmt.Fprint(w, `{"access_token":"access-secret","refresh_token":"refresh-secret","expires_in":3600}`)
				case "/auth/me":
					if r.Header.Get("Authorization") != "Bearer access-secret" {
						t.Error("wrong API token")
					}
					if mode == "wrong-api" {
						w.WriteHeader(401)
						fmt.Fprint(w, `{"error":"invalid_token"}`)
						return
					}
					fmt.Fprint(w, `{"id":"account-id"}`)
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
				}
			}))
			defer server.Close()
			client, err := newClient(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			err = client.browserLogin(context.Background(), server.URL, &output, func(target string) error {
				start, _ := url.Parse(target)
				if start.Path != "/auth/google" || start.Query().Get("code_verifier") != "" {
					t.Error("invalid authorization URL")
				}
				challenge = start.Query().Get("cli_challenge")
				callback := "http://127.0.0.1:" + start.Query().Get("cli_port") + "/callback"
				// Unsolicited requests cannot consume the waiting login.
				response, err := http.Get(callback + "?state=wrong&ticket=signed-ticket")
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 400 {
					t.Error("invalid state accepted")
				}
				request, _ := http.NewRequest("GET", callback, nil)
				request.Host = "attacker.example"
				response, err = http.DefaultClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				response.Body.Close()
				if response.StatusCode != 404 {
					t.Error("wrong host accepted")
				}
				query := url.Values{"state": {start.Query().Get("cli_state")}, "ticket": {"signed-ticket"}}
				if mode == "cancelled" {
					query.Set("error", "access_denied")
				}
				response, err = http.Get(callback + "?" + query.Encode())
				if err != nil {
					t.Fatal(err)
				}
				body, _ := io.ReadAll(response.Body)
				response.Body.Close()
				if (mode == "success") != (response.StatusCode == 200) {
					t.Errorf("callback status: %d", response.StatusCode)
				}
				if strings.Contains(string(body), "secret") {
					t.Error("token in browser response")
				}
				return nil
			})
			if mode == "success" {
				if err != nil {
					t.Fatal(err)
				}
				var saved tokens
				if err := readJSON(client.credential, &saved); err != nil {
					t.Fatal(err)
				}
				if saved.Access != "access-secret" || saved.Refresh != "refresh-secret" {
					t.Fatal("missing credentials")
				}
			} else {
				if err == nil {
					t.Fatal("failed login reported success")
				}
				if _, err := os.Stat(client.credential); !os.IsNotExist(err) {
					t.Fatal("failed login saved credentials")
				}
			}
			if strings.Contains(output.String(), "access-secret") || strings.Contains(output.String(), "refresh-secret") {
				t.Fatal("credentials printed")
			}
		})
	}
}

func TestBrowserLoginCancellation(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	client, _ := newClient("http://127.0.0.1:12345")
	if err := client.browserLogin(context.Background(), "", io.Discard, nil); err == nil {
		t.Fatal("custom API needs explicit site")
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"google":true,"googleCLI":true}`)
	}))
	defer server.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := client.browserLogin(ctx, server.URL, io.Discard, nil); err == nil || !strings.Contains(err.Error(), "deadline") {
		t.Fatalf("timeout: %v", err)
	}
}

func TestBrowserLoginRejectsUnsupportedWebBeforeOpeningBrowser(t *testing.T) {
	t.Setenv("SHAREOJ_CONFIG_DIR", t.TempDir())
	for _, body := range []string{`{"google":true}`, `{"google":false,"googleCLI":false}`} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/api/auth/providers" {
				t.Errorf("unexpected request: %s", r.URL.Path)
			}
			fmt.Fprint(w, body)
		}))
		client, _ := newClient(server.URL)
		var output strings.Builder
		err := client.browserLogin(context.Background(), server.URL, &output, func(string) error {
			t.Error("opened browser on unsupported website")
			return nil
		})
		server.Close()
		if err == nil || !strings.Contains(err.Error(), "deploy the updated ShareOJ web server") {
			t.Fatalf("missing deployment guidance: %v", err)
		}
		if output.Len() != 0 {
			t.Fatal("printed a login URL or entered callback wait")
		}
		if _, err := os.Stat(client.credential); !os.IsNotExist(err) {
			t.Fatal("unsupported website saved credentials")
		}
	}
}

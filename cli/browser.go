package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"sync/atomic"
	"time"
)

const defaultSite = "https://www.share-oj.net"

func openBrowser(target string) error {
	var command *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		command = exec.Command("open", target)
	case "windows":
		command = exec.Command("rundll32.exe", "url.dll,FileProtocolHandler", target)
	default:
		command = exec.Command("xdg-open", target)
	}
	if err := command.Start(); err != nil {
		return err
	}
	go func() { _ = command.Wait() }()
	return nil
}

func (c *apiClient) browserLogin(ctx context.Context, site string, out io.Writer, open func(string) error) error {
	if site == "" {
		if c.base != defaultAPI {
			return errors.New("Google login with a custom --api requires an explicit --site URL")
		}
		site = defaultSite
	}
	if err := validateURL(site); err != nil {
		return err
	}
	base, _ := url.Parse(site)
	if (base.Path != "" && base.Path != "/") || base.RawQuery != "" || base.ForceQuery {
		return errors.New("--site must be a website origin without a path or query")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return errors.New("could not start the localhost login callback")
	}
	defer listener.Close()
	var random [64]byte
	if _, err := rand.Read(random[:]); err != nil {
		return err
	}
	state := base64.RawURLEncoding.EncodeToString(random[:32])
	verifier := base64.RawURLEncoding.EncodeToString(random[32:])
	digest := sha256.Sum256([]byte(verifier))
	start := base.ResolveReference(&url.URL{Path: "/auth/google"})
	start.RawQuery = url.Values{
		"cli_port":  {fmt.Sprint(listener.Addr().(*net.TCPAddr).Port)},
		"cli_state": {state}, "cli_challenge": {base64.RawURLEncoding.EncodeToString(digest[:])},
	}.Encode()
	finished := make(chan error, 1)
	var consumed atomic.Bool
	server := &http.Server{ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 5 * time.Second, MaxHeaderBytes: 16 << 10}
	server.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Host != listener.Addr().String() || r.URL.Path != "/callback" || r.Method != "GET" {
			http.Error(w, "Not found", 404)
			return
		}
		query, err := url.ParseQuery(r.URL.RawQuery)
		if err != nil || len(query["state"]) != 1 || subtle.ConstantTimeCompare([]byte(query.Get("state")), []byte(state)) != 1 {
			http.Error(w, "Invalid login state", 400)
			return
		}
		if len(r.URL.RawQuery) > 12000 || (query.Get("error") == "" && (len(query["ticket"]) != 1 || len(query.Get("ticket")) == 0 || len(query.Get("ticket")) > 8192)) {
			http.Error(w, "Invalid login callback", 400)
			return
		}
		if !consumed.CompareAndSwap(false, true) {
			http.Error(w, "Login callback already received", 409)
			return
		}
		if query.Get("error") != "" {
			err = errors.New("Google login was cancelled or failed; start a new login")
		} else {
			body, _ := json.Marshal(map[string]string{"ticket": query.Get("ticket"), "verifier": verifier})
			var result authResult
			err = c.request(ctx, "POST", base.ResolveReference(&url.URL{Path: "/api/auth/cli/exchange"}).String(), body, map[string]string{"Content-Type": "application/json"}, &result)
			if err == nil {
				// Confirm that the returned token belongs to the API selected by the user.
				var account struct {
					ID string `json:"id"`
				}
				err = c.request(ctx, "GET", c.base+"/auth/me", nil, map[string]string{"Authorization": "Bearer " + result.Access}, &account)
				if err == nil && account.ID == "" {
					err = errors.New("Google login returned an invalid account")
				}
			}
			if err == nil {
				err = c.saveTokens(result, "")
			}
		}
		if err != nil {
			http.Error(w, "Login failed. Return to the terminal and start a new login.", 400)
		} else {
			fmt.Fprintln(w, "ShareOJ CLI login completed. You can close this tab.")
		}
		// Flush the browser response before the main goroutine closes the listener.
		_ = http.NewResponseController(w).Flush()
		finished <- err
	})
	defer server.Close()
	go func() {
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			select {
			case finished <- errors.New("localhost login server stopped"):
			default:
			}
		}
	}()
	fmt.Fprintf(out, "Open this URL in a browser on this computer:\n%s\n", start.String())
	if open != nil {
		if err := open(start.String()); err != nil {
			fmt.Fprintln(out, "Could not open a browser automatically; open the URL above.")
		}
	}
	select {
	case err := <-finished:
		return err
	case <-ctx.Done():
		return fmt.Errorf("browser login stopped: %w", ctx.Err())
	}
}

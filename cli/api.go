package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const defaultAPI = "https://api.share-oj.net"

type apiClient struct {
	base       string
	token      string
	credential string
	http       *http.Client
}

type httpError struct {
	status int
	code   string
}

func (e *httpError) Error() string { return fmt.Sprintf("HTTP %d: %s", e.status, e.code) }

func validateURL(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Fragment != "" {
		return errors.New("invalid server URL")
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1")) {
		return errors.New("HTTPS is required (HTTP is allowed only for localhost)")
	}
	return nil
}

func newClient(base string) (*apiClient, error) {
	base = strings.TrimRight(base, "/")
	if err := validateURL(base); err != nil {
		return nil, err
	}
	u, _ := url.Parse(base)
	if u.RawQuery != "" || u.ForceQuery {
		return nil, errors.New("API URL must not contain a query")
	}
	directory := os.Getenv("SHAREOJ_CONFIG_DIR")
	if directory == "" {
		var err error
		directory, err = os.UserConfigDir()
		if err != nil {
			return nil, err
		}
		directory = filepath.Join(directory, "shareoj")
	}
	return &apiClient{
		base: base, credential: filepath.Join(directory, fmt.Sprintf("%x.json", sha256.Sum256([]byte(base)))),
		http: &http.Client{Timeout: 60 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		}},
	}, nil
}

var errorCode = regexp.MustCompile(`^[a-z_]{1,80}$`)

func (c *apiClient) request(ctx context.Context, method, target string, data []byte, headers map[string]string, result any) error {
	if err := validateURL(target); err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, method, target, bytes.NewReader(data))
	if err != nil {
		return errors.New("could not create HTTP request")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	response, err := c.http.Do(req)
	if err != nil {
		// URLs can contain signed credentials. Do not print transport errors or bodies.
		return errors.New("network request failed; a remote save may have completed, check the draft before retrying")
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, (4<<20)+1))
	if err != nil {
		return errors.New("could not read server response; check the remote draft before retrying")
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		code := struct {
			Error string `json:"error"`
		}{}
		_ = json.Unmarshal(body, &code)
		if !errorCode.MatchString(code.Error) {
			code.Error = "request_failed"
		}
		return &httpError{response.StatusCode, code.Error}
	}
	if len(body) > 4<<20 {
		return errors.New("server response exceeds 4 MiB")
	}
	if result == nil {
		return nil
	}
	if err := decodeJSON(body, result); err != nil {
		return errors.New("invalid JSON response from server")
	}
	return nil
}

func (c *apiClient) call(ctx context.Context, method, path string, body, result any, auth bool) error {
	headers := map[string]string{"Content-Type": "application/json"}
	if auth {
		if err := c.authenticate(ctx); err != nil {
			return err
		}
		headers["Authorization"] = "Bearer " + c.token
	}
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return err
		}
		if len(data) > 3<<20 {
			return errors.New("API request exceeds 3 MiB")
		}
	}
	return c.request(ctx, method, c.base+path, data, headers, result)
}

type tokens struct {
	Access  string `json:"access_token"`
	Refresh string `json:"refresh_token"`
	Expires int64  `json:"expires_at"`
}

type authResult struct {
	Access     string            `json:"access_token"`
	Refresh    string            `json:"refresh_token"`
	Expires    int64             `json:"expires_in"`
	Challenge  string            `json:"challenge_name"`
	Session    string            `json:"session"`
	Parameters map[string]string `json:"challenge_parameters"`
}

func (c *apiClient) saveTokens(result authResult, refresh string) error {
	if result.Access == "" || result.Expires <= 0 || result.Challenge != "" {
		return errors.New("login did not return an access token")
	}
	if result.Refresh != "" {
		refresh = result.Refresh
	}
	if err := atomicJSON(c.credential, tokens{result.Access, refresh, time.Now().Unix() + result.Expires}); err != nil {
		return err
	}
	c.token = result.Access
	return nil
}

func (c *apiClient) authenticate(ctx context.Context) error {
	if c.token != "" {
		return nil
	}
	if c.token = os.Getenv("SHAREOJ_ACCESS_TOKEN"); c.token != "" {
		return nil
	}
	var saved tokens
	if err := readJSON(c.credential, &saved); err != nil {
		return errors.New("run shareoj login or set SHAREOJ_ACCESS_TOKEN first")
	}
	if saved.Expires > time.Now().Unix()+60 && saved.Access != "" {
		c.token = saved.Access
		return nil
	}
	if saved.Refresh == "" {
		return errors.New("session expired; run shareoj login again")
	}
	var result authResult
	if err := c.call(ctx, "POST", "/auth/refresh", map[string]string{"refresh_token": saved.Refresh}, &result, false); err != nil {
		return fmt.Errorf("session refresh failed; run shareoj login again: %w", err)
	}
	return c.saveTokens(result, saved.Refresh)
}

// prompt is supplied by the terminal command; tests can provide deterministic answers.
func (c *apiClient) login(ctx context.Context, username string, prompt func(string, bool) (string, error)) error {
	if username == "" {
		var err error
		username, err = prompt("Email", false)
		if err != nil {
			return err
		}
	}
	password, err := prompt("Password", true)
	if err != nil {
		return err
	}
	var result authResult
	if err := c.call(ctx, "POST", "/auth/login", map[string]string{"username": username, "password": password}, &result, false); err != nil {
		return err
	}
	for result.Challenge != "" {
		field := map[string]string{"NEW_PASSWORD_REQUIRED": "NEW_PASSWORD", "SMS_MFA": "SMS_MFA_CODE", "SOFTWARE_TOKEN_MFA": "SOFTWARE_TOKEN_MFA_CODE", "EMAIL_OTP": "EMAIL_OTP_CODE"}[result.Challenge]
		if field == "" {
			return errors.New("unsupported login challenge; complete account setup on the website")
		}
		if name := result.Parameters["USER_ID_FOR_SRP"]; name != "" {
			username = name
		}
		value, err := prompt(field, true)
		if err != nil {
			return err
		}
		responses := map[string]string{field: value}
		if result.Challenge == "NEW_PASSWORD_REQUIRED" {
			var attributes []string
			if raw := result.Parameters["requiredAttributes"]; raw != "" {
				if err := json.Unmarshal([]byte(raw), &attributes); err != nil {
					return errors.New("invalid required attributes in login challenge")
				}
			}
			for _, attr := range attributes {
				value, err := prompt(attr, false)
				if err != nil {
					return err
				}
				responses["userAttributes."+strings.TrimPrefix(attr, "userAttributes.")] = value
			}
		}
		body := map[string]any{"username": username, "challenge_name": result.Challenge, "session": result.Session, "responses": responses}
		result = authResult{}
		if err := c.call(ctx, "POST", "/auth/challenge", body, &result, false); err != nil {
			return err
		}
	}
	return c.saveTokens(result, "")
}

func (c *apiClient) upload(ctx context.Context, id, data string) (*testFile, error) {
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(data)))
	path := "/my/problems/" + id + "/test-files"
	var upload struct {
		ID      string            `json:"id"`
		URL     string            `json:"url"`
		Headers map[string]string `json:"headers"`
	}
	if err := c.call(ctx, "POST", path, map[string]any{"size": len(data), "sha256": digest}, &upload, true); err != nil {
		return nil, err
	}
	if !problemID.MatchString(upload.ID) {
		return nil, errors.New("invalid upload ID from server")
	}
	// The signed storage request must not carry the API access token.
	if err := c.request(ctx, "PUT", upload.URL, []byte(data), upload.Headers, nil); err != nil {
		return nil, err
	}
	var ref testFile
	if err := c.call(ctx, "POST", path+"/"+upload.ID+"/complete", nil, &ref, true); err != nil {
		return nil, err
	}
	if ref.ID != upload.ID || ref.Size != len(data) || ref.SHA256 != digest {
		return nil, errors.New("uploaded file integrity check failed")
	}
	return &ref, nil
}

func readJSON(path string, value any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return decodeJSON(data, value)
}

func atomicJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".shareoj-*") // 0600, including credentials
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, writeErr := f.Write(append(data, '\n'))
	syncErr := f.Sync()
	closeErr := f.Close()
	if err := errors.Join(writeErr, syncErr, closeErr); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

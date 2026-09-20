package httpapi

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"image/png"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"judge/api/internal/profiles"
)

var userHandle = regexp.MustCompile(`^[a-z][a-z0-9_]{2,19}$`)

// Accept only bounded PNGs and re-encode them to discard metadata and trailing data.
func cleanAvatar(value string) (string, error) {
	if value == "" {
		return "", nil
	}
	if len(value) > 180000 || !strings.HasPrefix(value, "data:image/png;base64,") {
		return "", errors.New("invalid avatar")
	}
	data, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(value, "data:image/png;base64,"))
	if err != nil {
		return "", err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	if cfg.Width < 1 || cfg.Height < 1 || cfg.Width > 256 || cfg.Height > 256 {
		return "", errors.New("invalid dimensions")
	}
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err = png.Encode(&buf, img); err != nil {
		return "", err
	}
	result := "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
	if len(result) > 180000 {
		return "", errors.New("avatar too large")
	}
	return result, nil
}

func (p profileHandler) profile(w http.ResponseWriter, r *http.Request, owner string) {
	if p.Profiles == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	if r.Method == http.MethodGet {
		profile, err := p.Profiles.Get(r.Context(), owner)
		if errors.Is(err, profiles.ErrNotFound) {
			writeAuthJSON(w, 200, map[string]any{"profile": nil})
			return
		}
		if err != nil {
			authError(w, 503, "database_unavailable")
			return
		}
		writeAuthJSON(w, 200, map[string]any{"profile": profile})
		return
	}
	if strings.Split(r.Header.Get("Content-Type"), ";")[0] != "application/json" {
		authError(w, 415, "json_required")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 200<<10)
	var input struct {
		Accounts profiles.Accounts `json:"accounts"`
		Handle   string            `json:"handle"`
		Avatar   string            `json:"avatar"`
		Version  int64             `json:"version"`
	}
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	err := decoder.Decode(&input)
	if err == nil {
		if decoder.Decode(new(any)) != io.EOF {
			err = errors.New("trailing JSON")
		}
	}
	if err != nil {
		var large *http.MaxBytesError
		if errors.As(err, &large) {
			authError(w, 413, "request_too_large")
		} else {
			authError(w, 400, "invalid_request")
		}
		return
	}
	input.Handle = strings.ToLower(strings.TrimSpace(input.Handle))
	if !userHandle.MatchString(input.Handle) || input.Version < 0 || input.Version > 9007199254740990 {
		authError(w, 400, "invalid_profile")
		return
	}
	if !cleanAccounts(&input.Accounts) {
		authError(w, 400, "invalid_accounts")
		return
	}
	avatar, err := cleanAvatar(input.Avatar)
	if err != nil {
		authError(w, 400, "invalid_avatar")
		return
	}
	result, err := p.Profiles.Save(r.Context(), owner, input.Handle, avatar, input.Version, input.Accounts)
	switch {
	case errors.Is(err, profiles.ErrHandleTaken):
		authError(w, 409, "handle_taken")
	case errors.Is(err, profiles.ErrConflict):
		authError(w, 409, "profile_conflict")
	case err != nil:
		authError(w, 503, "database_unavailable")
	default:
		writeAuthJSON(w, 200, map[string]any{"profile": result})
	}
}

func (p profileHandler) publicProfile(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	handle := r.PathValue("handle")
	if !userHandle.MatchString(handle) {
		authError(w, 404, "not_found")
		return
	}
	store, ok := p.Profiles.(interface {
		GetByHandle(context.Context, string) (profiles.Profile, error)
	})
	if !ok {
		authError(w, 503, "database_unavailable")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	profile, err := store.GetByHandle(ctx, handle)
	if errors.Is(err, profiles.ErrNotFound) {
		authError(w, 404, "not_found")
		return
	}
	if err != nil {
		authError(w, 503, "database_unavailable")
		return
	}
	writeAuthJSON(w, 200, map[string]any{"handle": profile.Handle, "avatar": profile.Avatar, "createdAt": profile.CreatedAt, "accounts": profile.Accounts})
}

var accountPatterns = []*regexp.Regexp{
	regexp.MustCompile(`^[A-Za-z0-9_]{1,15}$`),
	regexp.MustCompile(`^[A-Za-z0-9_]{1,16}$`),
	regexp.MustCompile(`^[A-Za-z0-9_.-]{3,24}$`),
	regexp.MustCompile(`^[0-9]{1,20}$`),
}

func cleanAccounts(accounts *profiles.Accounts) bool {
	for i, value := range []*string{&accounts.X, &accounts.AtCoder, &accounts.Codeforces, &accounts.Yukicoder} {
		*value = strings.TrimSpace(*value)
		if i == 0 {
			*value = strings.TrimPrefix(*value, "@")
		}
		if *value != "" && !accountPatterns[i].MatchString(*value) {
			return false
		}
	}
	return true
}

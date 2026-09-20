package httpapi

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"

	"judge/api/internal/profiles"
)

type TokenVerifier interface {
	Verify(context.Context, string) (string, error)
}

type cognitoVerifier struct {
	verifier *oidc.IDTokenVerifier
	clientID string
}

func newCognitoVerifier(issuer, clientID string) *cognitoVerifier {
	ctx := oidc.ClientContext(context.Background(), &http.Client{Timeout: 5 * time.Second})
	keys := oidc.NewRemoteKeySet(ctx, issuer+"/.well-known/jwks.json")
	return &cognitoVerifier{oidc.NewVerifier(issuer, keys, &oidc.Config{
		SkipClientIDCheck:    true, // Cognito access tokens use client_id instead of aud.
		SupportedSigningAlgs: []string{"RS256"},
	}), clientID}
}

func (v *cognitoVerifier) Verify(ctx context.Context, raw string) (string, error) {
	token, err := v.verifier.Verify(ctx, raw)
	if err != nil {
		return "", err
	}
	var claims struct {
		Use      string `json:"token_use"`
		ClientID string `json:"client_id"`
	}
	if err := token.Claims(&claims); err != nil {
		return "", err
	}
	if claims.Use != "access" || claims.ClientID != v.clientID || token.Subject == "" {
		return "", errors.New("invalid access token")
	}
	return token.Subject, nil
}

type authentication struct {
	Verifier TokenVerifier
	Profiles profiles.Repository
}
type actorHandler func(http.ResponseWriter, *http.Request, string)

func (a authentication) require(next actorHandler, profileRequired bool, timeout time.Duration) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if a.Verifier == nil {
			authError(w, 503, "authentication_unavailable")
			return
		}
		parts := strings.Fields(r.Header.Get("Authorization"))
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") || len(parts[1]) > 16384 {
			authError(w, 401, "authentication_required")
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), timeout)
		defer cancel()
		owner, err := a.Verifier.Verify(ctx, parts[1])
		if err != nil || owner == "" {
			authError(w, 401, "invalid_token")
			return
		}
		if profileRequired && a.Profiles != nil {
			if _, err := a.Profiles.Get(ctx, owner); err != nil {
				if errors.Is(err, profiles.ErrNotFound) {
					authError(w, 403, "profile_required")
				} else {
					authError(w, 503, "database_unavailable")
				}
				return
			}
		}
		next(w, r.WithContext(ctx), owner)
	}
}

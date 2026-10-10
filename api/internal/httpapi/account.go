package httpapi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/smithy-go"

	"judge/api/internal/accounts"
	"judge/api/internal/profiles"
)

// CognitoUsers manages the signed-in user's Cognito account. The Admin operations need the API role's IAM permission,
// because Google sign-ins do not carry the scope that the token-based operations require.
type CognitoUsers interface {
	AdminGetUser(context.Context, *cognitoidentityprovider.AdminGetUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminGetUserOutput, error)
	AdminDeleteUser(context.Context, *cognitoidentityprovider.AdminDeleteUserInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.AdminDeleteUserOutput, error)
	ChangePassword(context.Context, *cognitoidentityprovider.ChangePasswordInput, ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ChangePasswordOutput, error)
}

type accountHandler struct {
	Users    CognitoUsers
	PoolID   string
	Accounts *accounts.Store
}

// bearer returns the access token that authentication.require already verified, and its Cognito username.
func bearer(r *http.Request) (token, username string) {
	parts := strings.Fields(r.Header.Get("Authorization"))
	if len(parts) != 2 {
		return "", ""
	}
	segments := strings.Split(parts[1], ".")
	if len(segments) != 3 {
		return "", ""
	}
	payload, err := base64.RawURLEncoding.DecodeString(segments[1])
	if err != nil {
		return "", ""
	}
	var claims struct {
		Username string `json:"username"`
	}
	if json.Unmarshal(payload, &claims) != nil {
		return "", ""
	}
	return parts[1], claims.Username
}

// Federated users are named after their provider, such as google_123; they have no Cognito password.
func federated(username string) bool { return strings.HasPrefix(strings.ToLower(username), "google_") }

func (a accountHandler) available(w http.ResponseWriter, username string) bool {
	if a.Users == nil || a.PoolID == "" {
		authError(w, 503, "authentication_unavailable")
		return false
	}
	if username == "" {
		authError(w, 401, "invalid_token")
		return false
	}
	return true
}

func (a accountHandler) account(w http.ResponseWriter, r *http.Request, _ string) {
	_, username := bearer(r)
	if !a.available(w, username) {
		return
	}
	out, err := a.Users.AdminGetUser(r.Context(), &cognitoidentityprovider.AdminGetUserInput{UserPoolId: aws.String(a.PoolID), Username: aws.String(username)})
	if err != nil || out == nil {
		authError(w, 502, "authentication_unavailable")
		return
	}
	result := struct {
		Email    string `json:"email"`
		Provider string `json:"provider"`
	}{Provider: "password"}
	for _, attribute := range out.UserAttributes {
		switch aws.ToString(attribute.Name) {
		case "email":
			result.Email = aws.ToString(attribute.Value)
		case "identities":
			var identities []struct {
				ProviderName string `json:"providerName"`
			}
			if json.Unmarshal([]byte(aws.ToString(attribute.Value)), &identities) == nil && len(identities) > 0 {
				result.Provider = strings.ToLower(identities[0].ProviderName)
			}
		}
	}
	writeAuthJSON(w, 200, result)
}

func (a accountHandler) password(w http.ResponseWriter, r *http.Request, _ string) {
	token, username := bearer(r)
	if !a.available(w, username) {
		return
	}
	var input struct {
		Current  string `json:"current"`
		Proposed string `json:"proposed"`
	}
	if !readAuthJSON(w, r, &input) {
		return
	}
	if input.Current == "" || input.Proposed == "" || len(input.Current) > 256 || len(input.Proposed) > 256 {
		authError(w, 400, "invalid_request")
		return
	}
	if federated(username) {
		authError(w, 409, "password_unavailable")
		return
	}
	_, err := a.Users.ChangePassword(r.Context(), &cognitoidentityprovider.ChangePasswordInput{AccessToken: aws.String(token), PreviousPassword: aws.String(input.Current), ProposedPassword: aws.String(input.Proposed)})
	var apiError smithy.APIError
	switch {
	case err == nil:
		w.WriteHeader(204)
	case errors.As(err, &apiError) && apiError.ErrorCode() == "NotAuthorizedException":
		authError(w, 400, "incorrect_password")
	default:
		writeCognitoError(w, err)
	}
}

func (a accountHandler) remove(w http.ResponseWriter, r *http.Request, owner string) {
	_, username := bearer(r)
	if !a.available(w, username) {
		return
	}
	if a.Accounts == nil {
		authError(w, 503, "database_unavailable")
		return
	}
	var input struct {
		Handle string `json:"handle"`
	}
	if !readAuthJSON(w, r, &input) {
		return
	}
	err := a.Accounts.Delete(r.Context(), owner, input.Handle, func(ctx context.Context) error {
		_, err := a.Users.AdminDeleteUser(ctx, &cognitoidentityprovider.AdminDeleteUserInput{UserPoolId: aws.String(a.PoolID), Username: aws.String(username)})
		var apiError smithy.APIError
		if errors.As(err, &apiError) && apiError.ErrorCode() == "UserNotFoundException" {
			return nil
		}
		if err != nil {
			return errCognito{err}
		}
		return nil
	})
	var cognito errCognito
	switch {
	case err == nil:
		w.WriteHeader(204)
	case errors.Is(err, accounts.ErrMismatch):
		authError(w, 400, "confirmation_mismatch")
	case errors.Is(err, accounts.ErrActiveContest):
		authError(w, 409, "account_has_active_contest")
	case errors.Is(err, profiles.ErrDeleted), errors.Is(err, profiles.ErrNotFound):
		authError(w, 401, "account_deleted")
	case errors.As(err, &cognito):
		authError(w, 502, "authentication_unavailable")
	default:
		authError(w, 503, "database_unavailable")
	}
}

type errCognito struct{ error }

func (e errCognito) Unwrap() error { return e.error }

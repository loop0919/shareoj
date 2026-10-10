package httpapi_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/cognitoidentityprovider"
	"github.com/aws/smithy-go"
	"judge/api/internal/database"
	"judge/api/internal/httpapi"
)

type registrationStub struct {
	cognitoStub
	signup  func(*cognitoidentityprovider.SignUpInput) (*cognitoidentityprovider.SignUpOutput, error)
	confirm func(*cognitoidentityprovider.ConfirmSignUpInput) (*cognitoidentityprovider.ConfirmSignUpOutput, error)
	resend  func(*cognitoidentityprovider.ResendConfirmationCodeInput) (*cognitoidentityprovider.ResendConfirmationCodeOutput, error)
}

func (s registrationStub) SignUp(_ context.Context, in *cognitoidentityprovider.SignUpInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.SignUpOutput, error) {
	return s.signup(in)
}

func (s registrationStub) ConfirmSignUp(_ context.Context, in *cognitoidentityprovider.ConfirmSignUpInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ConfirmSignUpOutput, error) {
	return s.confirm(in)
}

func (s registrationStub) ResendConfirmationCode(_ context.Context, in *cognitoidentityprovider.ResendConfirmationCodeInput, _ ...func(*cognitoidentityprovider.Options)) (*cognitoidentityprovider.ResendConfirmationCodeOutput, error) {
	return s.resend(in)
}

func TestRegistrationFlowAndSecretHash(t *testing.T) {
	check := func(clientID, username, hash *string) {
		t.Helper()
		if aws.ToString(clientID) != "client" || aws.ToString(username) != "user@example.com" || aws.ToString(hash) != "4zqhOFl0JivfkWh1VINyOJyrDTdsinJktzPUT+t0plg=" {
			t.Fatal("wrong registration client, username or secret hash")
		}
	}
	client := registrationStub{
		signup: func(in *cognitoidentityprovider.SignUpInput) (*cognitoidentityprovider.SignUpOutput, error) {
			check(in.ClientId, in.Username, in.SecretHash)
			if aws.ToString(in.Password) != "ValidPassword123!" || len(in.UserAttributes) != 1 || aws.ToString(in.UserAttributes[0].Name) != "email" || aws.ToString(in.UserAttributes[0].Value) != "user@example.com" {
				t.Fatal("wrong signup attributes")
			}
			return &cognitoidentityprovider.SignUpOutput{UserConfirmed: false}, nil
		},
		confirm: func(in *cognitoidentityprovider.ConfirmSignUpInput) (*cognitoidentityprovider.ConfirmSignUpOutput, error) {
			check(in.ClientId, in.Username, in.SecretHash)
			if aws.ToString(in.ConfirmationCode) != "123456" || in.ForceAliasCreation {
				t.Fatal("wrong confirmation code or unsafe alias transfer")
			}
			return &cognitoidentityprovider.ConfirmSignUpOutput{}, nil
		},
		resend: func(in *cognitoidentityprovider.ResendConfirmationCodeInput) (*cognitoidentityprovider.ResendConfirmationCodeOutput, error) {
			check(in.ClientId, in.Username, in.SecretHash)
			return &cognitoidentityprovider.ResendConfirmationCodeOutput{}, nil
		},
	}
	h := httpapi.NewHandler(httpapi.AuthConfig{Client: client, ClientID: "client", ClientSecret: "secret"})
	for _, tc := range []struct{ path, body, response string }{
		{"signup", `{"email":" User@Example.com ","password":"ValidPassword123!"}`, "{\"confirmed\":false}\n"},
		{"confirm-signup", `{"email":"user@example.com","code":"123456"}`, "{\"confirmed\":true}\n"},
		{"resend-confirmation", `{"email":"user@example.com"}`, "{\"sent\":true}\n"},
	} {
		w := authRequest(h, "/auth/"+tc.path, tc.body)
		if w.Code != 200 || w.Body.String() != tc.response {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
		if w.Header().Get("Cache-Control") != "no-store" || w.Header().Get("Set-Cookie") != "" {
			t.Fatal("registration must be uncached and must not log in")
		}
	}
}

func TestRegistrationRejectsInvalidInput(t *testing.T) {
	h := httpapi.NewHandler(httpapi.AuthConfig{Client: registrationStub{}, ClientID: "client"})
	for _, tc := range []struct{ path, body string }{
		{"signup", `{"email":"not-email","password":"ValidPassword123!"}`},
		{"signup", `{"email":"Name <user@example.com>","password":"ValidPassword123!"}`},
		{"signup", `{"email":"user@example.com","password":"short"}`},
		{"signup", `{"email":"user@example.com","password":"ValidPassword123!","userAttributes":{"email_verified":"true"}}`},
		{"signup", `{"email":"user@example.com","password":"ValidPassword123!"} {}`},
		{"confirm-signup", `{"email":"user@example.com","code":""}`},
		{"confirm-signup", `{"email":"user@example.com","code":"123 456"}`},
		{"resend-confirmation", `{"email":"user@example.com","password":"not-allowed"}`},
	} {
		// A stub method would panic if validation unexpectedly reached Cognito.
		if w := authRequest(h, "/auth/"+tc.path, tc.body); w.Code != 400 {
			t.Fatalf("%s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	if w := authRequest(h, "/auth/signup", `{"email":"`+strings.Repeat("x", 17000)+`"}`); w.Code != 413 {
		t.Fatalf("large body: %d", w.Code)
	}
	r := httptest.NewRequest(http.MethodPost, "/auth/signup", strings.NewReader("{}"))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 415 {
		t.Fatalf("content type: %d", w.Code)
	}
	if w := authRequest(httpapi.NewHandler(), "/auth/signup", `{}`); w.Code != 503 {
		t.Fatalf("unconfigured: %d", w.Code)
	}
}

func TestRegistrationErrorsAreSanitized(t *testing.T) {
	for _, tc := range []struct {
		action, code string
		status       int
		response     string
	}{
		{"signup", "UsernameExistsException", 200, "confirmed"},
		{"signup", "InvalidPasswordException", 400, "invalid_password"},
		{"signup", "NotAuthorizedException", 503, "registration_unavailable"},
		{"signup", "TooManyRequestsException", 429, "too_many_requests"},
		{"signup", "CodeDeliveryFailureException", 502, "authentication_unavailable"},
		{"confirm-signup", "CodeMismatchException", 400, "invalid_confirmation"},
		{"confirm-signup", "ExpiredCodeException", 400, "invalid_confirmation"},
		{"confirm-signup", "AliasExistsException", 400, "invalid_confirmation"},
		{"resend-confirmation", "UserNotFoundException", 200, "sent"},
		{"resend-confirmation", "NotAuthorizedException", 200, "sent"},
		{"resend-confirmation", "InvalidParameterException", 200, "sent"},
	} {
		t.Run(tc.action+tc.code, func(t *testing.T) {
			err := &smithy.GenericAPIError{Code: tc.code, Message: "private diagnostics"}
			client := registrationStub{
				signup: func(*cognitoidentityprovider.SignUpInput) (*cognitoidentityprovider.SignUpOutput, error) {
					return nil, err
				},
				confirm: func(*cognitoidentityprovider.ConfirmSignUpInput) (*cognitoidentityprovider.ConfirmSignUpOutput, error) {
					return nil, err
				},
				resend: func(*cognitoidentityprovider.ResendConfirmationCodeInput) (*cognitoidentityprovider.ResendConfirmationCodeOutput, error) {
					return nil, err
				},
			}
			body := `{"email":"user@example.com"}`
			if tc.action == "signup" {
				body = `{"email":"user@example.com","password":"ValidPassword123!"}`
			}
			if tc.action == "confirm-signup" {
				body = `{"email":"user@example.com","code":"123456"}`
			}
			w := authRequest(httpapi.NewHandler(httpapi.AuthConfig{Client: client, ClientID: "client"}), "/auth/"+tc.action, body)
			if w.Code != tc.status || !strings.Contains(w.Body.String(), tc.response) || strings.Contains(w.Body.String(), "private") {
				t.Fatalf("%d %s", w.Code, w.Body.String())
			}
		})
	}
	client := registrationStub{signup: func(in *cognitoidentityprovider.SignUpInput) (*cognitoidentityprovider.SignUpOutput, error) {
		if in.SecretHash != nil {
			t.Fatal("secret hash sent for a public client")
		}
		return nil, errors.New("private network failure")
	}}
	w := authRequest(httpapi.NewHandler(httpapi.AuthConfig{Client: client, ClientID: "client"}), "/auth/signup", `{"email":"user@example.com","password":"ValidPassword123!"}`)
	if w.Code != 502 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("network error: %d %s", w.Code, w.Body.String())
	}
}

type quotaRecorder struct {
	calls []string
	err   error
}

func (q *quotaRecorder) Consume(_ context.Context, network, email string) error {
	q.calls = append(q.calls, network+" "+email)
	return q.err
}

// signedClientIP mirrors the Nuxt proxy, so a change on either side fails here.
func signedClientIP(secret, ip string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte("shareoj-client-ip:" + ip))
	return base64.StdEncoding.EncodeToString(mac.Sum(nil))
}

func TestRegistrationQuotaCountsSignedClientNetwork(t *testing.T) {
	sent := 0
	client := registrationStub{
		signup: func(*cognitoidentityprovider.SignUpInput) (*cognitoidentityprovider.SignUpOutput, error) {
			sent++
			return &cognitoidentityprovider.SignUpOutput{}, nil
		},
		confirm: func(*cognitoidentityprovider.ConfirmSignUpInput) (*cognitoidentityprovider.ConfirmSignUpOutput, error) {
			return &cognitoidentityprovider.ConfirmSignUpOutput{}, nil
		},
		resend: func(*cognitoidentityprovider.ResendConfirmationCodeInput) (*cognitoidentityprovider.ResendConfirmationCodeOutput, error) {
			sent++
			return &cognitoidentityprovider.ResendConfirmationCodeOutput{}, nil
		},
	}
	quota := &quotaRecorder{}
	h := httpapi.NewHandler(httpapi.AuthConfig{Client: client, ClientID: "client", ClientSecret: "secret", Registrations: quota})
	send := func(path, body, remote string, headers map[string]string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		r.Header.Set("Content-Type", "application/json")
		r.RemoteAddr = remote
		for name, value := range headers {
			r.Header.Set(name, value)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	const signup = `{"email":"User@Example.com","password":"ValidPassword123!"}`
	for _, tc := range []struct {
		name, remote string
		headers      map[string]string
		network      string
	}{
		{"API Gateway source", "203.0.113.7", nil, "203.0.113.7"},
		{"signed by Nuxt", "198.51.100.1", map[string]string{"X-ShareOJ-Client-IP": "203.0.113.8", "X-ShareOJ-Client-IP-Signature": signedClientIP("secret", "203.0.113.8")}, "203.0.113.8"},
		{"forged signature", "198.51.100.1", map[string]string{"X-ShareOJ-Client-IP": "203.0.113.8", "X-ShareOJ-Client-IP-Signature": signedClientIP("guess", "203.0.113.8")}, "198.51.100.1"},
		{"IPv6 per /64", "2001:db8:1:2:3:4:5:6", nil, "2001:db8:1:2::/64"},
		{"mapped IPv4 with port", "[::ffff:192.0.2.9]:1234", nil, "192.0.2.9"},
		{"unparsable source", "", nil, "unknown"},
	} {
		quota.calls = nil
		if w := send("/auth/signup", signup, tc.remote, tc.headers); w.Code != 200 {
			t.Fatalf("%s: %d %s", tc.name, w.Code, w.Body.String())
		}
		if want := tc.network + " user@example.com"; len(quota.calls) != 1 || quota.calls[0] != want {
			t.Fatalf("%s: counted %q, want %q", tc.name, quota.calls, want)
		}
	}
	quota.calls = nil
	if w := send("/auth/resend-confirmation", `{"email":"user@example.com"}`, "203.0.113.7", nil); w.Code != 200 || len(quota.calls) != 1 {
		t.Fatalf("resend: %d %q", w.Code, quota.calls)
	}
	// Confirmation sends no email, so it does not spend the quota.
	if w := send("/auth/confirm-signup", `{"email":"user@example.com","code":"123456"}`, "203.0.113.7", nil); w.Code != 200 || len(quota.calls) != 1 {
		t.Fatalf("confirm: %d %q", w.Code, quota.calls)
	}
	// Invalid input is rejected before it can spend the quota.
	if w := send("/auth/signup", `{"email":"user@example.com","password":"short"}`, "203.0.113.7", nil); w.Code != 400 || len(quota.calls) != 1 {
		t.Fatalf("invalid signup: %d %q", w.Code, quota.calls)
	}
	before := sent
	quota.err = &database.RegistrationQuotaError{RetryAfter: 42}
	w := send("/auth/signup", signup, "203.0.113.7", nil)
	if w.Code != 429 || w.Header().Get("Retry-After") != "42" || !strings.Contains(w.Body.String(), "too_many_requests") {
		t.Fatalf("limited: %d %q %s", w.Code, w.Header().Get("Retry-After"), w.Body.String())
	}
	quota.err = errors.New("private database failure")
	w = send("/auth/resend-confirmation", `{"email":"user@example.com"}`, "203.0.113.7", nil)
	if w.Code != 503 || !strings.Contains(w.Body.String(), "registration_unavailable") || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("quota failure: %d %s", w.Code, w.Body.String())
	}
	if sent != before {
		t.Fatal("a refused request reached Cognito")
	}
}

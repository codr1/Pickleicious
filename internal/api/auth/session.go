package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/codr1/Pickleicious/internal/api/authz"
)

const (
	authCookieName = "pickleicious_auth"
	authSessionTTL = 8 * time.Hour
)

var errAuthConfigMissing = errors.New("auth configuration missing")

type authSession struct {
	UserID         int64  `json:"user_id"`
	IsStaff        bool   `json:"is_staff"`
	HomeFacilityID *int64 `json:"home_facility_id,omitempty"`
	ExpiresAt      int64  `json:"exp"`
}

func SetAuthCookie(w http.ResponseWriter, r *http.Request, user *authz.AuthUser) error {
	if w == nil || r == nil || user == nil {
		return errors.New("auth session requires request, response, and user")
	}

	if appConfig == nil || appConfig.App.SecretKey == "" {
		return errAuthConfigMissing
	}

	expiresAt := time.Now().Add(authSessionTTL).Unix()
	session := authSession{
		UserID:         user.ID,
		IsStaff:        user.IsStaff,
		HomeFacilityID: user.HomeFacilityID,
		ExpiresAt:      expiresAt,
	}

	payload, err := json.Marshal(session)
	if err != nil {
		return err
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payload)
	signature, err := signPayload(encodedPayload)
	if err != nil {
		return err
	}

	secure := appConfig.App.Environment != "development"
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    encodedPayload + "." + signature,
		Path:     "/",
		HttpOnly: true,
		Secure:   secure,
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiresAt, 0),
		MaxAge:   int(authSessionTTL.Seconds()),
	})

	return nil
}

func UserFromRequest(r *http.Request) (*authz.AuthUser, error) {
	session, err := parseAuthCookie(r)
	if err != nil || session == nil {
		return nil, err
	}

	return &authz.AuthUser{
		ID:             session.UserID,
		IsStaff:        session.IsStaff,
		HomeFacilityID: session.HomeFacilityID,
	}, nil
}

func parseAuthCookie(r *http.Request) (*authSession, error) {
	if r == nil {
		return nil, nil
	}

	if appConfig == nil || appConfig.App.SecretKey == "" {
		return nil, errAuthConfigMissing
	}

	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		return nil, err
	}

	parts := strings.SplitN(cookie.Value, ".", 2)
	if len(parts) != 2 {
		return nil, errors.New("invalid auth cookie")
	}

	encodedPayload := parts[0]
	signature := parts[1]
	expectedSignature, err := signPayload(encodedPayload)
	if err != nil {
		return nil, err
	}

	if !hmac.Equal([]byte(signature), []byte(expectedSignature)) {
		return nil, errors.New("invalid auth cookie signature")
	}

	payload, err := base64.RawURLEncoding.DecodeString(encodedPayload)
	if err != nil {
		return nil, err
	}

	var session authSession
	if err := json.Unmarshal(payload, &session); err != nil {
		return nil, err
	}

	if session.ExpiresAt <= time.Now().Unix() {
		return nil, errors.New("auth session expired")
	}

	return &session, nil
}

func signPayload(payload string) (string, error) {
	if appConfig == nil || appConfig.App.SecretKey == "" {
		return "", errAuthConfigMissing
	}

	mac := hmac.New(sha256.New, []byte(appConfig.App.SecretKey))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/codr1/Pickleicious/internal/api/authz"
)

const (
	authCookieName         = "pickleicious_auth"
	sessionCookieName      = "pickleicious_session"
	authSessionTTL         = 8 * time.Hour
	sessionTokenBytes      = 32
	sessionCleanupInterval = 15 * time.Minute
	SessionTypeStaff       = "staff"
	SessionTypeMember      = "member"
)

var errAuthConfigMissing = errors.New("auth configuration missing")

type authSession struct {
	UserID          int64  `json:"user_id"`
	SessionType     string `json:"session_type"`
	HomeFacilityID  *int64 `json:"home_facility_id,omitempty"`
	MembershipLevel int64  `json:"membership_level"`
	ExpiresAt       int64  `json:"exp"`
}

type sessionRecord struct {
	UserID    int64
	ExpiresAt time.Time
}

var (
	sessionMu sync.RWMutex
	// In-memory sessions are intentionally ephemeral for local staff login.
	sessionStore       = make(map[string]sessionRecord)
	sessionCleanupOnce sync.Once
)

func isSecureCookie() bool {
	return appConfig == nil || appConfig.App.Environment != "development"
}

func CreateSession(w http.ResponseWriter, userID int64) error {
	if w == nil {
		return errors.New("session requires response writer")
	}

	startSessionCleanup()

	if err := clearExistingSessionsForUser(userID); err != nil {
		return err
	}

	token, err := newSessionToken()
	if err != nil {
		return err
	}

	expiresAt := time.Now().Add(authSessionTTL)
	sessionMu.Lock()
	sessionStore[token] = sessionRecord{
		UserID:    userID,
		ExpiresAt: expiresAt,
	}
	sessionMu.Unlock()

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureCookie(),
		SameSite: http.SameSiteLaxMode,
		Expires:  expiresAt,
		MaxAge:   int(authSessionTTL.Seconds()),
	})

	return nil
}

func ClearSession(w http.ResponseWriter, r *http.Request) {
	if r == nil {
		ClearSessionCookie(w)
		return
	}

	cookie, err := r.Cookie(sessionCookieName)
	if err == nil {
		deleteSession(cookie.Value)
	}

	ClearSessionCookie(w)
}

func ClearSessionCookie(w http.ResponseWriter) {
	if w == nil {
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureCookie(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func SetAuthCookie(w http.ResponseWriter, r *http.Request, user *authz.AuthUser) error {
	if w == nil || r == nil || user == nil {
		return errors.New("auth session requires request, response, and user")
	}

	if appConfig == nil || appConfig.App.SecretKey == "" {
		return errAuthConfigMissing
	}

	expiresAt := time.Now().Add(authSessionTTL).Unix()
	sessionType := user.SessionType
	if sessionType == "" {
		sessionType = sessionTypeFromStaff(user.IsStaff)
	}
	sessionType = normalizeSessionType(sessionType)
	session := authSession{
		UserID:          user.ID,
		SessionType:     sessionType,
		HomeFacilityID:  user.HomeFacilityID,
		MembershipLevel: user.MembershipLevel,
		ExpiresAt:       expiresAt,
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

	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    encodedPayload + "." + signature,
		Path:     "/",
		HttpOnly: true,
		Secure:   isSecureCookie(),
		SameSite: http.SameSiteLaxMode,
		Expires:  time.Unix(expiresAt, 0),
		MaxAge:   int(authSessionTTL.Seconds()),
	})

	return nil
}

func UserFromRequest(w http.ResponseWriter, r *http.Request) (*authz.AuthUser, error) {
	user, err := userFromSessionToken(w, r)
	if err != nil || user != nil {
		return user, err
	}

	session, err := parseAuthCookie(r)
	if err != nil || session == nil {
		return nil, err
	}

	return &authz.AuthUser{
		ID:              session.UserID,
		IsStaff:         session.SessionType == SessionTypeStaff,
		SessionType:     session.SessionType,
		HomeFacilityID:  session.HomeFacilityID,
		MembershipLevel: session.MembershipLevel,
	}, nil
}

func userFromSessionToken(w http.ResponseWriter, r *http.Request) (*authz.AuthUser, error) {
	if r == nil {
		return nil, nil
	}

	startSessionCleanup()

	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		if errors.Is(err, http.ErrNoCookie) {
			return nil, nil
		}
		return nil, err
	}

	token := cookie.Value
	session, ok := getSession(token)
	if !ok {
		ClearSessionCookie(w)
		return nil, nil
	}

	if queries == nil {
		ClearSessionCookie(w)
		return nil, errors.New("auth queries not initialized")
	}

	user, err := queries.GetUserByID(r.Context(), session.UserID)
	if err != nil {
		deleteSession(token)
		ClearSessionCookie(w)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}

	var homeFacilityID *int64
	if user.HomeFacilityID.Valid {
		id := user.HomeFacilityID.Int64
		homeFacilityID = &id
	}

	return &authz.AuthUser{
		ID:              user.ID,
		IsStaff:         user.IsStaff,
		SessionType:     sessionTypeFromStaff(user.IsStaff),
		HomeFacilityID:  homeFacilityID,
		MembershipLevel: user.MembershipLevel,
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

	session.SessionType = normalizeSessionType(session.SessionType)

	if session.ExpiresAt <= time.Now().Unix() {
		return nil, errors.New("auth session expired")
	}

	return &session, nil
}

func normalizeSessionType(sessionType string) string {
	switch sessionType {
	case SessionTypeStaff, SessionTypeMember:
		return sessionType
	default:
		return SessionTypeMember
	}
}

func sessionTypeFromStaff(isStaff bool) string {
	if isStaff {
		return SessionTypeStaff
	}
	return SessionTypeMember
}

func signPayload(payload string) (string, error) {
	if appConfig == nil || appConfig.App.SecretKey == "" {
		return "", errAuthConfigMissing
	}

	mac := hmac.New(sha256.New, []byte(appConfig.App.SecretKey))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), nil
}

func newSessionToken() (string, error) {
	token := make([]byte, sessionTokenBytes)
	if _, err := rand.Read(token); err != nil {
		return "", err
	}

	return base64.RawURLEncoding.EncodeToString(token), nil
}

func startSessionCleanup() {
	sessionCleanupOnce.Do(func() {
		// Lazy-start cleanup only when sessions are first used.
		go func() {
			ticker := time.NewTicker(sessionCleanupInterval)
			defer ticker.Stop()
			for range ticker.C {
				pruneExpiredSessions()
			}
		}()
	})
}

func pruneExpiredSessions() {
	now := time.Now()
	sessionMu.Lock()
	for token, session := range sessionStore {
		if session.ExpiresAt.Before(now) {
			delete(sessionStore, token)
		}
	}
	sessionMu.Unlock()
}

func clearExistingSessionsForUser(userID int64) error {
	sessionMu.Lock()
	for token, session := range sessionStore {
		if session.UserID == userID {
			delete(sessionStore, token)
		}
	}
	sessionMu.Unlock()
	return nil
}

func getSession(token string) (sessionRecord, bool) {
	sessionMu.RLock()
	session, ok := sessionStore[token]
	sessionMu.RUnlock()
	if !ok {
		return sessionRecord{}, false
	}

	if session.ExpiresAt.Before(time.Now()) {
		deleteSession(token)
		return sessionRecord{}, false
	}

	return session, true
}

func deleteSession(token string) {
	sessionMu.Lock()
	delete(sessionStore, token)
	sessionMu.Unlock()
}

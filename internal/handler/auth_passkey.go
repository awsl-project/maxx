package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const (
	passkeySessionTypeRegister = "register"
	passkeySessionTypeLogin    = "login"
)

type passkeySession struct {
	Type     string
	UserID   uint64
	TenantID uint64
	Session  webauthn.SessionData
}

type passkeySessionStore struct {
	mu       sync.Mutex
	sessions map[string]passkeySession
}

func newPasskeySessionStore() *passkeySessionStore {
	return &passkeySessionStore{
		sessions: make(map[string]passkeySession),
	}
}

func (s *passkeySessionStore) put(session passkeySession) string {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()
	sessionID := uuid.NewString()
	s.sessions[sessionID] = session
	return sessionID
}

func (s *passkeySessionStore) consume(sessionID string, expectedType string) (passkeySession, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.cleanupLocked()
	session, ok := s.sessions[sessionID]
	if !ok {
		return passkeySession{}, false
	}
	delete(s.sessions, sessionID)
	if session.Type != expectedType {
		return passkeySession{}, false
	}
	return session, true
}

func (s *passkeySessionStore) cleanupLocked() {
	now := time.Now()
	for id, session := range s.sessions {
		if now.After(session.Session.Expires) {
			delete(s.sessions, id)
		}
	}
}

type webAuthnUser struct {
	user        *domain.User
	credentials []webauthn.Credential
}

func newWebAuthnUser(user *domain.User, credentials []webauthn.Credential) *webAuthnUser {
	return &webAuthnUser{
		user:        user,
		credentials: credentials,
	}
}

func (u *webAuthnUser) WebAuthnID() []byte {
	return []byte(fmt.Sprintf("%d", u.user.ID))
}

func (u *webAuthnUser) WebAuthnName() string {
	return u.user.Username
}

func (u *webAuthnUser) WebAuthnDisplayName() string {
	return u.user.Username
}

func (u *webAuthnUser) WebAuthnCredentials() []webauthn.Credential {
	return u.credentials
}

func parsePasskeyCredentials(raw string) ([]webauthn.Credential, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []webauthn.Credential{}, nil
	}
	var credentials []webauthn.Credential
	if err := json.Unmarshal([]byte(trimmed), &credentials); err != nil {
		return nil, err
	}
	return credentials, nil
}

func encodePasskeyCredentials(credentials []webauthn.Credential) (string, error) {
	if len(credentials) == 0 {
		return "", nil
	}
	data, err := json.Marshal(credentials)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func upsertCredential(credentials []webauthn.Credential, updated *webauthn.Credential) []webauthn.Credential {
	for i := range credentials {
		if bytes.Equal(credentials[i].ID, updated.ID) {
			credentials[i] = *updated
			return credentials
		}
	}
	return append(credentials, *updated)
}

func (h *AuthHandler) handlePasskeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.authEnabled || h.authMiddleware == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authentication is disabled"})
		return
	}

	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Username == "" || body.Password == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username and password are required"})
		return
	}

	user, err := h.userRepo.GetByUsername(body.Username)
	if err != nil || bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(body.Password)) != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if !ensureUserIsActive(w, user) {
		return
	}

	credentials, err := parsePasskeyCredentials(user.PasskeyCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
		return
	}

	wAuthn, err := newWebAuthnFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	options := []webauthn.RegistrationOption{
		webauthn.WithAuthenticatorSelection(protocol.AuthenticatorSelection{
			ResidentKey:      protocol.ResidentKeyRequirementPreferred,
			UserVerification: protocol.VerificationRequired,
		}),
		webauthn.WithConveyancePreference(protocol.PreferNoAttestation),
	}
	if len(credentials) > 0 {
		options = append(options, webauthn.WithExclusions(webauthn.Credentials(credentials).CredentialDescriptors()))
	}

	creation, session, err := wAuthn.BeginRegistration(newWebAuthnUser(user, credentials), options...)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate passkey registration options"})
		return
	}

	sessionID := h.passkeyStore.put(passkeySession{
		Type:     passkeySessionTypeRegister,
		UserID:   user.ID,
		TenantID: user.TenantID,
		Session:  *session,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"sessionID": sessionID,
		"options":   creation.Response,
	})
}

func (h *AuthHandler) handlePasskeyRegisterVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.authEnabled || h.authMiddleware == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authentication is disabled"})
		return
	}

	var body struct {
		SessionID  string          `json:"sessionID"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.SessionID == "" || len(body.Credential) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionID and credential are required"})
		return
	}

	session, ok := h.passkeyStore.consume(body.SessionID, passkeySessionTypeRegister)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired passkey session"})
		return
	}

	user, err := h.userRepo.GetByID(session.TenantID, session.UserID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}
	if !ensureUserIsActive(w, user) {
		return
	}

	credentials, err := parsePasskeyCredentials(user.PasskeyCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
		return
	}

	wAuthn, err := newWebAuthnFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	credentialReq, err := newPasskeyCredentialRequest(r, body.Credential)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid passkey credential payload"})
		return
	}

	registeredCredential, err := wAuthn.FinishRegistration(
		newWebAuthnUser(user, credentials),
		session.Session,
		credentialReq,
	)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "passkey registration verification failed"})
		return
	}

	credentials = upsertCredential(credentials, registeredCredential)
	encodedCredentials, err := encodePasskeyCredentials(credentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store passkey credentials"})
		return
	}

	user.PasskeyCredentials = encodedCredentials
	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save passkey credentials"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"message": "passkey registered",
	})
}

func (h *AuthHandler) handlePasskeyLoginOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.authEnabled || h.authMiddleware == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authentication is disabled"})
		return
	}

	var body struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.Username == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "username is required"})
		return
	}

	user, err := h.userRepo.GetByUsername(body.Username)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if !ensureUserIsActive(w, user) {
		return
	}

	credentials, err := parsePasskeyCredentials(user.PasskeyCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
		return
	}
	if len(credentials) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "passkey is not registered for this user"})
		return
	}

	wAuthn, err := newWebAuthnFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	assertion, session, err := wAuthn.BeginLogin(
		newWebAuthnUser(user, credentials),
		webauthn.WithUserVerification(protocol.VerificationRequired),
	)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate passkey login options"})
		return
	}

	sessionID := h.passkeyStore.put(passkeySession{
		Type:     passkeySessionTypeLogin,
		UserID:   user.ID,
		TenantID: user.TenantID,
		Session:  *session,
	})

	writeJSON(w, http.StatusOK, map[string]any{
		"success":   true,
		"sessionID": sessionID,
		"options":   assertion.Response,
	})
}

func (h *AuthHandler) handlePasskeyLoginVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !h.authEnabled || h.authMiddleware == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authentication is disabled"})
		return
	}

	var body struct {
		SessionID  string          `json:"sessionID"`
		Credential json.RawMessage `json:"credential"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	if body.SessionID == "" || len(body.Credential) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "sessionID and credential are required"})
		return
	}

	session, ok := h.passkeyStore.consume(body.SessionID, passkeySessionTypeLogin)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired passkey session"})
		return
	}

	user, err := h.userRepo.GetByID(session.TenantID, session.UserID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}
	if !ensureUserIsActive(w, user) {
		return
	}

	credentials, err := parsePasskeyCredentials(user.PasskeyCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
		return
	}
	if len(credentials) == 0 {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
		return
	}

	wAuthn, err := newWebAuthnFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	credentialReq, err := newPasskeyCredentialRequest(r, body.Credential)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid passkey credential payload"})
		return
	}

	validatedCredential, err := wAuthn.FinishLogin(
		newWebAuthnUser(user, credentials),
		session.Session,
		credentialReq,
	)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid passkey credential"})
		return
	}

	credentials = upsertCredential(credentials, validatedCredential)
	encodedCredentials, err := encodePasskeyCredentials(credentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store passkey credentials"})
		return
	}
	user.PasskeyCredentials = encodedCredentials
	now := time.Now()
	user.LastLoginAt = &now
	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update user login state"})
		return
	}

	token, err := h.authMiddleware.GenerateToken(user)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate token"})
		return
	}

	var tenantName string
	if tenant, err := h.tenantRepo.GetByID(user.TenantID); err == nil {
		tenantName = tenant.Name
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success": true,
		"token":   token,
		"user": map[string]any{
			"id":         user.ID,
			"username":   user.Username,
			"tenantID":   user.TenantID,
			"tenantName": tenantName,
			"role":       user.Role,
		},
	})
}

func ensureUserIsActive(w http.ResponseWriter, user *domain.User) bool {
	if user.Status == domain.UserStatusActive {
		return true
	}
	if user.Status == domain.UserStatusPending {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "account pending approval"})
		return false
	}
	writeJSON(w, http.StatusForbidden, map[string]string{"error": "account is not active"})
	return false
}

func newWebAuthnFromRequest(r *http.Request) (*webauthn.WebAuthn, error) {
	origin, rpID, err := derivePasskeyOriginAndRPID(r)
	if err != nil {
		return nil, err
	}
	return webauthn.New(&webauthn.Config{
		RPDisplayName: "MAXX",
		RPID:          rpID,
		RPOrigins:     []string{origin},
	})
}

func derivePasskeyOriginAndRPID(r *http.Request) (origin string, rpID string, err error) {
	host := firstHeaderOrDefault(r.Header.Get("X-Forwarded-Host"), r.Host)
	host = strings.TrimSpace(host)
	if host == "" {
		return "", "", fmt.Errorf("missing request host")
	}

	proto := firstHeaderOrDefault(r.Header.Get("X-Forwarded-Proto"), "")
	proto = strings.TrimSpace(strings.ToLower(proto))
	if proto == "" {
		if r.TLS != nil {
			proto = "https"
		} else {
			proto = "http"
		}
	}

	parsedRPID := hostToRPID(host)
	if parsedRPID == "" {
		return "", "", fmt.Errorf("invalid request host")
	}

	return proto + "://" + host, parsedRPID, nil
}

func hostToRPID(host string) string {
	host = strings.TrimSpace(host)
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	host = strings.TrimSpace(strings.ToLower(host))
	return host
}

func firstHeaderOrDefault(raw string, fallback string) string {
	if strings.TrimSpace(raw) == "" {
		return fallback
	}
	parts := strings.Split(raw, ",")
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func newPasskeyCredentialRequest(r *http.Request, credential json.RawMessage) (*http.Request, error) {
	if len(credential) == 0 {
		return nil, fmt.Errorf("empty credential payload")
	}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "/", bytes.NewReader(credential))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	return req, nil
}

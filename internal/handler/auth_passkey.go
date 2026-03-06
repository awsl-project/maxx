package handler

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
	"github.com/google/uuid"
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

	// Set a default expiry if the webauthn library didn't set one
	// (Expires is only set when Config.Timeouts.*.Enforce is true)
	if session.Session.Expires.IsZero() {
		session.Session.Expires = time.Now().Add(5 * time.Minute)
	}

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
		if !session.Session.Expires.IsZero() && now.After(session.Session.Expires) {
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

type passkeyCredentialInfo struct {
	ID             string   `json:"id"`
	Label          string   `json:"label"`
	Attachment     string   `json:"attachment,omitempty"`
	Transports     []string `json:"transports,omitempty"`
	SignCount      uint32   `json:"signCount"`
	BackupEligible bool     `json:"backupEligible"`
	BackupState    bool     `json:"backupState"`
	CloneWarning   bool     `json:"cloneWarning"`
}

func encodeCredentialID(id []byte) string {
	return base64.RawURLEncoding.EncodeToString(id)
}

func decodeCredentialID(raw string) ([]byte, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil, fmt.Errorf("empty credential id")
	}
	if decoded, err := base64.RawURLEncoding.DecodeString(trimmed); err == nil {
		return decoded, nil
	}
	if decoded, err := base64.URLEncoding.DecodeString(trimmed); err == nil {
		return decoded, nil
	}
	return nil, fmt.Errorf("invalid credential id")
}

func removeCredentialByID(credentials []webauthn.Credential, credentialID []byte) ([]webauthn.Credential, bool) {
	updated := make([]webauthn.Credential, 0, len(credentials))
	removed := false
	for _, credential := range credentials {
		if bytes.Equal(credential.ID, credentialID) {
			removed = true
			continue
		}
		updated = append(updated, credential)
	}
	return updated, removed
}

func toPasskeyCredentialInfos(credentials []webauthn.Credential) []passkeyCredentialInfo {
	infos := make([]passkeyCredentialInfo, 0, len(credentials))
	for i, credential := range credentials {
		transports := make([]string, 0, len(credential.Transport))
		for _, transport := range credential.Transport {
			if transport == "" {
				continue
			}
			transports = append(transports, string(transport))
		}
		infos = append(infos, passkeyCredentialInfo{
			ID:             encodeCredentialID(credential.ID),
			Label:          fmt.Sprintf("Passkey %d", i+1),
			Attachment:     string(credential.Authenticator.Attachment),
			Transports:     transports,
			SignCount:      credential.Authenticator.SignCount,
			BackupEligible: credential.Flags.BackupEligible,
			BackupState:    credential.Flags.BackupState,
			CloneWarning:   credential.Authenticator.CloneWarning,
		})
	}
	return infos
}

func (h *AuthHandler) handlePasskeyRegisterOptions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	user, ok := h.getAuthenticatedPasskeyUser(w, r)
	if !ok {
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
			ResidentKey:      protocol.ResidentKeyRequirementRequired,
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

	// JWT 认证：确保调用者已登录
	currentUser, ok := h.getAuthenticatedPasskeyUser(w, r)
	if !ok {
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

	// 验证 JWT 身份与 session 中记录的用户一致，防止会话劫持
	if currentUser.ID != session.UserID || currentUser.TenantID != session.TenantID {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return
	}

	user := currentUser

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

	wAuthn, err := newWebAuthnFromRequest(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	if body.Username == "" {
		// Discoverable login: no username provided, let the authenticator choose
		assertion, session, err := wAuthn.BeginDiscoverableLogin(
			webauthn.WithUserVerification(protocol.VerificationRequired),
		)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to generate passkey login options"})
			return
		}

		sessionID := h.passkeyStore.put(passkeySession{
			Type:     passkeySessionTypeLogin,
			UserID:   0,
			TenantID: 0,
			Session:  *session,
		})

		writeJSON(w, http.StatusOK, map[string]any{
			"success":   true,
			"sessionID": sessionID,
			"options":   assertion.Response,
		})
		return
	}

	// Username-based login: existing flow
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

	var user *domain.User
	var credentials []webauthn.Credential
	var validatedCredential *webauthn.Credential

	if session.UserID == 0 {
		// Discoverable login: resolve user from userHandle in the credential response
		var discoveredUser *domain.User
		var discoveredCreds []webauthn.Credential
		validatedCredential, err = wAuthn.FinishDiscoverableLogin(
			func(rawID, userHandle []byte) (webauthn.User, error) {
				userID, parseErr := strconv.ParseUint(string(userHandle), 10, 64)
				if parseErr != nil {
					return nil, fmt.Errorf("invalid user handle")
				}
				u, dbErr := h.userRepo.GetByID(0, userID)
				if dbErr != nil {
					return nil, fmt.Errorf("user not found")
				}
				creds, credErr := parsePasskeyCredentials(u.PasskeyCredentials)
				if credErr != nil {
					return nil, fmt.Errorf("invalid stored passkey credentials")
				}
				discoveredUser = u
				discoveredCreds = creds
				return newWebAuthnUser(u, creds), nil
			},
			session.Session,
			credentialReq,
		)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid passkey credential"})
			return
		}
		if discoveredUser == nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid passkey credential"})
			return
		}
		user = discoveredUser
		credentials = discoveredCreds
	} else {
		// Username-based login: existing flow
		user, err = h.userRepo.GetByID(session.TenantID, session.UserID)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}

		credentials, err = parsePasskeyCredentials(user.PasskeyCredentials)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
			return
		}
		if len(credentials) == 0 {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}

		validatedCredential, err = wAuthn.FinishLogin(
			newWebAuthnUser(user, credentials),
			session.Session,
			credentialReq,
		)
		if err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid passkey credential"})
			return
		}
	}

	if !ensureUserIsActive(w, user) {
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

func (h *AuthHandler) getAuthenticatedPasskeyUser(w http.ResponseWriter, r *http.Request) (*domain.User, bool) {
	if !h.authEnabled || h.authMiddleware == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "authentication is disabled"})
		return nil, false
	}

	authHeader := r.Header.Get(AuthHeader)
	if !strings.HasPrefix(authHeader, "Bearer ") {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "authentication required"})
		return nil, false
	}

	claims, valid := h.authMiddleware.ValidateToken(strings.TrimPrefix(authHeader, "Bearer "))
	if !valid {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
		return nil, false
	}

	user, err := h.userRepo.GetByID(claims.TenantID, claims.UserID)
	if err != nil {
		if err == domain.ErrNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal server error"})
		return nil, false
	}

	if !ensureUserIsActive(w, user) {
		return nil, false
	}

	return user, true
}

func (h *AuthHandler) handlePasskeyCredentialList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	user, ok := h.getAuthenticatedPasskeyUser(w, r)
	if !ok {
		return
	}

	credentials, err := parsePasskeyCredentials(user.PasskeyCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"success":     true,
		"credentials": toPasskeyCredentialInfos(credentials),
	})
}

func (h *AuthHandler) handlePasskeyCredentialDelete(w http.ResponseWriter, r *http.Request, rawCredentialID string) {
	if r.Method != http.MethodDelete {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}

	credentialID, err := decodeCredentialID(rawCredentialID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid credential id"})
		return
	}

	user, ok := h.getAuthenticatedPasskeyUser(w, r)
	if !ok {
		return
	}

	credentials, err := parsePasskeyCredentials(user.PasskeyCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "invalid stored passkey credentials"})
		return
	}

	updatedCredentials, removed := removeCredentialByID(credentials, credentialID)
	if !removed {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "passkey credential not found"})
		return
	}

	encodedCredentials, err := encodePasskeyCredentials(updatedCredentials)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to store passkey credentials"})
		return
	}

	user.PasskeyCredentials = encodedCredentials
	if err := h.userRepo.Update(user); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update passkey credentials"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"success": true})
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

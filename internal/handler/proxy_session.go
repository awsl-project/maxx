package handler

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"net/http"

	"github.com/awsl-project/maxx/internal/domain"
	"github.com/google/uuid"
)

// resolveProxySessionID scopes client identities to their server-verified owner.
// Token ID zero denotes anonymous access; unverified credential headers never
// select a persistent anonymous session. Legacy records cannot establish their
// owner and are deliberately not reused by this versioned identity scheme.
func (h *ProxyHandler) resolveProxySessionID(r *http.Request, body []byte, clientType domain.ClientType, tenantID, apiTokenID uint64) string {
	var clientID string
	if apiTokenID != 0 {
		clientID = h.clientAdapter.ExtractSessionID(r, body, clientType)
	} else {
		clientID = h.clientAdapter.ExtractExplicitSessionID(r, body, clientType)
	}
	if clientID == "" {
		clientID = uuid.NewString()
	}
	return scopedProxySessionID(tenantID, apiTokenID, clientType, clientID)
}

// scopedProxySessionID encodes ownership and client identity in a bounded opaque
// lookup key. Fixed-width integers and length-prefixed strings prevent ambiguous
// tuples; all inputs, including apparent storage keys, undergo the same encoding.
func scopedProxySessionID(tenantID, apiTokenID uint64, clientType domain.ClientType, clientID string) string {
	key := []byte("maxx-proxy-session-v2")
	key = binary.BigEndian.AppendUint64(key, tenantID)
	key = binary.BigEndian.AppendUint64(key, apiTokenID)
	key = binary.BigEndian.AppendUint64(key, uint64(len(clientType)))
	key = append(key, clientType...)
	key = binary.BigEndian.AppendUint64(key, uint64(len(clientID)))
	key = append(key, clientID...)
	digest := sha256.Sum256(key)
	return "session-v2-" + hex.EncodeToString(digest[:])
}

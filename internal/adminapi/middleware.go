package adminapi

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type contextKey string

const ctxKeyUserID contextKey = "userID"

// JWTMiddleware validates Bearer token and injects claims into context.
func (s *Server) JWTMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token, ok := extractBearerToken(r)
		if !ok {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", "missing or malformed Authorization header")
			return
		}

		subject, err := s.parseJWT(token)
		if err != nil {
			writeJSONError(w, http.StatusUnauthorized, "unauthorized", err.Error())
			return
		}

		ctx := contextWithUserID(r.Context(), subject)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// parseJWT splits the token, verifies the HMAC-SHA256 signature, checks exp,
// and returns the sub claim.
func (s *Server) parseJWT(tokenStr string) (string, error) {
	parts := strings.Split(tokenStr, ".")
	if len(parts) != 3 {
		return "", fmt.Errorf("invalid token format")
	}

	headerPayload := parts[0] + "." + parts[1]

	sig, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return "", fmt.Errorf("invalid signature encoding")
	}

	mac := hmac.New(sha256.New, s.jwtSecret)
	mac.Write([]byte(headerPayload))
	expectedSig := mac.Sum(nil)

	if !hmac.Equal(sig, expectedSig) {
		return "", fmt.Errorf("invalid signature")
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return "", fmt.Errorf("invalid payload encoding")
	}

	var claims struct {
		Sub string  `json:"sub"`
		Exp float64 `json:"exp"`
	}
	if err := json.Unmarshal(payloadBytes, &claims); err != nil {
		return "", fmt.Errorf("invalid payload JSON")
	}

	if claims.Sub == "" {
		return "", fmt.Errorf("missing sub claim")
	}

	expTime := time.Unix(int64(claims.Exp), 0)
	if time.Now().After(expTime) {
		return "", fmt.Errorf("token expired")
	}

	return claims.Sub, nil
}

// generateJWT builds a JWT with the given subject and TTL, signed with HMAC-SHA256.
func (s *Server) generateJWT(subject string, ttl time.Duration) (string, time.Time, error) {
	header := map[string]string{"alg": "HS256", "typ": "JWT"}
	headerJSON, err := json.Marshal(header)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal header: %w", err)
	}

	expiresAt := time.Now().Add(ttl)
	payload := map[string]interface{}{
		"sub": subject,
		"iat": time.Now().Unix(),
		"exp": expiresAt.Unix(),
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("marshal payload: %w", err)
	}

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	signingInput := headerB64 + "." + payloadB64

	mac := hmac.New(sha256.New, s.jwtSecret)
	mac.Write([]byte(signingInput))
	sig := mac.Sum(nil)
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	token := signingInput + "." + sigB64
	return token, expiresAt, nil
}

// extractBearerToken extracts the token from the Authorization header.
func extractBearerToken(r *http.Request) (string, bool) {
	auth := r.Header.Get("Authorization")
	if auth == "" {
		return "", false
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(auth, prefix) {
		return "", false
	}
	token := strings.TrimSpace(auth[len(prefix):])
	if token == "" {
		return "", false
	}
	return token, true
}

// contextWithUserID stores the user ID in the context.
func contextWithUserID(ctx context.Context, userID string) context.Context {
	return context.WithValue(ctx, ctxKeyUserID, userID)
}

// userIDFromContext retrieves the user ID from the context.
func userIDFromContext(ctx context.Context) (string, bool) {
	uid, ok := ctx.Value(ctxKeyUserID).(string)
	return uid, ok
}

// writeJSONError writes a JSON error response with the given status code.
func writeJSONError(w http.ResponseWriter, status int, errCode, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	resp := map[string]string{"error": errCode, "message": message}
	json.NewEncoder(w).Encode(resp)
}

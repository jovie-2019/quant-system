package adminapi

import (
	"encoding/json"
	"net/http"
	"time"

	"golang.org/x/crypto/bcrypt"
)

const jwtTTL = 24 * time.Hour

// HandleLogin handles POST /api/v1/auth/login.
// It verifies the password and returns a signed JWT on success.
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}

	var req struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(s.passHash), []byte(req.Password)); err != nil {
		s.logger.Warn("login: invalid credentials")
		writeJSONError(w, http.StatusUnauthorized, "invalid_credentials", "wrong password")
		return
	}

	token, expiresAt, err := s.generateJWT("admin", jwtTTL)
	if err != nil {
		s.logger.Error("login: failed to generate JWT", "error", err)
		writeJSONError(w, http.StatusInternalServerError, "internal_error", "failed to generate token")
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"token":      token,
		"expires_at": expiresAt.UTC().Format(time.RFC3339),
	})
}

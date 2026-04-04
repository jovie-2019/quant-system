package adminapi

import (
	"net/http"
	"strconv"

	"quant-system/internal/adminstore"
)

// apiKeyResponse is the JSON representation of an API key with masked values.
type apiKeyResponse struct {
	ID           int64  `json:"id"`
	ExchangeID   int64  `json:"exchange_id"`
	Label        string `json:"label"`
	APIKey       string `json:"api_key"`
	APISecret    string `json:"api_secret"`
	Passphrase   string `json:"passphrase"`
	Permissions  string `json:"permissions"`
	Status       string `json:"status"`
	CreatedMS    int64  `json:"created_ms"`
	UpdatedMS    int64  `json:"updated_ms"`
}

// toAPIKeyResponse converts a store APIKey to its masked JSON representation.
func (s *Server) toAPIKeyResponse(key adminstore.APIKey) apiKeyResponse {
	// Decrypt the API key to show last 4 chars; fall back to full mask on error.
	maskedKey := "****"
	if plainKey, err := s.encryptor.Decrypt(key.APIKeyEnc); err == nil {
		maskedKey = maskAPIKey(plainKey)
	}

	maskedPassphrase := ""
	if key.PassphraseEnc != "" {
		maskedPassphrase = "********"
	}

	return apiKeyResponse{
		ID:          key.ID,
		ExchangeID:  key.ExchangeID,
		Label:       key.Label,
		APIKey:      maskedKey,
		APISecret:   "********",
		Passphrase:  maskedPassphrase,
		Permissions: key.Permissions,
		Status:      key.Status,
		CreatedMS:   key.CreatedMS,
		UpdatedMS:   key.UpdatedMS,
	}
}

// maskAPIKey returns a masked version of the API key showing only the last 4 chars.
func maskAPIKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// HandleListAPIKeys handles GET /api/v1/accounts.
func (s *Server) HandleListAPIKeys(w http.ResponseWriter, r *http.Request) {
	keys, err := s.store.ListAPIKeys(r.Context())
	if err != nil {
		s.logger.Error("list api keys failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to list accounts")
		return
	}

	out := make([]apiKeyResponse, len(keys))
	for i, k := range keys {
		out[i] = s.toAPIKeyResponse(k)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// HandleCreateAPIKey handles POST /api/v1/accounts.
func (s *Server) HandleCreateAPIKey(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ExchangeID int64  `json:"exchange_id"`
		Label      string `json:"label"`
		APIKey     string `json:"api_key"`
		APISecret  string `json:"api_secret"`
		Passphrase string `json:"passphrase"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if req.ExchangeID == 0 || req.Label == "" || req.APIKey == "" || req.APISecret == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "exchange_id, label, api_key and api_secret are required")
		return
	}

	// Verify the exchange exists.
	_, found, err := s.store.GetExchange(r.Context(), req.ExchangeID)
	if err != nil {
		s.logger.Error("get exchange for api key creation failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to verify exchange")
		return
	}
	if !found {
		s.writeError(w, http.StatusBadRequest, "bad_request", "exchange not found")
		return
	}

	// Encrypt sensitive fields.
	apiKeyEnc, err := s.encryptor.Encrypt(req.APIKey)
	if err != nil {
		s.logger.Error("encrypt api key failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "encryption failed")
		return
	}

	apiSecretEnc, err := s.encryptor.Encrypt(req.APISecret)
	if err != nil {
		s.logger.Error("encrypt api secret failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "encryption failed")
		return
	}

	var passphraseEnc string
	if req.Passphrase != "" {
		passphraseEnc, err = s.encryptor.Encrypt(req.Passphrase)
		if err != nil {
			s.logger.Error("encrypt passphrase failed", "error", err)
			s.writeError(w, http.StatusInternalServerError, "internal_error", "encryption failed")
			return
		}
	}

	key := adminstore.APIKey{
		ExchangeID:    req.ExchangeID,
		Label:         req.Label,
		APIKeyEnc:     apiKeyEnc,
		APISecretEnc:  apiSecretEnc,
		PassphraseEnc: passphraseEnc,
		Permissions:   "read,trade",
		Status:        "active",
	}

	id, err := s.store.CreateAPIKey(r.Context(), key)
	if err != nil {
		s.logger.Error("create api key failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to create account")
		return
	}

	created, _, err := s.store.GetAPIKey(r.Context(), id)
	if err != nil {
		s.logger.Error("get api key after create failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve created account")
		return
	}

	s.writeJSON(w, http.StatusCreated, s.toAPIKeyResponse(created))
}

// HandleGetAPIKey handles GET /api/v1/accounts/{id}.
func (s *Server) HandleGetAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "account_id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid account id")
		return
	}

	key, found, err := s.store.GetAPIKey(r.Context(), id)
	if err != nil {
		s.logger.Error("get api key failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get account")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}

	s.writeJSON(w, http.StatusOK, s.toAPIKeyResponse(key))
}

// HandleDeleteAPIKey handles DELETE /api/v1/accounts/{id}.
func (s *Server) HandleDeleteAPIKey(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "account_id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid account id")
		return
	}

	_, found, err := s.store.GetAPIKey(r.Context(), id)
	if err != nil {
		s.logger.Error("get api key for delete failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get account")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}

	if err := s.store.DeleteAPIKey(r.Context(), id); err != nil {
		s.logger.Error("delete api key failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete account")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

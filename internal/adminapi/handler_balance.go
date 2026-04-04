package adminapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"quant-system/internal/adapter"
)

// balanceResponse is the JSON response for account balance queries.
type balanceResponse struct {
	AccountID int64                 `json:"account_id"`
	Exchange  string                `json:"exchange"`
	Venue     string                `json:"venue"`
	Balances  []adapter.AssetBalance `json:"balances"`
	QueriedAt string                `json:"queried_at"`
}

// HandleGetBalance handles GET /api/v1/accounts/{id}/balance.
// It decrypts the API key from DB, creates a temporary gateway, and queries the balance.
func (s *Server) HandleGetBalance(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}

	// Parse account ID from URL path.
	idStr := s.pathParam(r, "account_id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid account id")
		return
	}

	// Load APIKey from store.
	key, found, err := s.store.GetAPIKey(r.Context(), id)
	if err != nil {
		s.logger.Error("get api key for balance failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get account")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "account not found")
		return
	}

	// Load the associated exchange to determine venue.
	exchange, found, err := s.store.GetExchange(r.Context(), key.ExchangeID)
	if err != nil {
		s.logger.Error("get exchange for balance failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get exchange")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "exchange not found")
		return
	}

	// Decrypt sensitive fields.
	apiKey, err := s.encryptor.Decrypt(key.APIKeyEnc)
	if err != nil {
		s.logger.Error("decrypt api key failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "decryption failed")
		return
	}

	apiSecret, err := s.encryptor.Decrypt(key.APISecretEnc)
	if err != nil {
		s.logger.Error("decrypt api secret failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "decryption failed")
		return
	}

	var passphrase string
	if key.PassphraseEnc != "" {
		passphrase, err = s.encryptor.Decrypt(key.PassphraseEnc)
		if err != nil {
			s.logger.Error("decrypt passphrase failed", "error", err)
			s.writeError(w, http.StatusInternalServerError, "internal_error", "decryption failed")
			return
		}
	}

	// Create the appropriate gateway and query balance.
	var querier adapter.BalanceQuerier

	venue := strings.ToLower(strings.TrimSpace(exchange.Venue))
	switch venue {
	case "binance":
		gw, gwErr := adapter.NewBinanceSpotTradeGateway(adapter.BinanceSpotRESTConfig{
			BaseURL:   "https://api.binance.com",
			APIKey:    apiKey,
			APISecret: apiSecret,
		}, nil)
		if gwErr != nil {
			s.logger.Error("create binance gateway failed", "error", gwErr)
			s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to create gateway")
			return
		}
		querier = gw
	case "okx":
		gw, gwErr := adapter.NewOKXSpotTradeGateway(adapter.OKXSpotRESTConfig{
			BaseURL:    "https://www.okx.com",
			APIKey:     apiKey,
			APISecret:  apiSecret,
			Passphrase: passphrase,
		}, nil)
		if gwErr != nil {
			s.logger.Error("create okx gateway failed", "error", gwErr)
			s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to create gateway")
			return
		}
		querier = gw
	default:
		s.writeError(w, http.StatusBadRequest, "bad_request", "unsupported venue: "+venue)
		return
	}

	balances, err := querier.QueryBalance(r.Context())
	if err != nil {
		s.logger.Error("query balance failed", "error", err, "account_id", id, "venue", venue)
		s.writeError(w, http.StatusBadGateway, "upstream_error", "failed to query balance: "+err.Error())
		return
	}

	if balances == nil {
		balances = []adapter.AssetBalance{}
	}

	s.writeJSON(w, http.StatusOK, balanceResponse{
		AccountID: id,
		Exchange:  exchange.Name,
		Venue:     venue,
		Balances:  balances,
		QueriedAt: time.Now().UTC().Format(time.RFC3339),
	})
}

package adminapi

import (
	"net/http"
)

// RiskConfigPayload is the JSON representation of the risk configuration.
type RiskConfigPayload struct {
	MaxOrderQty    float64  `json:"max_order_qty"`
	MaxOrderAmount float64  `json:"max_order_amount"`
	AllowedSymbols []string `json:"allowed_symbols"`
}

// HandleGetRiskConfig handles GET /api/v1/risk/config.
func (s *Server) HandleGetRiskConfig(w http.ResponseWriter, r *http.Request) {
	s.riskMu.RLock()
	cfg := s.riskCfg
	s.riskMu.RUnlock()

	s.writeJSON(w, http.StatusOK, cfg)
}

// HandleUpdateRiskConfig handles PUT /api/v1/risk/config.
func (s *Server) HandleUpdateRiskConfig(w http.ResponseWriter, r *http.Request) {
	var req RiskConfigPayload
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid JSON body")
		return
	}

	if req.MaxOrderQty < 0 {
		s.writeError(w, http.StatusBadRequest, "bad_request", "max_order_qty must be non-negative")
		return
	}
	if req.MaxOrderAmount < 0 {
		s.writeError(w, http.StatusBadRequest, "bad_request", "max_order_amount must be non-negative")
		return
	}

	s.riskMu.Lock()
	s.riskCfg = req
	s.riskMu.Unlock()

	s.logger.Info("risk config updated",
		"max_order_qty", req.MaxOrderQty,
		"max_order_amount", req.MaxOrderAmount,
		"allowed_symbols", req.AllowedSymbols,
	)

	s.writeJSON(w, http.StatusOK, req)
}

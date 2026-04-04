package adminapi

import (
	"net/http"
)

// HandleListPositions handles GET /api/v1/positions.
func (s *Server) HandleListPositions(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		s.writeJSON(w, http.StatusOK, []any{})
		return
	}

	positions, err := s.repo.LoadPositions(r.Context())
	if err != nil {
		s.logger.Error("list positions failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to list positions")
		return
	}
	s.writeJSON(w, http.StatusOK, positions)
}

// HandleListOrders handles GET /api/v1/orders.
func (s *Server) HandleListOrders(w http.ResponseWriter, r *http.Request) {
	if s.repo == nil {
		s.writeJSON(w, http.StatusOK, []any{})
		return
	}

	orders, err := s.repo.LoadOrders(r.Context())
	if err != nil {
		s.logger.Error("list orders failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to list orders")
		return
	}
	s.writeJSON(w, http.StatusOK, orders)
}

// overviewResponse is the JSON representation of the system overview.
type overviewResponse struct {
	RunningStrategies int              `json:"running_strategies"`
	TotalStrategies   int              `json:"total_strategies"`
	TotalPositions    int              `json:"total_positions"`
	TotalOrders       int              `json:"total_orders"`
	TotalRealizedPnL  float64          `json:"total_realized_pnl"`
	Exchanges         []exchangeEntry  `json:"exchanges"`
}

// exchangeEntry is a single exchange in the overview response.
type exchangeEntry struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

// HandleOverview handles GET /api/v1/overview.
func (s *Server) HandleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	// Fetch strategies.
	strategies, err := s.store.ListStrategyConfigs(ctx)
	if err != nil {
		s.logger.Error("overview: list strategies failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to load strategies")
		return
	}

	running := 0
	for _, cfg := range strategies {
		if cfg.Status == "running" {
			running++
		}
	}

	// Fetch exchanges.
	exchanges, err := s.store.ListExchanges(ctx)
	if err != nil {
		s.logger.Error("overview: list exchanges failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to load exchanges")
		return
	}

	exEntries := make([]exchangeEntry, len(exchanges))
	for i, ex := range exchanges {
		exEntries[i] = exchangeEntry{
			ID:     ex.ID,
			Name:   ex.Name,
			Status: ex.Status,
		}
	}

	resp := overviewResponse{
		RunningStrategies: running,
		TotalStrategies:   len(strategies),
		Exchanges:         exEntries,
	}

	// Fetch positions and orders from MySQL repo if available.
	if s.repo != nil {
		positions, err := s.repo.LoadPositions(ctx)
		if err != nil {
			s.logger.Error("overview: load positions failed", "error", err)
		} else {
			resp.TotalPositions = len(positions)
			for _, pos := range positions {
				resp.TotalRealizedPnL += pos.RealizedPnL
			}
		}

		orders, err := s.repo.LoadOrders(ctx)
		if err != nil {
			s.logger.Error("overview: load orders failed", "error", err)
		} else {
			resp.TotalOrders = len(orders)
		}
	}

	s.writeJSON(w, http.StatusOK, resp)
}

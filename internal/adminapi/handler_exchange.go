package adminapi

import (
	"net/http"
	"strconv"

	"quant-system/internal/adminstore"
)

// exchangeResponse is the JSON representation of an exchange.
type exchangeResponse struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Venue     string `json:"venue"`
	Status    string `json:"status"`
	CreatedMS int64  `json:"created_ms"`
	UpdatedMS int64  `json:"updated_ms"`
}

// toExchangeResponse converts a store Exchange to its JSON representation.
func toExchangeResponse(ex adminstore.Exchange) exchangeResponse {
	return exchangeResponse{
		ID:        ex.ID,
		Name:      ex.Name,
		Venue:     ex.Venue,
		Status:    ex.Status,
		CreatedMS: ex.CreatedMS,
		UpdatedMS: ex.UpdatedMS,
	}
}

// HandleListExchanges handles GET /api/v1/exchanges.
func (s *Server) HandleListExchanges(w http.ResponseWriter, r *http.Request) {
	exchanges, err := s.store.ListExchanges(r.Context())
	if err != nil {
		s.logger.Error("list exchanges failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to list exchanges")
		return
	}

	out := make([]exchangeResponse, len(exchanges))
	for i, ex := range exchanges {
		out[i] = toExchangeResponse(ex)
	}
	s.writeJSON(w, http.StatusOK, out)
}

// HandleCreateExchange handles POST /api/v1/exchanges.
func (s *Server) HandleCreateExchange(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name  string `json:"name"`
		Venue string `json:"venue"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if req.Name == "" || req.Venue == "" {
		s.writeError(w, http.StatusBadRequest, "bad_request", "name and venue are required")
		return
	}

	ex := adminstore.Exchange{
		Name:   req.Name,
		Venue:  req.Venue,
		Status: "active",
	}

	id, err := s.store.CreateExchange(r.Context(), ex)
	if err != nil {
		s.logger.Error("create exchange failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to create exchange")
		return
	}

	created, _, err := s.store.GetExchange(r.Context(), id)
	if err != nil {
		s.logger.Error("get exchange after create failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve created exchange")
		return
	}

	s.writeJSON(w, http.StatusCreated, toExchangeResponse(created))
}

// HandleGetExchange handles GET /api/v1/exchanges/{id}.
func (s *Server) HandleGetExchange(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "exchange_id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid exchange id")
		return
	}

	ex, found, err := s.store.GetExchange(r.Context(), id)
	if err != nil {
		s.logger.Error("get exchange failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get exchange")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "exchange not found")
		return
	}

	s.writeJSON(w, http.StatusOK, toExchangeResponse(ex))
}

// HandleUpdateExchange handles PUT /api/v1/exchanges/{id}.
func (s *Server) HandleUpdateExchange(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "exchange_id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid exchange id")
		return
	}

	existing, found, err := s.store.GetExchange(r.Context(), id)
	if err != nil {
		s.logger.Error("get exchange for update failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get exchange")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "exchange not found")
		return
	}

	var req struct {
		Name   string `json:"name"`
		Venue  string `json:"venue"`
		Status string `json:"status"`
	}
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	// Apply provided fields, keeping existing values as defaults.
	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Venue != "" {
		existing.Venue = req.Venue
	}
	if req.Status != "" {
		if req.Status != "active" && req.Status != "disabled" {
			s.writeError(w, http.StatusBadRequest, "bad_request", "status must be active or disabled")
			return
		}
		existing.Status = req.Status
	}

	if err := s.store.UpdateExchange(r.Context(), existing); err != nil {
		s.logger.Error("update exchange failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to update exchange")
		return
	}

	updated, _, err := s.store.GetExchange(r.Context(), id)
	if err != nil {
		s.logger.Error("get exchange after update failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to retrieve updated exchange")
		return
	}

	s.writeJSON(w, http.StatusOK, toExchangeResponse(updated))
}

// HandleDeleteExchange handles DELETE /api/v1/exchanges/{id}.
func (s *Server) HandleDeleteExchange(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(s.pathParam(r, "exchange_id"), 10, 64)
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", "invalid exchange id")
		return
	}

	_, found, err := s.store.GetExchange(r.Context(), id)
	if err != nil {
		s.logger.Error("get exchange for delete failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to get exchange")
		return
	}
	if !found {
		s.writeError(w, http.StatusNotFound, "not_found", "exchange not found")
		return
	}

	if err := s.store.DeleteExchange(r.Context(), id); err != nil {
		s.logger.Error("delete exchange failed", "error", err)
		s.writeError(w, http.StatusInternalServerError, "internal_error", "failed to delete exchange")
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

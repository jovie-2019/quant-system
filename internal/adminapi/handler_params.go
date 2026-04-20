package adminapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"quant-system/internal/adminstore"
	"quant-system/internal/bus/natsbus"
	"quant-system/pkg/contracts"
)

// Hot-reload / lifecycle-control REST handlers.
//
// The /api/v1/strategies/:id/params endpoint lets an operator (or the
// scheduled optimiser pipeline) propose a new parameter set for a live
// strategy WITHOUT restarting its process. The flow:
//
//   1. Client POSTs {"params": {...}, "reason": "...", "type": "update_params"}
//   2. Server allocates the next revision atomically from MySQL.
//   3. Server writes an audit row with status=pending.
//   4. Server publishes a StrategyControlEnvelope on NATS
//      subject strategy.control.<id>.
//   5. The runner handles the command, applies it to the live Strategy,
//      and publishes a StrategyControlAck.
//   6. A background subscriber (StartControlAckListener) updates the
//      audit row with the ack outcome.
//
// GET /api/v1/strategies/:id/revisions returns the full audit trail.

// StrategyParamsRequest is the body of POST /api/v1/strategies/:id/params.
type StrategyParamsRequest struct {
	// Type defaults to "update_params". Other values let the UI issue
	// pause / resume / shadow_on / shadow_off commands through the same
	// channel.
	Type contracts.StrategyControlType `json:"type"`
	// Params is the new parameter set for update_params. Ignored for
	// pause / resume / shadow commands.
	Params json.RawMessage `json:"params,omitempty"`
	// Reason is a short operator-supplied note stored in the audit log.
	Reason string `json:"reason,omitempty"`
	// Actor is filled from the JWT subject when available; clients may
	// leave it empty.
	Actor string `json:"actor,omitempty"`
}

// StrategyRevisionResponse is the payload of GET .../revisions.
type StrategyRevisionResponse struct {
	Items []RevisionRow `json:"items"`
	Count int           `json:"count"`
}

// RevisionRow is the JSON view of an audit entry.
type RevisionRow struct {
	ID            int64  `json:"id"`
	StrategyID    string `json:"strategy_id"`
	Revision      int64  `json:"revision"`
	CommandType   string `json:"command_type"`
	ParamsBefore  string `json:"params_before,omitempty"`
	ParamsAfter   string `json:"params_after,omitempty"`
	Actor         string `json:"actor"`
	Reason        string `json:"reason,omitempty"`
	IssuedMS      int64  `json:"issued_ms"`
	AckReceivedMS int64  `json:"ack_received_ms,omitempty"`
	AckAccepted   *bool  `json:"ack_accepted,omitempty"`
	AckError      string `json:"ack_error,omitempty"`
}

// HandleProposeStrategyParams implements POST /api/v1/strategies/:id/params.
// The path expects a numeric id matching strategy_configs.id, mirroring
// the other /strategies/:id/... routes.
func (s *Server) HandleProposeStrategyParams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}
	if s.bus == nil {
		s.writeError(w, http.StatusServiceUnavailable, "bus_unavailable",
			"hot-reload requires NATS; start admin-api with NATS_URL set")
		return
	}
	id, err := parseTrailingID(r.URL.Path, "/api/v1/strategies/", "/params")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}

	var req StrategyParamsRequest
	if err := s.readJSON(r, &req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if err := validateParamsRequest(req); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	if req.Actor == "" {
		if uid, ok := userIDFromContext(r.Context()); ok {
			req.Actor = uid
		} else {
			req.Actor = "anonymous"
		}
	}

	nextRev, err := s.store.NextRevision(r.Context(), cfg.StrategyID)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	// Write audit row BEFORE publishing so an ack that races in never
	// references a non-existent revision.
	paramsBefore := cfg.ConfigJSON
	paramsAfter := string(req.Params)
	revRow := adminstore.ParamRevision{
		StrategyID:   cfg.StrategyID,
		Revision:     nextRev,
		CommandType:  string(req.Type),
		ParamsBefore: paramsBefore,
		ParamsAfter:  paramsAfter,
		Actor:        req.Actor,
		Reason:       req.Reason,
		IssuedMS:     time.Now().UTC().UnixMilli(),
	}
	if _, err := s.store.CreateParamRevision(r.Context(), revRow); err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}

	env := contracts.StrategyControlEnvelope{
		StrategyID: cfg.StrategyID,
		Type:       req.Type,
		Revision:   nextRev,
		Params:     req.Params,
		Reason:     req.Reason,
		Actor:      req.Actor,
		IssuedMS:   revRow.IssuedMS,
	}
	if err := natsbus.PublishStrategyControl(r.Context(), s.bus, env, nil); err != nil {
		s.writeError(w, http.StatusInternalServerError, "nats_publish_failed", err.Error())
		return
	}

	// When it's an update_params command, also mirror the new params into
	// strategy_configs so a future runner restart picks them up. Other
	// command types (pause / shadow) are ephemeral and don't persist.
	if req.Type == contracts.StrategyControlUpdateParams && len(req.Params) > 0 {
		cfg.ConfigJSON = string(req.Params)
		if err := s.store.UpdateStrategyConfig(r.Context(), cfg); err != nil {
			s.logger.Warn("params: update config_json failed (ack still pending)",
				"error", err, "strategy_id", cfg.StrategyID)
		}
	}

	s.writeJSON(w, http.StatusAccepted, map[string]any{
		"strategy_id": cfg.StrategyID,
		"revision":    nextRev,
		"issued_ms":   revRow.IssuedMS,
		"status":      "pending_ack",
	})
}

// HandleListStrategyRevisions implements GET /api/v1/strategies/:id/revisions.
func (s *Server) HandleListStrategyRevisions(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "GET required")
		return
	}
	id, err := parseTrailingID(r.URL.Path, "/api/v1/strategies/", "/revisions")
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}
	cfg, ok, err := s.store.GetStrategyConfig(r.Context(), id)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	if !ok {
		s.writeError(w, http.StatusNotFound, "not_found", "strategy not found")
		return
	}
	limit := 50
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 && v <= 500 {
			limit = v
		}
	}
	rows, err := s.store.ListParamRevisions(r.Context(), cfg.StrategyID, limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "db_error", err.Error())
		return
	}
	resp := StrategyRevisionResponse{Count: len(rows)}
	for _, r := range rows {
		item := RevisionRow{
			ID:           r.ID,
			StrategyID:   r.StrategyID,
			Revision:     r.Revision,
			CommandType:  r.CommandType,
			ParamsBefore: r.ParamsBefore,
			ParamsAfter:  r.ParamsAfter,
			Actor:        r.Actor,
			Reason:       r.Reason,
			IssuedMS:     r.IssuedMS,
		}
		if r.AckReceivedMS.Valid {
			item.AckReceivedMS = r.AckReceivedMS.Int64
		}
		if r.AckAccepted.Valid {
			b := r.AckAccepted.Bool
			item.AckAccepted = &b
		}
		if r.AckError.Valid {
			item.AckError = r.AckError.String
		}
		resp.Items = append(resp.Items, item)
	}
	s.writeJSON(w, http.StatusOK, resp)
}

// StartControlAckListener subscribes to strategy.control.ack.> and writes
// incoming acks into the audit log. Callers (cmd/admin-api/main.go) invoke
// this once at startup after the server is constructed; the subscription
// is cancelled when ctx is cancelled.
func (s *Server) StartControlAckListener(ctx context.Context) error {
	if s.bus == nil {
		return errors.New("adminapi: bus is nil, ack listener not started")
	}
	_, err := s.bus.Subscribe(ctx, "strategy.control.ack.>", natsbus.SubscribeConfig{
		Durable: "admin-api-ctl-acks",
		AckWait: 5 * time.Second,
	}, func(ctx context.Context, msg natsbus.Message) error {
		var ack contracts.StrategyControlAck
		if err := json.Unmarshal(msg.Data, &ack); err != nil {
			s.logger.Warn("ack listener: bad payload", "error", err)
			return nil
		}
		if err := s.store.UpdateParamRevisionAck(ctx,
			ack.StrategyID, ack.Revision, ack.Accepted, ack.Error, ack.AppliedMS,
		); err != nil {
			s.logger.Warn("ack listener: update audit failed",
				"error", err, "strategy_id", ack.StrategyID, "rev", ack.Revision)
		}
		return nil
	})
	return err
}

func validateParamsRequest(req StrategyParamsRequest) error {
	switch req.Type {
	case contracts.StrategyControlUpdateParams:
		if len(req.Params) == 0 {
			return errors.New("params required for update_params")
		}
		// Sanity check: must be a JSON object.
		var probe map[string]any
		if err := json.Unmarshal(req.Params, &probe); err != nil {
			return fmt.Errorf("params must be a JSON object: %w", err)
		}
	case contracts.StrategyControlPause,
		contracts.StrategyControlResume,
		contracts.StrategyControlShadowOn,
		contracts.StrategyControlShadowOff:
		// No params expected.
	case "":
		return errors.New("type is required")
	default:
		return fmt.Errorf("unknown control type %q", req.Type)
	}
	return nil
}

// parseTrailingID extracts the id from `<prefix><id><suffix>` URL paths.
// For example, "/api/v1/strategies/42/params" with prefix
// "/api/v1/strategies/" and suffix "/params" yields 42.
func parseTrailingID(path, prefix, suffix string) (int64, error) {
	rest := strings.TrimPrefix(path, prefix)
	rest = strings.TrimSuffix(rest, suffix)
	if rest == path || rest == "" {
		return 0, fmt.Errorf("cannot extract id from %q", path)
	}
	id, err := strconv.ParseInt(rest, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid id %q", rest)
	}
	return id, nil
}

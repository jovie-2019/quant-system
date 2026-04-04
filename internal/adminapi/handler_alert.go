package adminapi

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// alertmanagerPayload represents the Alertmanager webhook JSON body.
type alertmanagerPayload struct {
	Status string              `json:"status"`
	Alerts []alertmanagerAlert `json:"alerts"`
}

// alertmanagerAlert represents a single alert from Alertmanager.
type alertmanagerAlert struct {
	Status      string            `json:"status"`
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
	StartsAt    time.Time         `json:"startsAt"`
}

// HandleAlertWebhook handles POST /api/v1/alerts/webhook.
// It receives Alertmanager webhook format and forwards each alert to Feishu.
func (s *Server) HandleAlertWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		s.writeError(w, http.StatusMethodNotAllowed, "method_not_allowed", "POST required")
		return
	}

	var payload alertmanagerPayload
	if err := s.readJSON(r, &payload); err != nil {
		s.writeError(w, http.StatusBadRequest, "bad_request", err.Error())
		return
	}

	if s.feishu == nil {
		s.logger.Warn("alert webhook received but feishu client not configured")
		s.writeJSON(w, http.StatusOK, map[string]string{"status": "skipped", "reason": "feishu not configured"})
		return
	}

	var errs []string
	for _, alert := range payload.Alerts {
		level := mapSeverity(alert.Labels["severity"])
		title := alert.Labels["alertname"]
		if title == "" {
			title = "Unnamed Alert"
		}

		content := buildAlertContent(alert)

		if err := s.feishu.SendAlert(r.Context(), level, title, content); err != nil {
			s.logger.Error("failed to send alert to feishu", "alertname", title, "error", err)
			errs = append(errs, fmt.Sprintf("%s: %v", title, err))
		}
	}

	if len(errs) > 0 {
		s.writeError(w, http.StatusInternalServerError, "partial_failure",
			fmt.Sprintf("failed to send %d/%d alerts: %s", len(errs), len(payload.Alerts), strings.Join(errs, "; ")))
		return
	}

	s.writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// mapSeverity maps Alertmanager severity labels to Feishu alert levels.
func mapSeverity(severity string) string {
	switch severity {
	case "critical":
		return "critical"
	case "warning":
		return "warning"
	default:
		return "info"
	}
}

// buildAlertContent formats alert details into a readable string.
func buildAlertContent(alert alertmanagerAlert) string {
	var parts []string
	if summary := alert.Annotations["summary"]; summary != "" {
		parts = append(parts, fmt.Sprintf("**Summary:** %s", summary))
	}
	if desc := alert.Annotations["description"]; desc != "" {
		parts = append(parts, fmt.Sprintf("**Description:** %s", desc))
	}
	parts = append(parts, fmt.Sprintf("**Status:** %s", alert.Status))
	parts = append(parts, fmt.Sprintf("**StartsAt:** %s", alert.StartsAt.Format(time.RFC3339)))
	return strings.Join(parts, "\n")
}

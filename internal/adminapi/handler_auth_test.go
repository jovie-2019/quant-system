package adminapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleLogin_Success(t *testing.T) {
	srv := testServer(t)

	body := `{"password":"test123"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.HandleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if resp["token"] == "" {
		t.Error("expected non-empty token")
	}
	if resp["expires_at"] == "" {
		t.Error("expected non-empty expires_at")
	}

	// Verify the returned token is valid.
	subject, err := srv.parseJWT(resp["token"])
	if err != nil {
		t.Fatalf("parse returned token: %v", err)
	}
	if subject != "admin" {
		t.Errorf("expected subject=admin, got %s", subject)
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	srv := testServer(t)

	body := `{"password":"wrong-password"}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.HandleLogin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["error"] != "invalid_credentials" {
		t.Errorf("expected error=invalid_credentials, got %s", resp["error"])
	}
}

func TestHandleLogin_InvalidJSON(t *testing.T) {
	srv := testServer(t)

	body := `{not valid json}`
	req := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	srv.HandleLogin(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

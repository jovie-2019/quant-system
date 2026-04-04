package adminapi

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func testServer(t *testing.T) *Server {
	t.Helper()
	hash, err := bcrypt.GenerateFromPassword([]byte("test123"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt hash: %v", err)
	}
	return &Server{
		jwtSecret: []byte("test-secret-key-for-testing-only"),
		passHash:  string(hash),
		logger:    slog.Default(),
	}
}

func TestJWTMiddleware_ValidToken(t *testing.T) {
	srv := testServer(t)

	token, _, err := srv.generateJWT("admin", time.Hour)
	if err != nil {
		t.Fatalf("generate JWT: %v", err)
	}

	handler := srv.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		uid, ok := userIDFromContext(r.Context())
		if !ok {
			t.Error("expected userID in context")
			return
		}
		if uid != "admin" {
			t.Errorf("expected userID=admin, got %s", uid)
		}
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
}

func TestJWTMiddleware_MissingHeader(t *testing.T) {
	srv := testServer(t)

	handler := srv.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_ExpiredToken(t *testing.T) {
	srv := testServer(t)

	// Generate a token that expired 1 hour ago.
	token, _, err := srv.generateJWT("admin", -time.Hour)
	if err != nil {
		t.Fatalf("generate JWT: %v", err)
	}

	handler := srv.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestJWTMiddleware_InvalidSignature(t *testing.T) {
	srv := testServer(t)

	// Generate a valid token then tamper with the signature.
	token, _, err := srv.generateJWT("admin", time.Hour)
	if err != nil {
		t.Fatalf("generate JWT: %v", err)
	}

	// Corrupt the last character of the signature.
	tampered := token[:len(token)-1] + "X"

	handler := srv.JWTMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("handler should not be called")
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Authorization", "Bearer "+tampered)
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

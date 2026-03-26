package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
)

func TestAuthRejectsNoToken(t *testing.T) {
	app := newAuthTestApp(t, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "missing_token") {
		t.Fatalf("expected missing_token, got %s", rec.Body.String())
	}
}

func TestAuthRejectsWrongToken(t *testing.T) {
	app := newAuthTestApp(t, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	req.Header.Set("Authorization", "Bearer wrong")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "invalid_token") {
		t.Fatalf("expected invalid_token, got %s", rec.Body.String())
	}
}

func TestAuthAcceptsValidToken(t *testing.T) {
	app := newAuthTestApp(t, "secret")

	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAuthPassthroughWhenNotConfigured(t *testing.T) {
	app := newAuthTestApp(t, "")

	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestHealthzNotProtected(t *testing.T) {
	app := newAuthTestApp(t, "secret")

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func newAuthTestApp(t *testing.T, authToken string) *App {
	t.Helper()

	fsys, contractDir := writeTestFixtures(t)
	return mustNewApp(t, config.Config{
		Addr:         ":0",
		FS:           fsys,
		ContractDir:  contractDir,
		QueryTimeout: 5 * time.Second,
		AuthToken:    authToken,
	})
}

package server

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
)

func TestStaticHandlerServesIndexAndAssets(t *testing.T) {
	fsys, contractDir := writeTestFixtures(t)
	app := mustNewApp(t, config.Config{
		Addr:         ":0",
		FS:           fsys,
		ContractDir:  contractDir,
		QueryTimeout: 5 * time.Second,
		WebFS: fstest.MapFS{
			"index.html": {
				Data: []byte("<!doctype html><html><body>ptd</body></html>"),
			},
			"assets/app.js": {
				Data: []byte("console.log('ptd');"),
			},
		},
	})

	t.Run("root serves index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("expected html index, got %s", rec.Body.String())
		}
	})

	t.Run("spa route falls back to index", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/dashboard/fi-origin-monthly", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("expected html index, got %s", rec.Body.String())
		}
	})

	t.Run("asset serves exact file", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), "console.log('ptd');") {
			t.Fatalf("expected asset contents, got %s", rec.Body.String())
		}
	})

	t.Run("api route still wins", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
		rec := httptest.NewRecorder()
		app.Handler().ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
		}
		if strings.Contains(rec.Body.String(), "<!doctype html>") {
			t.Fatalf("expected api json, got html: %s", rec.Body.String())
		}
	})
}

func TestStaticHandlerNotRegisteredWithoutWebFS(t *testing.T) {
	fsys, contractDir := writeTestFixtures(t)
	app := mustNewApp(t, config.Config{
		Addr:         ":0",
		FS:           fsys,
		ContractDir:  contractDir,
		QueryTimeout: 5 * time.Second,
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d body=%s", rec.Code, rec.Body.String())
	}
}

var _ fs.FS = fstest.MapFS{}

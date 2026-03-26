package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"
	"time"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/catalog"
	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
)

func TestCacheKeyDeterministic(t *testing.T) {
	keyA := cacheKey("fi-origin-monthly", mustValues(t, "posting_from=202601&company_code=1000&limit=10"))
	keyB := cacheKey("fi-origin-monthly", mustValues(t, "limit=10&company_code=1000&posting_from=202601"))

	if keyA != keyB {
		t.Fatalf("expected matching keys, got %q and %q", keyA, keyB)
	}
}

func TestCacheSetAndGet(t *testing.T) {
	cache := newResponseCache()
	cache.Set("dataset", []byte(`{"ok":true}`), time.Minute)

	body, ok := cache.Get("dataset")
	if !ok {
		t.Fatal("expected cache hit")
	}
	if string(body) != `{"ok":true}` {
		t.Fatalf("unexpected cache body: %s", string(body))
	}
}

func TestCacheExpiration(t *testing.T) {
	cache := newResponseCache()
	cache.entries["dataset"] = cacheEntry{
		body:      []byte(`{"ok":true}`),
		createdAt: time.Now().Add(-2 * time.Minute),
		ttl:       time.Minute,
	}

	if _, ok := cache.Get("dataset"); ok {
		t.Fatal("expected expired entry to miss")
	}
}

func TestCacheSweep(t *testing.T) {
	cache := newResponseCache()
	cache.entries["expired"] = cacheEntry{
		body:      []byte(`{"ok":true}`),
		createdAt: time.Now().Add(-2 * time.Minute),
		ttl:       time.Minute,
	}
	cache.entries["fresh"] = cacheEntry{
		body:      []byte(`{"ok":true}`),
		createdAt: time.Now(),
		ttl:       time.Minute,
	}

	cache.sweep()

	if _, ok := cache.entries["expired"]; ok {
		t.Fatal("expected expired entry to be removed")
	}
	if _, ok := cache.entries["fresh"]; !ok {
		t.Fatal("expected fresh entry to remain")
	}
}

func TestCacheBypassHeader(t *testing.T) {
	app := newCacheTestApp(t)
	seedDatasetRowsCache(t, app, mustValues(t, "limit=10"))

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/fi-origin-monthly/rows?limit=10", nil)
	req.Header.Set("Cache-Control", "no-cache")
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Cache") != "MISS" {
		t.Fatalf("expected cache miss, got %q", rec.Header().Get("X-Cache"))
	}

	var payload struct {
		RowCount int `json:"row_count"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.RowCount != 1 {
		t.Fatalf("expected one row, got %d", payload.RowCount)
	}
}

func TestCacheHit(t *testing.T) {
	app := newCacheTestApp(t)
	seedDatasetRowsCache(t, app, mustValues(t, "limit=10"))

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/fi-origin-monthly/rows?limit=10", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Cache") != "HIT" {
		t.Fatalf("expected cache hit, got %q", rec.Header().Get("X-Cache"))
	}

	var payload struct {
		Cached bool `json:"cached"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if !payload.Cached {
		t.Fatalf("expected cached payload, got %s", rec.Body.String())
	}
}

func TestNocacheQueryParam(t *testing.T) {
	app := newCacheTestApp(t)
	seedDatasetRowsCache(t, app, mustValues(t, "limit=10&nocache=true"))

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/fi-origin-monthly/rows?limit=10&nocache=true", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("X-Cache") != "MISS" {
		t.Fatalf("expected cache miss, got %q", rec.Header().Get("X-Cache"))
	}
}

func newCacheTestApp(t *testing.T) *App {
	t.Helper()

	registerCacheTestDriver()

	fsys, contractDir := writeNamedFixtures(t, namedFixture{
		id:             "fi-origin-monthly",
		title:          "FI document origin monthly",
		domain:         "fi",
		plannedFilters: []string{"posting_from", "posting_to", "company_code", "fi_origin"},
		columns:        []string{"posting_yyyymm", "company_code", "fi_origin"},
		sqlText:        "SELECT posting_yyyymm, company_code, fi_origin FROM some_source",
	})

	cat, err := catalog.Load(fsys, contractDir)
	if err != nil {
		t.Fatalf("load catalog: %v", err)
	}

	db, err := sql.Open(cacheTestDriverName, "")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}

	app := &App{
		cfg: config.Config{
			Addr:         ":0",
			FS:           fsys,
			ContractDir:  contractDir,
			QueryTimeout: 5 * time.Second,
		},
		catalog:   cat,
		db:        db,
		mux:       http.NewServeMux(),
		cache:     newResponseCache(),
		cacheStop: make(chan struct{}),
	}
	app.cache.StartSweep(time.Minute, app.cacheStop)
	app.routes()

	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Fatalf("close app: %v", err)
		}
	})

	return app
}

func seedDatasetRowsCache(t *testing.T, app *App, query url.Values) {
	t.Helper()

	body, err := json.MarshalIndent(map[string]any{
		"dataset_id": "fi-origin-monthly",
		"cached":     true,
	}, "", "  ")
	if err != nil {
		t.Fatalf("marshal cache body: %v", err)
	}
	body = append(body, '\n')

	app.cache.Set(cacheKey("fi-origin-monthly", query), body, time.Minute)
}

const cacheTestDriverName = "ptd-api-cache-test"

var registerCacheDriverOnce sync.Once

func registerCacheTestDriver() {
	registerCacheDriverOnce.Do(func() {
		sql.Register(cacheTestDriverName, cacheTestDriver{})
	})
}

type cacheTestDriver struct{}

func (cacheTestDriver) Open(string) (driver.Conn, error) {
	return cacheTestConn{}, nil
}

type cacheTestConn struct{}

func (cacheTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (cacheTestConn) Close() error {
	return nil
}

func (cacheTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (cacheTestConn) QueryContext(context.Context, string, []driver.NamedValue) (driver.Rows, error) {
	return &cacheTestRows{
		columns: []string{"posting_yyyymm", "company_code", "fi_origin"},
		rows: [][]driver.Value{
			{"202603", "1000", "MKPF"},
		},
	}, nil
}

type cacheTestRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *cacheTestRows) Columns() []string {
	return r.columns
}

func (r *cacheTestRows) Close() error {
	return nil
}

func (r *cacheTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}

	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = cacheTestConn{}

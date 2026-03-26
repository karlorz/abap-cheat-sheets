package server

import (
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
)

func TestListDatasets(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datasets", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var payload struct {
		Count int `json:"count"`
		Items []struct {
			ID                string   `json:"id"`
			FilterSupport     string   `json:"filter_support"`
			ExecutableFilters []string `json:"executable_filters"`
		} `json:"items"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if payload.Count != 1 {
		t.Fatalf("expected count 1, got %d", payload.Count)
	}
	if len(payload.Items) != 1 || payload.Items[0].ID != "test-dataset" {
		t.Fatalf("unexpected payload items: %+v", payload.Items)
	}
	if payload.Items[0].FilterSupport != "limit_only" {
		t.Fatalf("unexpected filter support: %+v", payload.Items[0])
	}
	if len(payload.Items[0].ExecutableFilters) != 0 {
		t.Fatalf("expected no executable filters, got %+v", payload.Items[0].ExecutableFilters)
	}
}

func TestGetDatasetRowsWithoutDSN(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/test-dataset/rows?limit=10", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "PTD_SQLSERVER_DSN") {
		t.Fatalf("expected sql server hint in body, got %s", rec.Body.String())
	}
}

func TestGetDatasetRejectsUnsupportedFilters(t *testing.T) {
	app := newTestAppWithDSN(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/test-dataset/rows?company_code=1000", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}

	if !strings.Contains(rec.Body.String(), "unsupported_query_params") {
		t.Fatalf("expected unsupported params error, got %s", rec.Body.String())
	}
}

func TestGetDatasetOmitsRepoRoot(t *testing.T) {
	app := newTestApp(t)

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/test-dataset", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	if strings.Contains(rec.Body.String(), "repo_root") {
		t.Fatalf("expected repo_root to stay server-side, got %s", rec.Body.String())
	}
}

func TestBuildDatasetQueryFIOriginMonthly(t *testing.T) {
	query, args, err := buildDatasetQuery(
		"fi-origin-monthly",
		"SELECT posting_yyyymm, company_code, fi_origin FROM some_source",
		25,
		mustValues(t, "posting_from=202601&posting_to=202603&company_code=1000&fi_origin=mkpf"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	expectedParts := []string{
		"SELECT TOP (25) * FROM (",
		"WHERE dataset_rows.posting_yyyymm >= @p1 AND dataset_rows.posting_yyyymm <= @p2 AND dataset_rows.company_code = @p3 AND dataset_rows.fi_origin = @p4",
		"ORDER BY dataset_rows.posting_yyyymm DESC, dataset_rows.company_code ASC, dataset_rows.fi_origin ASC",
	}
	for _, part := range expectedParts {
		if !strings.Contains(query, part) {
			t.Fatalf("expected query to contain %q, got %s", part, query)
		}
	}

	expectedArgs := []any{"202601", "202603", "1000", "MKPF"}
	if len(args) != len(expectedArgs) {
		t.Fatalf("unexpected arg count %d", len(args))
	}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryAROpenMonthly(t *testing.T) {
	query, args, err := buildDatasetQuery(
		"ar-open-monthly",
		"SELECT posting_yyyymm, company_code, currency_code, document_type FROM some_source",
		50,
		mustValues(t, "posting_from=202601&company_code=1000&currency_code=hkd&document_type=dr"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	if !strings.Contains(query, "dataset_rows.currency_code = @p3") {
		t.Fatalf("expected currency filter in query, got %s", query)
	}
	if !strings.Contains(query, "dataset_rows.document_type = @p4") {
		t.Fatalf("expected document type filter in query, got %s", query)
	}

	expectedArgs := []any{"202601", "1000", "HKD", "DR"}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryFIOriginMonthlySupportsBlankToken(t *testing.T) {
	_, args, err := buildDatasetQuery(
		"fi-origin-monthly",
		"SELECT posting_yyyymm, company_code, fi_origin FROM some_source",
		25,
		mustValues(t, "fi_origin=%5Bblank%5D"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	expectedArgs := []any{"[blank]"}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryARClearedMonthly(t *testing.T) {
	query, args, err := buildDatasetQuery(
		"ar-cleared-monthly",
		"SELECT clearing_yyyymm, company_code, original_document_type, clearing_document_type FROM some_source",
		30,
		mustValues(t, "clearing_from=202601&clearing_to=202603&company_code=1000&original_document_type=rv&clearing_document_type=dz"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	expectedParts := []string{
		"dataset_rows.clearing_yyyymm >= @p1",
		"dataset_rows.clearing_yyyymm <= @p2",
		"dataset_rows.company_code = @p3",
		"dataset_rows.original_document_type = @p4",
		"dataset_rows.clearing_document_type = @p5",
	}
	for _, part := range expectedParts {
		if !strings.Contains(query, part) {
			t.Fatalf("expected query to contain %q, got %s", part, query)
		}
	}

	expectedArgs := []any{"202601", "202603", "1000", "RV", "DZ"}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryPurchasingHistoryMonthly(t *testing.T) {
	query, args, err := buildDatasetQuery(
		"purchasing-history-monthly",
		"SELECT posting_yyyymm, history_category, plant_code FROM some_source",
		40,
		mustValues(t, "posting_from=202601&posting_to=202603&history_category=2&plant_code=6110"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	expectedParts := []string{
		"dataset_rows.posting_yyyymm >= @p1",
		"dataset_rows.posting_yyyymm <= @p2",
		"dataset_rows.history_category = @p3",
		"dataset_rows.plant_code = @p4",
	}
	for _, part := range expectedParts {
		if !strings.Contains(query, part) {
			t.Fatalf("expected query to contain %q, got %s", part, query)
		}
	}

	expectedArgs := []any{"202601", "202603", "2", "6110"}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryPurchasingHistoryMonthlySupportsBlankPlantCode(t *testing.T) {
	_, args, err := buildDatasetQuery(
		"purchasing-history-monthly",
		"SELECT posting_yyyymm, history_category, plant_code FROM some_source",
		40,
		mustValues(t, "plant_code=%5Bblank%5D"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	expectedArgs := []any{"[blank]"}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryCurrentStock(t *testing.T) {
	query, args, err := buildDatasetQuery(
		"current-stock",
		"SELECT plant_code, storage_location FROM some_source",
		15,
		mustValues(t, "plant_code=6110&storage_location=3g"),
	)
	if err != nil {
		t.Fatalf("build query: %v", err)
	}

	expectedParts := []string{
		"dataset_rows.plant_code = @p1",
		"dataset_rows.storage_location = @p2",
		"ORDER BY dataset_rows.plant_code ASC, dataset_rows.storage_location ASC",
	}
	for _, part := range expectedParts {
		if !strings.Contains(query, part) {
			t.Fatalf("expected query to contain %q, got %s", part, query)
		}
	}

	expectedArgs := []any{"6110", "3G"}
	for i := range expectedArgs {
		if args[i] != expectedArgs[i] {
			t.Fatalf("unexpected arg %d: got %#v want %#v", i, args[i], expectedArgs[i])
		}
	}
}

func TestBuildDatasetQueryRejectsInvalidPostingFrom(t *testing.T) {
	_, _, err := buildDatasetQuery(
		"fi-origin-monthly",
		"SELECT posting_yyyymm, company_code, fi_origin FROM some_source",
		25,
		mustValues(t, "posting_from=2026AA"),
	)
	if err == nil {
		t.Fatal("expected invalid posting_from error")
	}
	if !strings.Contains(err.Error(), "posting_from") {
		t.Fatalf("expected posting_from error, got %v", err)
	}
}

func TestBuildDatasetQueryRejectsInvalidPlantCode(t *testing.T) {
	_, _, err := buildDatasetQuery(
		"current-stock",
		"SELECT plant_code, storage_location FROM some_source",
		10,
		mustValues(t, "plant_code=61100"),
	)
	if err == nil {
		t.Fatal("expected invalid plant_code error")
	}
	if !strings.Contains(err.Error(), "plant_code") {
		t.Fatalf("expected plant_code error, got %v", err)
	}
}

func TestGetDatasetRowsDryRun(t *testing.T) {
	app := newNamedTestApp(t, namedFixture{
		id:             "fi-origin-monthly",
		title:          "FI document origin monthly",
		domain:         "fi",
		plannedFilters: []string{"posting_from", "posting_to", "company_code", "fi_origin"},
		columns:        []string{"posting_yyyymm", "company_code", "fi_origin"},
		sqlText:        "SELECT posting_yyyymm, company_code, fi_origin FROM some_source",
	})

	req := httptest.NewRequest(http.MethodGet, "/api/datasets/fi-origin-monthly/rows?dry_run=true&posting_from=202601&company_code=1000", nil)
	rec := httptest.NewRecorder()
	app.Handler().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	if !strings.Contains(rec.Body.String(), "\"dry_run\": true") {
		t.Fatalf("expected dry_run response, got %s", rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "dataset_rows.posting_yyyymm \\u003e= @p1") {
		t.Fatalf("expected filtered query in body, got %s", rec.Body.String())
	}
}

func newTestApp(t *testing.T) *App {
	t.Helper()

	fsys, contractDir := writeTestFixtures(t)
	return mustNewApp(t, config.Config{
		Addr:         ":0",
		FS:           fsys,
		ContractDir:  contractDir,
		QueryTimeout: 5 * time.Second,
		AuthToken:    "",
	})
}

func newTestAppWithDSN(t *testing.T) *App {
	t.Helper()

	fsys, contractDir := writeTestFixtures(t)
	return mustNewApp(t, config.Config{
		Addr:         ":0",
		FS:           fsys,
		ContractDir:  contractDir,
		QueryTimeout: 5 * time.Second,
		SQLServerDSN: "sqlserver://user:pass@localhost:1433?database=PTD_READONLY",
		AuthToken:    "",
	})
}

type namedFixture struct {
	id             string
	title          string
	domain         string
	plannedFilters []string
	columns        []string
	sqlText        string
}

func newNamedTestApp(t *testing.T, fixture namedFixture) *App {
	t.Helper()

	fsys, contractDir := writeNamedFixtures(t, fixture)
	return mustNewApp(t, config.Config{
		Addr:         ":0",
		FS:           fsys,
		ContractDir:  contractDir,
		QueryTimeout: 5 * time.Second,
		AuthToken:    "",
	})
}

func mustNewApp(t *testing.T, cfg config.Config) *App {
	t.Helper()

	app, err := New(cfg)
	if err != nil {
		t.Fatalf("create app: %v", err)
	}
	t.Cleanup(func() {
		if err := app.Close(); err != nil {
			t.Fatalf("close app: %v", err)
		}
	})

	return app
}

func writeTestFixtures(t *testing.T) (fs.FS, string) {
	t.Helper()

	return writeNamedFixtures(t, namedFixture{
		id:             "test-dataset",
		title:          "Test Dataset",
		domain:         "fi",
		plannedFilters: []string{"company_code"},
		columns:        []string{"posting_yyyymm", "company_code"},
		sqlText:        "SELECT '202603' AS posting_yyyymm, '1000' AS company_code",
	})
}

func writeNamedFixtures(t *testing.T, fixture namedFixture) (fs.FS, string) {
	t.Helper()

	root := t.TempDir()

	sqlDir := filepath.Join(root, "catalog")
	if err := os.MkdirAll(sqlDir, 0o755); err != nil {
		t.Fatalf("create sql dir: %v", err)
	}

	contractDir := filepath.Join(root, "contracts")
	if err := os.MkdirAll(contractDir, 0o755); err != nil {
		t.Fatalf("create contract dir: %v", err)
	}

	if err := os.WriteFile(filepath.Join(sqlDir, "test.sql"), []byte(fixture.sqlText), 0o644); err != nil {
		t.Fatalf("write sql file: %v", err)
	}

	contractText := `{
  "id": "` + fixture.id + `",
  "title": "` + fixture.title + `",
  "domain": "` + fixture.domain + `",
  "sql_file": "catalog/test.sql",
  "read_only": true,
  "default_filters": {
    "mandt": "200"
  },
  "planned_filters": ` + mustJSON(t, fixture.plannedFilters) + `,
  "limit": {
    "default": 100,
    "max": 500
  },
  "cache_ttl_seconds": 60,
  "columns": ` + mustJSON(t, fixture.columns) + `
}`
	if err := os.WriteFile(filepath.Join(contractDir, "test.json"), []byte(contractText), 0o644); err != nil {
		t.Fatalf("write contract file: %v", err)
	}

	return os.DirFS(root), "contracts"
}

func mustValues(t *testing.T, raw string) url.Values {
	t.Helper()

	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatalf("parse query values: %v", err)
	}

	return values
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()

	content, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}

	return string(content)
}

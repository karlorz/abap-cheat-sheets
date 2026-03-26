package server

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	_ "github.com/microsoft/go-mssqldb"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/catalog"
	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/config"
)

type App struct {
	cfg     config.Config
	catalog *catalog.Catalog
	db      *sql.DB
	mux     *http.ServeMux
	cache   *responseCache

	cacheStop chan struct{}
}

var (
	reYYYYMM      = regexp.MustCompile(`^\d{6}$`)
	reCompanyCode = regexp.MustCompile(`^[A-Za-z0-9]{1,4}$`)
	reShortCode   = regexp.MustCompile(`^[A-Za-z0-9]{1,4}$`)
	reToken       = regexp.MustCompile(`^[A-Za-z0-9_]{1,10}$`)
	reCurrency    = regexp.MustCompile(`^[A-Za-z]{3}$`)
)

const blankToken = "[blank]"

func New(cfg config.Config) (*App, error) {
	cat, err := catalog.Load(cfg.FS, cfg.ContractDir)
	if err != nil {
		return nil, fmt.Errorf("load catalog: %w", err)
	}

	var db *sql.DB
	if cfg.SQLServerDSN != "" {
		db, err = sql.Open("sqlserver", cfg.SQLServerDSN)
		if err != nil {
			return nil, fmt.Errorf("open sql server connection: %w", err)
		}

		db.SetMaxOpenConns(5)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(5 * time.Minute)
	}

	app := &App{
		cfg:     cfg,
		catalog: cat,
		db:      db,
		mux:     http.NewServeMux(),
		cache:   newResponseCache(),

		cacheStop: make(chan struct{}),
	}

	app.cache.StartSweep(time.Minute, app.cacheStop)
	app.routes()
	return app, nil
}

func (a *App) Handler() http.Handler {
	return a.mux
}

func (a *App) Close() error {
	if a.cacheStop != nil {
		close(a.cacheStop)
		a.cacheStop = nil
	}
	if a.db != nil {
		return a.db.Close()
	}

	return nil
}

func (a *App) routes() {
	a.mux.HandleFunc("GET /healthz", a.handleHealthz)
	a.mux.HandleFunc("GET /api/datasets", a.requireAuth(a.handleListDatasets))
	a.mux.HandleFunc("GET /api/datasets/{id}", a.requireAuth(a.handleGetDataset))
	a.mux.HandleFunc("GET /api/datasets/{id}/rows", a.requireAuth(a.handleGetDatasetRows))
	if a.cfg.WebFS != nil {
		a.mux.Handle("GET /", a.spaHandler())
	}
}

func (a *App) spaHandler() http.Handler {
	fileServer := http.FileServerFS(a.cfg.WebFS)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath := path.Clean(strings.TrimPrefix(r.URL.Path, "/"))
		if requestPath == "." {
			requestPath = ""
		}

		if requestPath != "" {
			if _, err := fs.Stat(a.cfg.WebFS, requestPath); err == nil {
				fileServer.ServeHTTP(w, r)
				return
			}
		}

		indexContent, err := fs.ReadFile(a.cfg.WebFS, "index.html")
		if err != nil {
			writeError(w, http.StatusInternalServerError, "static_asset_missing", "embedded web index.html is not available")
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexContent)
	})
}

func (a *App) handleHealthz(w http.ResponseWriter, r *http.Request) {
	response := map[string]any{
		"status":                "ok",
		"dataset_count":         a.catalog.Count(),
		"sql_server_configured": a.db != nil,
	}

	if a.db != nil {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := a.db.PingContext(ctx); err != nil {
			response["status"] = "degraded"
			response["sql_server_error"] = err.Error()
			writeJSON(w, http.StatusServiceUnavailable, response)
			return
		}
	}

	writeJSON(w, http.StatusOK, response)
}

func (a *App) handleListDatasets(w http.ResponseWriter, _ *http.Request) {
	type summary struct {
		ID                string   `json:"id"`
		Title             string   `json:"title"`
		Domain            string   `json:"domain"`
		Columns           []string `json:"columns"`
		DefaultLimit      int      `json:"default_limit"`
		MaxLimit          int      `json:"max_limit"`
		PlannedFilters    []string `json:"planned_filters"`
		ExecutableFilters []string `json:"executable_filters"`
		FilterSupport     string   `json:"filter_support"`
		ReadOnly          bool     `json:"read_only"`
	}

	contracts := a.catalog.All()
	items := make([]summary, 0, len(contracts))
	for _, contract := range contracts {
		items = append(items, summary{
			ID:                contract.ID,
			Title:             contract.Title,
			Domain:            contract.Domain,
			Columns:           contract.Columns,
			DefaultLimit:      contract.Limit.Default,
			MaxLimit:          contract.Limit.Max,
			PlannedFilters:    contract.PlannedFilters,
			ExecutableFilters: executableFilters(contract.ID),
			FilterSupport:     filterSupportMode(contract.ID),
			ReadOnly:          contract.ReadOnly,
		})
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"items": items,
		"count": len(items),
	})
}

func (a *App) handleGetDataset(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	contract, ok := a.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "dataset_not_found", fmt.Sprintf("dataset %q was not found", id))
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"dataset":                contract,
		"supported_query_params": supportedQueryParams(contract.ID),
		"executable_filters":     executableFilters(contract.ID),
		"filter_support":         filterSupportMode(contract.ID),
		"dry_run_supported":      true,
	})
}

func (a *App) handleGetDatasetRows(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	contract, ok := a.catalog.Get(id)
	if !ok {
		writeError(w, http.StatusNotFound, "dataset_not_found", fmt.Sprintf("dataset %q was not found", id))
		return
	}

	if unsupported := unsupportedParams(r.URL.Query(), supportedQueryParams(contract.ID)...); len(unsupported) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{
			"error":                    "unsupported_query_params",
			"message":                  "unsupported query parameters for this dataset",
			"unsupported_query_params": unsupported,
			"supported_query_params":   supportedQueryParams(contract.ID),
			"executable_filters":       executableFilters(contract.ID),
			"planned_filters":          contract.PlannedFilters,
		})
		return
	}

	limit, err := parseLimit(r.URL.Query().Get("limit"), contract.Limit.Default, contract.Limit.Max)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_limit", err.Error())
		return
	}

	dryRun, err := parseDryRun(r.URL.Query().Get("dry_run"))
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_dry_run", err.Error())
		return
	}

	sqlText, _, err := a.catalog.LoadSQL(id)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "load_sql_failed", err.Error())
		return
	}

	query, args, err := buildDatasetQuery(contract.ID, sqlText, limit, r.URL.Query())
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid_filters", err.Error())
		return
	}

	if dryRun {
		writeJSON(w, http.StatusOK, map[string]any{
			"dataset_id":         contract.ID,
			"title":              contract.Title,
			"query_limit":        limit,
			"dry_run":            true,
			"query":              query,
			"args":               args,
			"filter_support":     filterSupportMode(contract.ID),
			"executable_filters": executableFilters(contract.ID),
			"planned_filters":    contract.PlannedFilters,
		})
		return
	}

	if a.db == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{
			"error":              "sql_server_not_configured",
			"message":            "set PTD_SQLSERVER_DSN to enable dataset execution",
			"filter_support":     filterSupportMode(contract.ID),
			"executable_filters": executableFilters(contract.ID),
			"planned_filters":    contract.PlannedFilters,
		})
		return
	}

	cacheKeyValue := cacheKey(contract.ID, r.URL.Query())
	if !shouldBypassCache(r) {
		if cachedBody, ok := a.cache.Get(cacheKeyValue); ok {
			w.Header().Set("X-Cache", "HIT")
			writeJSONBytes(w, http.StatusOK, cachedBody)
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.QueryTimeout)
	defer cancel()

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		writeError(w, http.StatusBadGateway, "query_failed", err.Error())
		return
	}
	defer rows.Close()

	columns, items, err := scanRows(rows)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "scan_failed", err.Error())
		return
	}

	payload := map[string]any{
		"dataset_id":         contract.ID,
		"title":              contract.Title,
		"query_limit":        limit,
		"row_count":          len(items),
		"columns":            columns,
		"rows":               items,
		"filter_support":     filterSupportMode(contract.ID),
		"executable_filters": executableFilters(contract.ID),
		"planned_filters":    contract.PlannedFilters,
		"cache_ttl_seconds":  contract.CacheTTL,
		"generated_at":       time.Now().UTC(),
	}

	body, err := marshalJSON(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode_failed", err.Error())
		return
	}

	a.cache.Set(cacheKeyValue, body, time.Duration(contract.CacheTTL)*time.Second)
	w.Header().Set("X-Cache", "MISS")
	writeJSONBytes(w, http.StatusOK, body)
}

func parseLimit(raw string, defaultLimit, maxLimit int) (int, error) {
	if raw == "" {
		return defaultLimit, nil
	}

	limit, err := strconv.Atoi(raw)
	if err != nil {
		return 0, errors.New("limit must be an integer")
	}
	if limit <= 0 {
		return 0, errors.New("limit must be > 0")
	}
	if limit > maxLimit {
		return 0, fmt.Errorf("limit must be <= %d", maxLimit)
	}

	return limit, nil
}

func unsupportedParams(values url.Values, allowed ...string) []string {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, key := range allowed {
		allowedSet[key] = struct{}{}
	}

	var unsupported []string
	for key := range values {
		if _, ok := allowedSet[key]; !ok {
			unsupported = append(unsupported, key)
		}
	}

	slices.Sort(unsupported)
	return unsupported
}

func parseDryRun(raw string) (bool, error) {
	if raw == "" {
		return false, nil
	}

	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, errors.New("dry_run must be a boolean")
	}
}

func supportedQueryParams(datasetID string) []string {
	keys := []string{"limit", "dry_run", "nocache"}
	keys = append(keys, executableFilters(datasetID)...)
	return keys
}

func shouldBypassCache(r *http.Request) bool {
	cacheControl := strings.ToLower(r.Header.Get("Cache-Control"))
	if strings.Contains(cacheControl, "no-cache") {
		return true
	}

	raw := strings.TrimSpace(r.URL.Query().Get("nocache"))
	switch strings.ToLower(raw) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func executableFilters(datasetID string) []string {
	switch datasetID {
	case "fi-origin-monthly":
		return []string{"posting_from", "posting_to", "company_code", "fi_origin"}
	case "ar-open-monthly":
		return []string{"posting_from", "posting_to", "company_code", "currency_code", "document_type"}
	case "ar-cleared-monthly":
		return []string{"clearing_from", "clearing_to", "company_code", "original_document_type", "clearing_document_type"}
	case "purchasing-history-monthly":
		return []string{"posting_from", "posting_to", "history_category", "plant_code"}
	case "inventory-movement-monthly":
		return []string{"posting_from", "posting_to", "plant_code", "movement_type"}
	case "current-stock":
		return []string{"plant_code", "storage_location"}
	default:
		return nil
	}
}

func filterSupportMode(datasetID string) string {
	if len(executableFilters(datasetID)) == 0 {
		return "limit_only"
	}

	return "selected_filters"
}

func buildDatasetQuery(datasetID, sqlText string, limit int, values url.Values) (string, []any, error) {
	trimmed := strings.TrimSpace(sqlText)
	trimmed = strings.TrimSuffix(trimmed, ";")

	whereClauses := make([]string, 0, 4)
	args := make([]any, 0, 4)
	appendArg := func(value any) string {
		args = append(args, value)
		return fmt.Sprintf("@p%d", len(args))
	}

	switch datasetID {
	case "fi-origin-monthly":
		var err error
		whereClauses, err = appendRangeFilter(whereClauses, "dataset_rows.posting_yyyymm", "posting_from", values.Get("posting_from"), "posting_to", values.Get("posting_to"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.company_code", values.Get("company_code"), normalizeCompanyCode, appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.fi_origin", values.Get("fi_origin"), normalizeTokenOrBlank, appendArg)
		if err != nil {
			return "", nil, err
		}
	case "ar-open-monthly":
		var err error
		whereClauses, err = appendRangeFilter(whereClauses, "dataset_rows.posting_yyyymm", "posting_from", values.Get("posting_from"), "posting_to", values.Get("posting_to"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.company_code", values.Get("company_code"), normalizeCompanyCode, appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.currency_code", values.Get("currency_code"), normalizeCurrencyCode, appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.document_type", values.Get("document_type"), normalizeToken, appendArg)
		if err != nil {
			return "", nil, err
		}
	case "ar-cleared-monthly":
		var err error
		whereClauses, err = appendRangeFilter(whereClauses, "dataset_rows.clearing_yyyymm", "clearing_from", values.Get("clearing_from"), "clearing_to", values.Get("clearing_to"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.company_code", values.Get("company_code"), normalizeCompanyCode, appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.original_document_type", values.Get("original_document_type"), normalizeToken, appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.clearing_document_type", values.Get("clearing_document_type"), normalizeToken, appendArg)
		if err != nil {
			return "", nil, err
		}
	case "purchasing-history-monthly":
		var err error
		whereClauses, err = appendRangeFilter(whereClauses, "dataset_rows.posting_yyyymm", "posting_from", values.Get("posting_from"), "posting_to", values.Get("posting_to"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.history_category", values.Get("history_category"), normalizeToken, appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.plant_code", values.Get("plant_code"), normalizeShortCodeOrBlank("plant_code"), appendArg)
		if err != nil {
			return "", nil, err
		}
	case "inventory-movement-monthly":
		var err error
		whereClauses, err = appendRangeFilter(whereClauses, "dataset_rows.posting_yyyymm", "posting_from", values.Get("posting_from"), "posting_to", values.Get("posting_to"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.plant_code", values.Get("plant_code"), normalizeShortCode("plant_code"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.movement_type", values.Get("movement_type"), normalizeToken, appendArg)
		if err != nil {
			return "", nil, err
		}
	case "current-stock":
		var err error
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.plant_code", values.Get("plant_code"), normalizeShortCode("plant_code"), appendArg)
		if err != nil {
			return "", nil, err
		}
		whereClauses, err = appendExactFilter(whereClauses, "dataset_rows.storage_location", values.Get("storage_location"), normalizeShortCode("storage_location"), appendArg)
		if err != nil {
			return "", nil, err
		}
	}

	query := fmt.Sprintf("SELECT TOP (%d) * FROM (\n%s\n) AS dataset_rows", limit, trimmed)
	if len(whereClauses) > 0 {
		query += "\nWHERE " + strings.Join(whereClauses, " AND ")
	}

	if order := defaultOrderBy(datasetID); order != "" {
		query += "\nORDER BY " + order
	}

	return query, args, nil
}

func appendRangeFilter(whereClauses []string, column, fromName, fromRaw, toName, toRaw string, appendArg func(any) string) ([]string, error) {
	if fromRaw != "" {
		value, err := normalizeYYYYMM(fromRaw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fromName, err)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("%s >= %s", column, appendArg(value)))
	}

	if toRaw != "" {
		value, err := normalizeYYYYMM(toRaw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", toName, err)
		}
		whereClauses = append(whereClauses, fmt.Sprintf("%s <= %s", column, appendArg(value)))
	}

	return whereClauses, nil
}

func appendExactFilter(whereClauses []string, column, raw string, normalize func(string) (string, error), appendArg func(any) string) ([]string, error) {
	if raw == "" {
		return whereClauses, nil
	}

	value, err := normalize(raw)
	if err != nil {
		return nil, err
	}

	return append(whereClauses, fmt.Sprintf("%s = %s", column, appendArg(value))), nil
}

func defaultOrderBy(datasetID string) string {
	switch datasetID {
	case "fi-origin-monthly":
		return "dataset_rows.posting_yyyymm DESC, dataset_rows.company_code ASC, dataset_rows.fi_origin ASC"
	case "ar-open-monthly":
		return "dataset_rows.posting_yyyymm DESC, dataset_rows.company_code ASC, dataset_rows.currency_code ASC, dataset_rows.document_type ASC"
	case "ar-cleared-monthly":
		return "dataset_rows.clearing_yyyymm DESC, dataset_rows.company_code ASC, dataset_rows.original_document_type ASC, dataset_rows.clearing_document_type ASC"
	case "purchasing-history-monthly":
		return "dataset_rows.posting_yyyymm DESC, dataset_rows.history_category ASC, dataset_rows.plant_code ASC"
	case "inventory-movement-monthly":
		return "dataset_rows.posting_yyyymm DESC, dataset_rows.plant_code ASC, dataset_rows.movement_type ASC"
	case "current-stock":
		return "dataset_rows.plant_code ASC, dataset_rows.storage_location ASC"
	default:
		return ""
	}
}

func normalizeYYYYMM(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !reYYYYMM.MatchString(value) {
		return "", errors.New("must be a 6 digit YYYYMM value")
	}

	return value, nil
}

func normalizeCompanyCode(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !reCompanyCode.MatchString(value) {
		return "", errors.New("company_code must be 1 to 4 alphanumeric characters")
	}

	return strings.ToUpper(value), nil
}

func normalizeToken(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !reToken.MatchString(value) {
		return "", errors.New("value must be 1 to 10 alphanumeric or underscore characters")
	}

	return strings.ToUpper(value), nil
}

func normalizeTokenOrBlank(raw string) (string, error) {
	return normalizeBlankable(raw, normalizeToken)
}

func normalizeCurrencyCode(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if !reCurrency.MatchString(value) {
		return "", errors.New("currency_code must be a 3 letter code")
	}

	return strings.ToUpper(value), nil
}

func normalizeShortCode(fieldName string) func(string) (string, error) {
	return func(raw string) (string, error) {
		value := strings.TrimSpace(raw)
		if !reShortCode.MatchString(value) {
			return "", fmt.Errorf("%s must be 1 to 4 alphanumeric characters", fieldName)
		}

		return strings.ToUpper(value), nil
	}
}

func normalizeShortCodeOrBlank(fieldName string) func(string) (string, error) {
	base := normalizeShortCode(fieldName)
	return func(raw string) (string, error) {
		return normalizeBlankable(raw, base)
	}
}

func normalizeBlankable(raw string, normalize func(string) (string, error)) (string, error) {
	value := strings.TrimSpace(raw)
	if strings.EqualFold(value, blankToken) {
		return blankToken, nil
	}

	return normalize(value)
}

func scanRows(rows *sql.Rows) ([]string, []map[string]any, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, nil, fmt.Errorf("load columns: %w", err)
	}

	var result []map[string]any
	for rows.Next() {
		values := make([]any, len(columns))
		targets := make([]any, len(columns))
		for i := range values {
			targets[i] = &values[i]
		}

		if err := rows.Scan(targets...); err != nil {
			return nil, nil, fmt.Errorf("scan row: %w", err)
		}

		item := make(map[string]any, len(columns))
		for i, name := range columns {
			item[name] = normalizeValue(values[i])
		}

		result = append(result, item)
	}

	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("iterate rows: %w", err)
	}

	return columns, result, nil
}

func normalizeValue(value any) any {
	switch typed := value.(type) {
	case nil:
		return nil
	case []byte:
		return string(typed)
	case time.Time:
		return typed.UTC().Format(time.RFC3339)
	default:
		return typed
	}
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	writeJSON(w, status, map[string]any{
		"error":   code,
		"message": message,
	})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	body, err := marshalJSON(payload)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "encode_failed", err.Error())
		return
	}

	writeJSONBytes(w, status, body)
}

func marshalJSON(payload any) ([]byte, error) {
	body, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}

	return append(body, '\n'), nil
}

func writeJSONBytes(w http.ResponseWriter, status int, body []byte) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

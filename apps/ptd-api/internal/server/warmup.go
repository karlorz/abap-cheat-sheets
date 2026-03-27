package server

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/karlchow/abap-cheat-sheets/apps/ptd-api/internal/catalog"
)

var (
	errLoadDatasetSQL = errors.New("load dataset sql")
	errDatasetQuery   = errors.New("dataset query failed")
	errDatasetRowScan = errors.New("dataset row scan failed")
)

func (a *App) buildDatasetRowsQuery(contract catalog.Contract, limit int, values url.Values) (string, []any, error) {
	sqlText, _, err := a.catalog.LoadSQL(contract.ID)
	if err != nil {
		return "", nil, fmt.Errorf("%w: %v", errLoadDatasetSQL, err)
	}

	return buildDatasetQuery(contract.ID, sqlText, limit, values)
}

func (a *App) queryDatasetRows(ctx context.Context, query string, args []any) ([]string, []map[string]any, error) {
	if a.db == nil {
		return nil, nil, fmt.Errorf("%w: sql server not configured", errDatasetQuery)
	}

	rows, err := a.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errDatasetQuery, err)
	}
	defer rows.Close()

	columns, items, err := scanRows(rows)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", errDatasetRowScan, err)
	}

	return columns, items, nil
}

func (a *App) fetchDatasetRows(ctx context.Context, contract catalog.Contract, limit int, values url.Values) ([]string, []map[string]any, error) {
	query, args, err := a.buildDatasetRowsQuery(contract, limit, values)
	if err != nil {
		return nil, nil, err
	}

	queryCtx, cancel := a.withQueryTimeout(ctx)
	defer cancel()

	return a.queryDatasetRows(queryCtx, query, args)
}

func writeDatasetRowsBuildError(w http.ResponseWriter, err error) {
	if errors.Is(err, errLoadDatasetSQL) {
		writeError(w, http.StatusInternalServerError, "load_sql_failed", err.Error())
		return
	}

	writeError(w, http.StatusBadRequest, "invalid_filters", err.Error())
}

func writeDatasetRowsExecutionError(w http.ResponseWriter, err error) {
	if errors.Is(err, errDatasetRowScan) {
		writeError(w, http.StatusInternalServerError, "scan_failed", err.Error())
		return
	}

	writeError(w, http.StatusBadGateway, "query_failed", err.Error())
}

func (a *App) withQueryTimeout(parent context.Context) (context.Context, context.CancelFunc) {
	if parent == nil {
		parent = context.Background()
	}

	return context.WithTimeout(parent, a.cfg.QueryTimeout)
}

func buildDatasetRowsPayload(contract catalog.Contract, limit int, columns []string, items []map[string]any, generatedAt time.Time) map[string]any {
	return map[string]any{
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
		"generated_at":       generatedAt,
	}
}

func (a *App) Warmup() {
	if a.db == nil {
		return
	}

	contracts := a.catalog.All()
	warmed := 0
	for _, contract := range contracts {
		startedAt := time.Now()

		columns, items, err := a.fetchDatasetRows(context.Background(), contract, contract.Limit.Default, url.Values{})
		if err != nil {
			log.Printf("cache warmup: %s: %v", contract.ID, err)
			continue
		}

		body, err := marshalJSON(buildDatasetRowsPayload(contract, contract.Limit.Default, columns, items, time.Now().UTC()))
		if err != nil {
			log.Printf("cache warmup: %s: encode payload: %v", contract.ID, err)
			continue
		}

		a.cache.Set(cacheKey(contract.ID, url.Values{}), body, time.Duration(contract.CacheTTL)*time.Second)
		warmed++

		log.Printf(
			"cache warmup: %s: %d rows cached in %s",
			contract.ID,
			len(items),
			formatDuration(time.Since(startedAt)),
		)
	}

	log.Printf("cache warmup: complete (%d/%d datasets cached)", warmed, len(contracts))
}

func formatDuration(value time.Duration) string {
	if value < 0 {
		value = 0
	}

	return fmt.Sprintf("%.2fs", value.Seconds())
}

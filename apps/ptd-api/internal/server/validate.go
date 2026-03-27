package server

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
)

var validationTables = []string{
	"BKPF",
	"BSID",
	"BSAD",
	"EKBE",
	"MKPF",
	"MSEG",
	"MARD",
	"T001",
	"T001W",
}

const (
	validationDatasetLimit   = 10
	validationAnalyticsMANDT = "200"
	tableBKPF                = "BKPF"
	tableBSID                = "BSID"
)

type ValidationReport struct {
	SchemaCheck   SchemaCheckResult
	MANDTCheck    MANDTCheckResult
	RowCounts     []RowCountResult
	Currencies    CurrencyCheckResult
	DatasetChecks []DatasetCheckResult
}

type TableInfo struct {
	SchemaName string
	TableName  string
}

type SchemaCheckResult struct {
	Tables  []TableInfo
	Missing []string
	Error   string
	OK      bool
}

type MANDTCheckResult struct {
	Values         []string
	ExpectedClient string
	ExtraClients   []string
	Warning        string
	Error          string
	OK             bool
}

type RowCountResult struct {
	Table string
	Count int64
	Error string
}

type CurrencyCheckResult struct {
	Codes   []string
	HasJPY  bool
	HasKWD  bool
	HasBHD  bool
	Warning string
	Error   string
}

type DatasetCheckResult struct {
	DatasetID string
	RowCount  int
	Duration  time.Duration
	Error     string
}

func (a *App) Validate(ctx context.Context) (*ValidationReport, error) {
	if a.db == nil {
		return nil, errors.New("validation requires PTD_SQLSERVER_DSN")
	}

	report := &ValidationReport{}

	tables, err := a.loadSchemaTables(ctx)
	if err != nil {
		report.SchemaCheck.Error = err.Error()
	} else {
		report.SchemaCheck.Tables = tables
		report.SchemaCheck.Missing = missingValidationTables(tables)
		report.SchemaCheck.OK = len(report.SchemaCheck.Missing) == 0
	}

	schemaByTable := discoveredSchemaByTable(report.SchemaCheck.Tables)

	report.MANDTCheck = a.validateMANDT(ctx, schemaByTable)
	report.RowCounts = a.validateRowCounts(ctx, schemaByTable)
	report.Currencies = a.validateCurrencies(ctx, schemaByTable)
	report.DatasetChecks = a.validateDatasets(ctx)

	return report, nil
}

func (r *ValidationReport) HasFailures() bool {
	if r == nil {
		return true
	}

	if !r.SchemaCheck.OK || r.SchemaCheck.Error != "" {
		return true
	}
	if !r.MANDTCheck.OK || r.MANDTCheck.Error != "" {
		return true
	}
	if r.Currencies.Error != "" {
		return true
	}

	return slices.ContainsFunc(r.RowCounts, func(result RowCountResult) bool {
		return result.Error != ""
	}) || slices.ContainsFunc(r.DatasetChecks, func(result DatasetCheckResult) bool {
		return result.Error != ""
	})
}

func FormatValidationReport(report *ValidationReport) string {
	if report == nil {
		return ""
	}

	var builder strings.Builder
	builder.WriteString("PTD Validation Report\n")
	builder.WriteString("=====================\n\n")

	builder.WriteString("Schema check:\n")
	if report.SchemaCheck.Error != "" {
		builder.WriteString(fmt.Sprintf("  [fail] %s\n", report.SchemaCheck.Error))
	} else {
		for _, table := range report.SchemaCheck.Tables {
			builder.WriteString(fmt.Sprintf("  %s.%s\n", table.SchemaName, table.TableName))
		}
		if len(report.SchemaCheck.Missing) > 0 {
			builder.WriteString(fmt.Sprintf("  Missing: %s\n", strings.Join(report.SchemaCheck.Missing, ", ")))
		}
		builder.WriteString(fmt.Sprintf("  Status: %s\n", formatStatus(report.SchemaCheck.OK)))
	}

	builder.WriteString("\nMANDT check:\n")
	if report.MANDTCheck.Error != "" {
		builder.WriteString(fmt.Sprintf("  [fail] %s\n", report.MANDTCheck.Error))
	} else {
		builder.WriteString(fmt.Sprintf("  Distinct values: %s\n", formatStringList(report.MANDTCheck.Values)))
		if report.MANDTCheck.ExpectedClient != "" {
			builder.WriteString(fmt.Sprintf("  Analytics scope: %s\n", report.MANDTCheck.ExpectedClient))
		}
		if len(report.MANDTCheck.ExtraClients) > 0 {
			builder.WriteString(fmt.Sprintf("  Extra clients outside scope: %s\n", formatStringList(report.MANDTCheck.ExtraClients)))
		}
		if report.MANDTCheck.Warning != "" {
			builder.WriteString(fmt.Sprintf("  Warning: %s\n", report.MANDTCheck.Warning))
		}
		builder.WriteString(fmt.Sprintf("  Status: %s\n", formatStatus(report.MANDTCheck.OK)))
	}

	builder.WriteString("\nRow counts:\n")
	for _, result := range report.RowCounts {
		if result.Error != "" {
			builder.WriteString(fmt.Sprintf("  %-8s [fail] %s\n", result.Table, result.Error))
			continue
		}
		builder.WriteString(fmt.Sprintf("  %-8s %s\n", result.Table, formatInt64(result.Count)))
	}

	builder.WriteString("\nCurrency exposure:\n")
	if report.Currencies.Error != "" {
		builder.WriteString(fmt.Sprintf("  [fail] %s\n", report.Currencies.Error))
	} else {
		builder.WriteString(fmt.Sprintf("  Codes: %s\n", formatStringList(report.Currencies.Codes)))
		if report.Currencies.Warning != "" {
			builder.WriteString(fmt.Sprintf("  Warning: %s\n", report.Currencies.Warning))
		} else {
			builder.WriteString("  Warning: none\n")
		}
	}

	builder.WriteString("\nDataset timing (TOP 10, no filters):\n")
	successCount := 0
	for _, result := range report.DatasetChecks {
		status := formatStatus(result.Error == "")
		if result.Error == "" {
			successCount++
			builder.WriteString(fmt.Sprintf(
				"  %-28s %6d rows  %7s  %s\n",
				result.DatasetID,
				result.RowCount,
				formatDuration(result.Duration),
				status,
			))
			continue
		}

		builder.WriteString(fmt.Sprintf(
			"  %-28s %6s      %7s  [fail] %s\n",
			result.DatasetID,
			"-",
			"-",
			result.Error,
		))
	}

	resultLabel := "PASS"
	if report.HasFailures() {
		resultLabel = "FAIL"
	}
	builder.WriteString(fmt.Sprintf("\nResult: %s (%d/%d datasets OK)\n", resultLabel, successCount, len(report.DatasetChecks)))

	return builder.String()
}

func (a *App) validateMANDT(ctx context.Context, schemaByTable map[string]string) MANDTCheckResult {
	schema, err := discoveredSchema(schemaByTable, tableBKPF)
	if err != nil {
		return MANDTCheckResult{
			Error: err.Error(),
		}
	}

	values, err := a.queryDistinctStrings(
		ctx,
		fmt.Sprintf("SELECT DISTINCT MANDT FROM %s ORDER BY MANDT", qualifyTable(schema, tableBKPF)),
	)
	if err != nil {
		return MANDTCheckResult{
			Error: err.Error(),
		}
	}

	return MANDTCheckResult{
		Values:         values,
		ExpectedClient: validationAnalyticsMANDT,
		ExtraClients:   extraValidationClients(values, validationAnalyticsMANDT),
		Warning:        mandtScopeWarning(values, validationAnalyticsMANDT),
		OK:             slices.Contains(values, validationAnalyticsMANDT),
	}
}

func (a *App) validateRowCounts(ctx context.Context, schemaByTable map[string]string) []RowCountResult {
	results := make([]RowCountResult, 0, len(validationTables))
	for _, table := range validationTables {
		result := RowCountResult{Table: table}
		schema, err := discoveredSchema(schemaByTable, table)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		count, err := a.queryCount(
			ctx,
			fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE MANDT = %s", qualifyTable(schema, table), quoteSQLString(validationAnalyticsMANDT)),
		)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		result.Count = count
		results = append(results, result)
	}

	return results
}

func (a *App) validateCurrencies(ctx context.Context, schemaByTable map[string]string) CurrencyCheckResult {
	schema, err := discoveredSchema(schemaByTable, tableBSID)
	if err != nil {
		return CurrencyCheckResult{
			Error: err.Error(),
		}
	}

	codes, err := a.queryDistinctStrings(
		ctx,
		fmt.Sprintf("SELECT DISTINCT WAERS FROM %s WHERE MANDT = %s ORDER BY WAERS", qualifyTable(schema, tableBSID), quoteSQLString(validationAnalyticsMANDT)),
	)
	if err != nil {
		return CurrencyCheckResult{
			Error: err.Error(),
		}
	}

	result := CurrencyCheckResult{
		Codes:  codes,
		HasJPY: slices.Contains(codes, "JPY"),
		HasKWD: slices.Contains(codes, "KWD"),
		HasBHD: slices.Contains(codes, "BHD"),
	}
	if result.HasJPY || result.HasKWD || result.HasBHD {
		var risky []string
		if result.HasJPY {
			risky = append(risky, "JPY")
		}
		if result.HasKWD {
			risky = append(risky, "KWD")
		}
		if result.HasBHD {
			risky = append(risky, "BHD")
		}
		result.Warning = fmt.Sprintf("risky currency precision detected for pilot: %s", strings.Join(risky, ", "))
	}

	return result
}

func (a *App) validateDatasets(ctx context.Context) []DatasetCheckResult {
	contracts := a.catalog.All()
	results := make([]DatasetCheckResult, 0, len(contracts))
	for _, contract := range contracts {
		startedAt := time.Now()

		result := DatasetCheckResult{
			DatasetID: contract.ID,
		}

		_, items, err := a.fetchDatasetRows(ctx, contract, validationDatasetLimit, url.Values{})
		result.Duration = time.Since(startedAt)
		if err != nil {
			result.Error = err.Error()
			results = append(results, result)
			continue
		}

		result.RowCount = len(items)
		results = append(results, result)
	}

	return results
}

func (a *App) loadSchemaTables(ctx context.Context) ([]TableInfo, error) {
	queryCtx, cancel := a.withQueryTimeout(ctx)
	defer cancel()

	rows, err := a.db.QueryContext(
		queryCtx,
		fmt.Sprintf(`SELECT SCHEMA_NAME(schema_id) AS schema_name, name
FROM sys.tables
WHERE name IN (%s)
ORDER BY name, SCHEMA_NAME(schema_id)`, validationTableNamesSQL()),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tables []TableInfo
	for rows.Next() {
		var info TableInfo
		if err := rows.Scan(&info.SchemaName, &info.TableName); err != nil {
			return nil, err
		}
		tables = append(tables, info)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return tables, nil
}

func (a *App) queryDistinctStrings(ctx context.Context, query string) ([]string, error) {
	queryCtx, cancel := a.withQueryTimeout(ctx)
	defer cancel()

	rows, err := a.db.QueryContext(queryCtx, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var values []string
	for rows.Next() {
		var value sql.NullString
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if !value.Valid {
			continue
		}
		values = append(values, strings.TrimSpace(value.String))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return values, nil
}

func (a *App) queryCount(ctx context.Context, query string) (int64, error) {
	queryCtx, cancel := a.withQueryTimeout(ctx)
	defer cancel()

	var count int64
	if err := a.db.QueryRowContext(queryCtx, query).Scan(&count); err != nil {
		return 0, err
	}

	return count, nil
}

func missingValidationTables(found []TableInfo) []string {
	seen := make(map[string]struct{}, len(found))
	for _, table := range found {
		seen[strings.ToUpper(table.TableName)] = struct{}{}
	}

	missing := make([]string, 0, len(validationTables))
	for _, table := range validationTables {
		if _, ok := seen[table]; !ok {
			missing = append(missing, table)
		}
	}

	return missing
}

func validationTableNamesSQL() string {
	names := make([]string, 0, len(validationTables))
	for _, table := range validationTables {
		names = append(names, quoteSQLString(table))
	}

	return strings.Join(names, ",")
}

func discoveredSchemaByTable(found []TableInfo) map[string]string {
	schemaByTable := make(map[string]string, len(found))
	for _, table := range found {
		name := strings.ToUpper(table.TableName)
		schema := table.SchemaName
		if current, ok := schemaByTable[name]; !ok || strings.EqualFold(schema, "ptd") || current == "" {
			schemaByTable[name] = schema
		}
	}

	return schemaByTable
}

func discoveredSchema(schemaByTable map[string]string, tableName string) (string, error) {
	schema, ok := schemaByTable[tableName]
	if !ok {
		return "", fmt.Errorf("%s not found during schema discovery", tableName)
	}

	return schema, nil
}

func qualifyTable(schemaName, tableName string) string {
	return quoteIdentifier(schemaName) + "." + quoteIdentifier(tableName)
}

func quoteIdentifier(name string) string {
	return "[" + strings.ReplaceAll(name, "]", "]]") + "]"
}

func quoteSQLString(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func formatStatus(ok bool) string {
	if ok {
		return "[ok]"
	}

	return "[fail]"
}

func extraValidationClients(values []string, expected string) []string {
	extras := make([]string, 0, len(values))
	for _, value := range values {
		if value == expected {
			continue
		}
		extras = append(extras, value)
	}

	return extras
}

func mandtScopeWarning(values []string, expected string) string {
	extras := extraValidationClients(values, expected)
	if len(extras) == 0 {
		return ""
	}

	return fmt.Sprintf("analytics scope is limited to client %s; extra clients observed: %s", expected, strings.Join(extras, ", "))
}

func formatStringList(values []string) string {
	if len(values) == 0 {
		return "[]"
	}

	return "[" + strings.Join(values, ", ") + "]"
}

func formatInt64(value int64) string {
	raw := fmt.Sprintf("%d", value)
	if len(raw) <= 3 {
		return raw
	}

	var builder strings.Builder
	prefixLen := len(raw) % 3
	if prefixLen == 0 {
		prefixLen = 3
	}
	builder.WriteString(raw[:prefixLen])
	for i := prefixLen; i < len(raw); i += 3 {
		builder.WriteByte(',')
		builder.WriteString(raw[i : i+3])
	}

	return builder.String()
}

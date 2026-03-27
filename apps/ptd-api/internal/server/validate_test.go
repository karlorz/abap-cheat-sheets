package server

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestValidateRequiresDB(t *testing.T) {
	app := newTestApp(t)

	report, err := app.Validate(context.Background())
	if err == nil {
		t.Fatal("expected validation error without db")
	}
	if report != nil {
		t.Fatalf("expected nil report on db validation error, got %+v", report)
	}
}

func TestValidationReportFormat(t *testing.T) {
	report := &ValidationReport{
		SchemaCheck: SchemaCheckResult{
			Tables: []TableInfo{
				{SchemaName: "ptd", TableName: "BKPF"},
				{SchemaName: "ptd", TableName: "BSID"},
			},
			OK: true,
		},
		MANDTCheck: MANDTCheckResult{
			Values: []string{"200"},
			OK:     true,
		},
		RowCounts: []RowCountResult{
			{Table: "BKPF", Count: 1234567},
			{Table: "BSID", Count: 456789},
		},
		Currencies: CurrencyCheckResult{
			Codes: []string{"EUR", "USD"},
		},
		DatasetChecks: []DatasetCheckResult{
			{DatasetID: "fi-origin-monthly", RowCount: 12, Duration: 340 * time.Millisecond},
		},
	}

	formatted := FormatValidationReport(report)

	expectedFragments := []string{
		"PTD Validation Report",
		"Schema check:",
		"MANDT check:",
		"Row counts:",
		"Currency exposure:",
		"Dataset timing (TOP 10, no filters):",
		"Result: PASS (1/1 datasets OK)",
	}
	for _, fragment := range expectedFragments {
		if !strings.Contains(formatted, fragment) {
			t.Fatalf("expected formatted report to contain %q, got %s", fragment, formatted)
		}
	}
}

func TestValidationTableNamesSQL(t *testing.T) {
	got := validationTableNamesSQL()
	want := "'BKPF','BSID','BSAD','EKBE','MKPF','MSEG','MARD','T001','T001W'"
	if got != want {
		t.Fatalf("unexpected validation table sql list: got %q want %q", got, want)
	}
}

func TestValidateBuildsReport(t *testing.T) {
	app := newValidationTestApp(t)

	report, err := app.Validate(context.Background())
	if err != nil {
		t.Fatalf("validate: %v", err)
	}
	if report == nil {
		t.Fatal("expected validation report")
	}
	if !report.SchemaCheck.OK {
		t.Fatalf("expected schema check ok, got %+v", report.SchemaCheck)
	}
	if !report.MANDTCheck.OK {
		t.Fatalf("expected MANDT check ok, got %+v", report.MANDTCheck)
	}
	if len(report.RowCounts) != len(validationTables) {
		t.Fatalf("expected %d row counts, got %d", len(validationTables), len(report.RowCounts))
	}
	if len(report.DatasetChecks) != 1 {
		t.Fatalf("expected 1 dataset check, got %d", len(report.DatasetChecks))
	}
	if report.HasFailures() {
		t.Fatalf("expected validation report without failures, got %+v", report)
	}
}

func newValidationTestApp(t *testing.T) *App {
	t.Helper()

	registerValidationTestDriver()

	return newNamedTestAppWithDB(t, fiOriginMonthlyFixture("SELECT '202603' AS posting_yyyymm, '1000' AS company_code, 'MKPF' AS fi_origin"), validationTestDriverName)
}

const validationTestDriverName = "ptd-api-validate-test"

var registerValidationDriverOnce sync.Once

func registerValidationTestDriver() {
	registerValidationDriverOnce.Do(func() {
		sql.Register(validationTestDriverName, validationTestDriver{})
	})
}

type validationTestDriver struct{}

func (validationTestDriver) Open(string) (driver.Conn, error) {
	return validationTestConn{}, nil
}

type validationTestConn struct{}

func (validationTestConn) Prepare(string) (driver.Stmt, error) {
	return nil, driver.ErrSkip
}

func (validationTestConn) Close() error {
	return nil
}

func (validationTestConn) Begin() (driver.Tx, error) {
	return nil, driver.ErrSkip
}

func (validationTestConn) QueryContext(_ context.Context, query string, _ []driver.NamedValue) (driver.Rows, error) {
	switch {
	case strings.Contains(query, "FROM sys.tables"):
		rows := [][]driver.Value{
			{"ptd", "BKPF"},
			{"ptd", "BSID"},
			{"ptd", "BSAD"},
			{"ptd", "EKBE"},
			{"ptd", "MKPF"},
			{"ptd", "MSEG"},
			{"ptd", "MARD"},
			{"ptd", "T001"},
			{"ptd", "T001W"},
		}
		return &validationTestRows{
			columns: []string{"schema_name", "name"},
			rows:    rows,
		}, nil
	case strings.Contains(query, "SELECT DISTINCT MANDT"):
		return &validationTestRows{
			columns: []string{"MANDT"},
			rows: [][]driver.Value{
				{"200"},
			},
		}, nil
	case strings.Contains(query, "SELECT COUNT(*) FROM"):
		return &validationTestRows{
			columns: []string{"count"},
			rows: [][]driver.Value{
				{int64(42)},
			},
		}, nil
	case strings.Contains(query, "SELECT DISTINCT WAERS"):
		return &validationTestRows{
			columns: []string{"WAERS"},
			rows: [][]driver.Value{
				{"EUR"},
				{"USD"},
			},
		}, nil
	case strings.Contains(query, "AS dataset_rows"):
		return &validationTestRows{
			columns: []string{"posting_yyyymm", "company_code", "fi_origin"},
			rows: [][]driver.Value{
				{"202603", "1000", "MKPF"},
			},
		}, nil
	default:
		return nil, fmt.Errorf("unexpected query: %s", query)
	}
}

type validationTestRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

func (r *validationTestRows) Columns() []string {
	return r.columns
}

func (r *validationTestRows) Close() error {
	return nil
}

func (r *validationTestRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}

	copy(dest, r.rows[r.index])
	r.index++
	return nil
}

var _ driver.QueryerContext = validationTestConn{}

#!/usr/bin/env bash
set -euo pipefail

HOST_ALIAS="${HOST_ALIAS:-${1:-msi-1}}"
DB="${DB:-PTD_READONLY}"
CLIENT="${CLIENT:-200}"
OUT_DIR="${OUT_DIR:-files/dashboard/output}"

mkdir -p "$OUT_DIR"

run_query() {
  local name="$1"
  local sql="$2"
  local out_file="$OUT_DIR/${name}.psv"

  printf 'Running %s -> %s\n' "$name" "$out_file"

  ssh "$HOST_ALIAS" powershell -NoProfile -NonInteractive -Command - <<EOF > "$out_file"
\$ErrorActionPreference = 'Stop'
sqlcmd -S localhost -E -C -d '$DB' -W -s '|' -Q "$sql"
EOF
}

run_query "01_database_identity" "SET NOCOUNT ON; SELECT DB_NAME() AS database_name, DATABASEPROPERTYEX(DB_NAME(), 'Updateability') AS updateability, DATABASEPROPERTYEX(DB_NAME(), 'Collation') AS collation_name, compatibility_level FROM sys.databases WHERE name = DB_NAME();"

run_query "02_fi_origin_monthly" "SET NOCOUNT ON; SELECT LEFT(BUDAT, 6) AS posting_yyyymm, BUKRS, ISNULL(NULLIF(AWTYP, ''), '[blank]') AS awtyp, COUNT(*) AS document_count FROM ptd.BKPF WHERE MANDT = '$CLIENT' GROUP BY LEFT(BUDAT, 6), BUKRS, ISNULL(NULLIF(AWTYP, ''), '[blank]') ORDER BY posting_yyyymm DESC, document_count DESC;"

run_query "03_billing_origin_fi" "SET NOCOUNT ON; SELECT LEFT(BUDAT, 6) AS posting_yyyymm, BUKRS, COUNT(*) AS billing_origin_fi_docs FROM ptd.BKPF WHERE MANDT = '$CLIENT' AND AWTYP = 'VBRK' GROUP BY LEFT(BUDAT, 6), BUKRS ORDER BY posting_yyyymm DESC, billing_origin_fi_docs DESC;"

run_query "04_open_ar_mix" "SET NOCOUNT ON; SELECT BUKRS, WAERS, BLART, COUNT(*) AS open_item_count, CAST(SUM(CAST(WRBTR AS decimal(38,2))) AS decimal(38,2)) AS open_doc_amount FROM ptd.BSID WHERE MANDT = '$CLIENT' GROUP BY BUKRS, WAERS, BLART ORDER BY open_item_count DESC, BUKRS, WAERS, BLART;"

run_query "05_cleared_ar_monthly" "SET NOCOUNT ON; SELECT LEFT(AUGDT, 6) AS clearing_yyyymm, BUKRS, BLART, COUNT(*) AS cleared_item_count FROM ptd.BSAD WHERE MANDT = '$CLIENT' GROUP BY LEFT(AUGDT, 6), BUKRS, BLART ORDER BY clearing_yyyymm DESC, cleared_item_count DESC;"

run_query "06_purchasing_history" "SET NOCOUNT ON; SELECT LEFT(BUDAT, 6) AS posting_yyyymm, VGABE, ISNULL(NULLIF(WERKS, ''), '[blank]') AS werks, COUNT(*) AS history_rows FROM ptd.EKBE WHERE MANDT = '$CLIENT' GROUP BY LEFT(BUDAT, 6), VGABE, ISNULL(NULLIF(WERKS, ''), '[blank]') ORDER BY posting_yyyymm DESC, history_rows DESC;"

run_query "07_inventory_movement" "SET NOCOUNT ON; SELECT LEFT(h.BUDAT, 6) AS posting_yyyymm, i.WERKS, i.BWART, COUNT(*) AS movement_rows FROM ptd.MKPF h JOIN ptd.MSEG i ON i.MANDT = h.MANDT AND i.MBLNR = h.MBLNR AND i.MJAHR = h.MJAHR WHERE h.MANDT = '$CLIENT' GROUP BY LEFT(h.BUDAT, 6), i.WERKS, i.BWART ORDER BY posting_yyyymm DESC, movement_rows DESC;"

run_query "08_current_stock" "SET NOCOUNT ON; SELECT d.WERKS, w.NAME1, d.LGORT, COUNT(*) AS material_rows, CAST(SUM(CAST(d.LABST AS decimal(38,3))) AS decimal(38,3)) AS unrestricted_stock, CAST(SUM(CAST(d.INSME AS decimal(38,3))) AS decimal(38,3)) AS quality_stock, CAST(SUM(CAST(d.SPEME AS decimal(38,3))) AS decimal(38,3)) AS blocked_stock FROM ptd.MARD d LEFT JOIN ptd.T001W w ON w.MANDT = d.MANDT AND w.WERKS = d.WERKS WHERE d.MANDT = '$CLIENT' GROUP BY d.WERKS, w.NAME1, d.LGORT ORDER BY unrestricted_stock DESC, material_rows DESC;"

printf 'Dashboard probe complete. Files written to %s\n' "$OUT_DIR"

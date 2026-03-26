-- Dashboard starter query pack for PTD_READONLY.
-- All queries are read-only and scoped to the validated client 200 slice.

USE PTD_READONLY;

-- 01. Database identity and safety check
SELECT
  DB_NAME() AS database_name,
  DATABASEPROPERTYEX(DB_NAME(), 'Updateability') AS updateability,
  DATABASEPROPERTYEX(DB_NAME(), 'Collation') AS collation_name;

-- 02. FI document origin by posting month and company code
SELECT
  LEFT(BUDAT, 6) AS posting_yyyymm,
  BUKRS,
  ISNULL(NULLIF(AWTYP, ''), '[blank]') AS awtyp,
  COUNT(*) AS document_count
FROM ptd.BKPF
WHERE MANDT = '200'
GROUP BY LEFT(BUDAT, 6), BUKRS, ISNULL(NULLIF(AWTYP, ''), '[blank]')
ORDER BY posting_yyyymm DESC, document_count DESC;

-- 03. Billing-origin FI slice by posting month
SELECT
  LEFT(BUDAT, 6) AS posting_yyyymm,
  BUKRS,
  COUNT(*) AS billing_origin_fi_docs
FROM ptd.BKPF
WHERE MANDT = '200'
  AND AWTYP = 'VBRK'
GROUP BY LEFT(BUDAT, 6), BUKRS
ORDER BY posting_yyyymm DESC, billing_origin_fi_docs DESC;

-- 04. Open AR grouped safely by company code, currency, and FI doc type
SELECT
  BUKRS,
  WAERS,
  BLART,
  COUNT(*) AS open_item_count,
  CAST(SUM(CAST(WRBTR AS decimal(38,2))) AS decimal(38,2)) AS open_doc_amount
FROM ptd.BSID
WHERE MANDT = '200'
GROUP BY BUKRS, WAERS, BLART
ORDER BY open_item_count DESC, BUKRS, WAERS, BLART;

-- 05. Cleared AR by clearing month and clearing document type
SELECT
  LEFT(AUGDT, 6) AS clearing_yyyymm,
  BUKRS,
  BLART,
  COUNT(*) AS cleared_item_count
FROM ptd.BSAD
WHERE MANDT = '200'
GROUP BY LEFT(AUGDT, 6), BUKRS, BLART
ORDER BY clearing_yyyymm DESC, cleared_item_count DESC;

-- 06. Purchasing history by category and plant
SELECT
  LEFT(BUDAT, 6) AS posting_yyyymm,
  VGABE,
  ISNULL(NULLIF(WERKS, ''), '[blank]') AS werks,
  COUNT(*) AS history_rows
FROM ptd.EKBE
WHERE MANDT = '200'
GROUP BY LEFT(BUDAT, 6), VGABE, ISNULL(NULLIF(WERKS, ''), '[blank]')
ORDER BY posting_yyyymm DESC, history_rows DESC;

-- 07. Inventory movement by month, plant, and movement type
SELECT
  LEFT(h.BUDAT, 6) AS posting_yyyymm,
  i.WERKS,
  i.BWART,
  COUNT(*) AS movement_rows
FROM ptd.MKPF h
JOIN ptd.MSEG i
  ON i.MANDT = h.MANDT
 AND i.MBLNR = h.MBLNR
 AND i.MJAHR = h.MJAHR
WHERE h.MANDT = '200'
GROUP BY LEFT(h.BUDAT, 6), i.WERKS, i.BWART
ORDER BY posting_yyyymm DESC, movement_rows DESC;

-- 08. Current stock by plant and storage location
SELECT
  d.WERKS,
  w.NAME1,
  d.LGORT,
  COUNT(*) AS material_rows,
  CAST(SUM(CAST(d.LABST AS decimal(38,3))) AS decimal(38,3)) AS unrestricted_stock,
  CAST(SUM(CAST(d.INSME AS decimal(38,3))) AS decimal(38,3)) AS quality_stock,
  CAST(SUM(CAST(d.SPEME AS decimal(38,3))) AS decimal(38,3)) AS blocked_stock
FROM ptd.MARD d
LEFT JOIN ptd.T001W w
  ON w.MANDT = d.MANDT
 AND w.WERKS = d.WERKS
WHERE d.MANDT = '200'
GROUP BY d.WERKS, w.NAME1, d.LGORT
ORDER BY unrestricted_stock DESC, material_rows DESC;

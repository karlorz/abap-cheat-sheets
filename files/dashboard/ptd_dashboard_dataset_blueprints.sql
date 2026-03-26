-- Dashboard dataset blueprints for PTD_READONLY.
-- Use these as native queries in Power BI Desktop, Metabase, or Superset.
-- They keep column names stable and stay inside validated transparent-table paths.

USE PTD_READONLY;

-- Dataset 01: FI document origin monthly
SELECT
  LEFT(b.BUDAT, 6) AS posting_yyyymm,
  b.BUKRS AS company_code,
  t.BUTXT AS company_name,
  ISNULL(NULLIF(b.AWTYP, ''), '[blank]') AS fi_origin,
  COUNT(*) AS fi_document_count
FROM ptd.BKPF b
LEFT JOIN ptd.T001 t
  ON t.MANDT = b.MANDT
 AND t.BUKRS = b.BUKRS
WHERE b.MANDT = '200'
GROUP BY LEFT(b.BUDAT, 6), b.BUKRS, t.BUTXT, ISNULL(NULLIF(b.AWTYP, ''), '[blank]');

-- Dataset 02: AR open item monthly
SELECT
  LEFT(s.BUDAT, 6) AS posting_yyyymm,
  s.BUKRS AS company_code,
  t.BUTXT AS company_name,
  s.WAERS AS currency_code,
  s.BLART AS document_type,
  COUNT(*) AS open_item_count,
  CAST(SUM(CAST(s.WRBTR AS decimal(38,2))) AS decimal(38,2)) AS open_doc_amount
FROM ptd.BSID s
LEFT JOIN ptd.T001 t
  ON t.MANDT = s.MANDT
 AND t.BUKRS = s.BUKRS
WHERE s.MANDT = '200'
GROUP BY LEFT(s.BUDAT, 6), s.BUKRS, t.BUTXT, s.WAERS, s.BLART;

-- Dataset 03: AR cleared item monthly
SELECT
  LEFT(s.AUGDT, 6) AS clearing_yyyymm,
  s.BUKRS AS company_code,
  t.BUTXT AS company_name,
  s.BLART AS original_document_type,
  cb.BLART AS clearing_document_type,
  COUNT(*) AS cleared_item_count
FROM ptd.BSAD s
LEFT JOIN ptd.T001 t
  ON t.MANDT = s.MANDT
 AND t.BUKRS = s.BUKRS
LEFT JOIN ptd.BKPF cb
  ON cb.MANDT = s.MANDT
 AND cb.BUKRS = s.BUKRS
 AND cb.BELNR = s.AUGBL
 AND cb.GJAHR = s.AUGGJ
WHERE s.MANDT = '200'
GROUP BY LEFT(s.AUGDT, 6), s.BUKRS, t.BUTXT, s.BLART, cb.BLART;

-- Dataset 04: Purchasing history monthly
SELECT
  LEFT(e.BUDAT, 6) AS posting_yyyymm,
  e.VGABE AS history_category,
  ISNULL(NULLIF(e.WERKS, ''), '[blank]') AS plant_code,
  w.NAME1 AS plant_name,
  COUNT(*) AS history_row_count,
  CAST(SUM(CAST(e.MENGE AS decimal(38,3))) AS decimal(38,3)) AS total_quantity,
  CAST(SUM(CAST(e.DMBTR AS decimal(38,2))) AS decimal(38,2)) AS total_local_amount
FROM ptd.EKBE e
LEFT JOIN ptd.T001W w
  ON w.MANDT = e.MANDT
 AND w.WERKS = e.WERKS
WHERE e.MANDT = '200'
GROUP BY LEFT(e.BUDAT, 6), e.VGABE, ISNULL(NULLIF(e.WERKS, ''), '[blank]'), w.NAME1;

-- Dataset 05: Inventory movement monthly
SELECT
  LEFT(h.BUDAT, 6) AS posting_yyyymm,
  i.WERKS AS plant_code,
  w.NAME1 AS plant_name,
  i.BWART AS movement_type,
  COUNT(*) AS movement_row_count
FROM ptd.MKPF h
JOIN ptd.MSEG i
  ON i.MANDT = h.MANDT
 AND i.MBLNR = h.MBLNR
 AND i.MJAHR = h.MJAHR
LEFT JOIN ptd.T001W w
  ON w.MANDT = i.MANDT
 AND w.WERKS = i.WERKS
WHERE h.MANDT = '200'
GROUP BY LEFT(h.BUDAT, 6), i.WERKS, w.NAME1, i.BWART;

-- Dataset 06: Current stock by plant and storage location
SELECT
  d.WERKS AS plant_code,
  w.NAME1 AS plant_name,
  d.LGORT AS storage_location,
  COUNT(*) AS material_row_count,
  CAST(SUM(CAST(d.LABST AS decimal(38,3))) AS decimal(38,3)) AS unrestricted_stock,
  CAST(SUM(CAST(d.INSME AS decimal(38,3))) AS decimal(38,3)) AS quality_stock,
  CAST(SUM(CAST(d.SPEME AS decimal(38,3))) AS decimal(38,3)) AS blocked_stock
FROM ptd.MARD d
LEFT JOIN ptd.T001W w
  ON w.MANDT = d.MANDT
 AND w.WERKS = d.WERKS
WHERE d.MANDT = '200'
GROUP BY d.WERKS, w.NAME1, d.LGORT;

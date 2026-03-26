-- Dataset catalog: Purchasing history monthly
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

-- Dataset catalog: Current stock by plant and storage location
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

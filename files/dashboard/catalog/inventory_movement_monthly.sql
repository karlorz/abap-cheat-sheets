-- Dataset catalog: Inventory movement monthly
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

-- Dataset catalog: AR open item monthly
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

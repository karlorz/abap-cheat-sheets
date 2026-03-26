-- Dataset catalog: AR cleared item monthly
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

-- Dataset catalog: FI document origin monthly
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

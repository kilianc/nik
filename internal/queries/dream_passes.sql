SELECT
  pass,
  content,
  completed_at
FROM dream
WHERE date = ?1
ORDER BY pass ASC

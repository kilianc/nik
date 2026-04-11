SELECT
  key,
  value,
  created_at,
  updated_at
FROM setting
WHERE key = ?1;

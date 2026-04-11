INSERT INTO setting (key, value)
VALUES (?1, ?2)
ON CONFLICT (key) DO UPDATE SET
  value = excluded.value,
  updated_at = NOW_ISO8601_MS();

INSERT INTO skill (
  id,
  name,
  status,
  content_hash,
  install_hash,
  created_at,
  updated_at
)
VALUES (?1, ?2, ?3, ?4, ?5, NOW_ISO8601_MS(), NOW_ISO8601_MS())
ON CONFLICT(name) DO UPDATE SET
  status = excluded.status,
  content_hash = excluded.content_hash,
  install_hash = excluded.install_hash,
  updated_at = NOW_ISO8601_MS()
RETURNING
  id,
  name,
  status,
  content_hash,
  install_hash,
  created_at,
  updated_at


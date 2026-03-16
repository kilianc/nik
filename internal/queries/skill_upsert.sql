INSERT INTO skill (
  id,
  name,
  status,
  content_hash,
  install_hash,
  created_at,
  updated_at
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?6)
ON CONFLICT(name) DO UPDATE SET
  status = excluded.status,
  content_hash = excluded.content_hash,
  install_hash = excluded.install_hash,
  updated_at = excluded.updated_at
RETURNING
  id,
  name,
  status,
  content_hash,
  install_hash,
  created_at,
  updated_at

SELECT
  id,
  name,
  kind,
  content_hash,
  install_hash,
  created_at
FROM skill_event
WHERE created_at >= ISO8601_MS(?1)
ORDER BY created_at ASC

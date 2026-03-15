SELECT
  id,
  name,
  kind,
  content_hash,
  install_hash,
  created_at
FROM skill_event
WHERE created_at >= ?1
ORDER BY created_at ASC

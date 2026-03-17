SELECT
  id,
  name,
  kind,
  content_hash,
  install_hash,
  created_at
FROM skill_event
WHERE id IN (
  SELECT id
  FROM skill_event se2
  WHERE se2.name = skill_event.name
  ORDER BY se2.created_at DESC, se2.id DESC
  LIMIT 1
)

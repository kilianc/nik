SELECT
  meta,
  created_at
FROM skill_reflex
WHERE skill_name = ?1
ORDER BY created_at DESC, id DESC
LIMIT 1

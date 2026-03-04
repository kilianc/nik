SELECT
  id,
  name,
  prompt,
  created_at
FROM crew_member
WHERE id = ?1
  OR name = ?1

-- ?1: day start (UTC), ?2: day end (UTC)
SELECT
  id,
  name,
  nicknames,
  one_liner,
  created_at
FROM contact
WHERE created_at >= datetime(?1) AND created_at < datetime(?2)
ORDER BY created_at ASC

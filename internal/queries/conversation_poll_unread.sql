-- ?1: allowed conversation ids (JSON array TEXT)
SELECT
  id
FROM conversation
WHERE id IN (SELECT value FROM json_each(?1))
  AND last_message_at > COALESCE(last_read_at, '1970-01-01')
ORDER BY last_message_at ASC;

-- ?1: allowed conversation ids (JSON array TEXT)
SELECT
  c.id
FROM conversation c
WHERE c.id IN (SELECT value FROM json_each(?1))
  AND COALESCE((
    SELECT MAX(m.sent_at) FROM message m
    WHERE m.conversation_id = c.id
      AND m.is_from_me = 0
  ), '1970-01-01') > COALESCE(c.last_read_at, '1970-01-01')
ORDER BY c.last_message_at ASC;

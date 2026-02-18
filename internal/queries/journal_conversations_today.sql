-- ?1: day start (UTC), ?2: day end (UTC)
SELECT
  c.id,
  c.platform,
  c.kind,
  c.title,
  COUNT(m.id) AS message_count
FROM conversation c
JOIN message m ON m.conversation_id = c.id
WHERE m.sent_at >= ?1 AND m.sent_at < ?2
GROUP BY c.id
ORDER BY COUNT(m.id) DESC

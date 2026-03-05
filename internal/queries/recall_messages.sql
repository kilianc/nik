-- ?1: since timestamp (only messages after this time)
SELECT
  m.body,
  m.sent_at,
  m.is_from_me,
  COALESCE(c.name, '') AS sender_name,
  COALESCE(conv.title, '') AS conversation_title,
  conv.kind
FROM message m
LEFT JOIN contact c ON c.id = m.contact_id
LEFT JOIN conversation conv ON conv.id = m.conversation_id
WHERE m.body != ''
  AND m.sent_at >= ?1
ORDER BY m.sent_at ASC

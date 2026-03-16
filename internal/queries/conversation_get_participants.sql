-- ?1: conversation id (UUID string)
SELECT
  cp.id,
  cp.contact_id,
  cp.display_name,
  c.name,
  c.timezone,
  c.location,
  c.one_liner
FROM conversation_participant cp
LEFT JOIN contact c ON c.id = cp.contact_id
WHERE cp.conversation_id = ?1
ORDER BY cp.created_at ASC;

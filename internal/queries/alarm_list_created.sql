SELECT
  id,
  origin_contact_id,
  origin_conversation_id,
  goal,
  recurrence,
  next_fire_at,
  last_fired_at,
  created_at
FROM alarm
WHERE origin_conversation_id = ?1
  AND created_at >= ?2
  AND cancelled_at IS NULL
ORDER BY created_at

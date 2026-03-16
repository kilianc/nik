-- ?1: identifier (alarm UUID or goal prefix match)
SELECT
  id,
  origin_contact_id,
  origin_conversation_id,
  goal,
  recurrence,
  last_occurrence_note,
  next_fire_at,
  last_fired_at,
  created_at
FROM alarm
WHERE cancelled_at IS NULL
  AND (id = ?1 OR goal LIKE ?1 || '%')
LIMIT 1

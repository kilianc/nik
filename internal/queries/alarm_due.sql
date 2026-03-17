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
WHERE datetime(next_fire_at) <= datetime(ISO8601_MS(?1))
  AND cancelled_at IS NULL
  AND (last_fired_at IS NULL OR datetime(last_fired_at) < datetime(next_fire_at))
ORDER BY next_fire_at ASC

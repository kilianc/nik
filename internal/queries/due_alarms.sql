SELECT
  id,
  origin_contact_id,
  origin_conversation_id,
  goal,
  recurrence,
  source,
  source_id,
  next_fire_at,
  last_fired_at,
  created_at
FROM alarm
WHERE next_fire_at IS NOT NULL
  AND next_fire_at <= ?1
  AND cancelled_at IS NULL
  AND (last_fired_at IS NULL OR last_fired_at < next_fire_at)
ORDER BY next_fire_at ASC

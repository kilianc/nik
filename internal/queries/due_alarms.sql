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
WHERE next_fire_at IS NOT NULL
  AND datetime(next_fire_at) <= datetime(?1)
  AND cancelled_at IS NULL
  AND (last_fired_at IS NULL OR datetime(last_fired_at) < datetime(next_fire_at))
ORDER BY next_fire_at ASC

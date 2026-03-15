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
WHERE cancelled_at IS NULL
  AND recurrence IS NOT NULL
  AND datetime(last_fired_at) >= datetime(next_fire_at)
  AND datetime(last_fired_at, '+30 minutes') <= datetime(?1)
ORDER BY next_fire_at ASC

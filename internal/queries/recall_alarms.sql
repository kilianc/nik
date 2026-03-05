SELECT
  goal,
  recurrence,
  next_fire_at,
  cancelled_at,
  created_at
FROM alarm
ORDER BY created_at DESC

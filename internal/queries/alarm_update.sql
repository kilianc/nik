UPDATE alarm
SET goal = COALESCE(?2, goal),
    recurrence = COALESCE(?3, recurrence),
    next_fire_at = COALESCE(?4, next_fire_at)
WHERE id = ?1

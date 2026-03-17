UPDATE alarm
SET goal = COALESCE(?2, goal),
    recurrence = COALESCE(?3, recurrence),
    next_fire_at = COALESCE(NULLABLE_ISO8601_MS(?4), next_fire_at),
    last_fired_at = COALESCE(NULLABLE_ISO8601_MS(?5), last_fired_at),
    last_occurrence_note = CASE
      WHEN ?6 THEN ?7
      ELSE last_occurrence_note
    END
WHERE id = ?1

-- ?1: session_id, ?2: output, ?3: exit_code, ?4: alive
UPDATE shell_output
SET output     = ?2,
    exit_code  = ?3,
    alive      = ?4,
    updated_at = NOW_ISO8601_MS()
WHERE session_id = ?1;

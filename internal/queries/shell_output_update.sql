UPDATE shell_output
SET output     = ?2,
    exit_code  = ?3,
    alive      = ?4,
    updated_at = datetime('now')
WHERE session_id = ?1;

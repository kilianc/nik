INSERT INTO shell_output (session_id, command, description, output, exit_code, alive, created_at, updated_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, datetime('now'), datetime('now'))
ON CONFLICT (session_id) DO UPDATE SET
  output     = ?4,
  exit_code  = ?5,
  alive      = ?6,
  updated_at = datetime('now');

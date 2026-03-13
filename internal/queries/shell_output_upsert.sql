INSERT INTO shell_output (session_id, activation_id, command, description, output, exit_code, alive, created_at, updated_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, datetime('now'), datetime('now'))
ON CONFLICT (session_id) DO UPDATE SET
  output     = ?5,
  exit_code  = ?6,
  alive      = ?7,
  updated_at = datetime('now');

INSERT INTO shell_output (id, session_id, activation_id, command, description, output, exit_code, alive, created_at, updated_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, datetime('now'), datetime('now'))
ON CONFLICT (session_id) DO UPDATE SET
  output     = ?6,
  exit_code  = ?7,
  alive      = ?8,
  updated_at = datetime('now');

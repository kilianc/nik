-- ?1: id, ?2: session_id, ?3: activation_id, ?4: command, ?5: description, ?6: output, ?7: exit_code, ?8: alive
INSERT INTO shell_output (id, session_id, activation_id, command, description, output, exit_code, alive, created_at, updated_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, NOW_ISO8601_MS(), NOW_ISO8601_MS())
ON CONFLICT (session_id) DO UPDATE SET
  output     = ?6,
  exit_code  = ?7,
  alive      = ?8,
  updated_at = NOW_ISO8601_MS();

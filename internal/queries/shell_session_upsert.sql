-- ?1: id, ?2: activation_id, ?3: command, ?4: description, ?5: output, ?6: exit_code, ?7: alive
INSERT INTO shell_session (id, activation_id, command, description, output, exit_code, alive, created_at, updated_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, NOW_ISO8601_MS(), NOW_ISO8601_MS())
ON CONFLICT (id) DO UPDATE SET
  output     = ?5,
  exit_code  = ?6,
  alive      = ?7,
  updated_at = NOW_ISO8601_MS()

-- ?1: conversation id (UUID string), ?2: read timestamp
UPDATE conversation
SET
  last_read_at = ISO8601_MS(?2),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1;

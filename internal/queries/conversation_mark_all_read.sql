-- ?1: platform
UPDATE conversation
SET
  last_read_at = COALESCE(last_message_at, NOW_ISO8601_MS()),
  updated_at = NOW_ISO8601_MS()
WHERE platform = ?1;

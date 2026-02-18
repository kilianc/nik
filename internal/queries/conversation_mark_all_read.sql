-- ?1: platform
UPDATE conversation
SET
  last_read_at = COALESCE(last_message_at, datetime('now')),
  updated_at = datetime('now')
WHERE platform = ?1;

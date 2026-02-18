-- ?1: conversation id (UUID string), ?2: read timestamp
UPDATE conversation
SET
  last_read_at = ?2,
  updated_at = datetime('now')
WHERE id = ?1;

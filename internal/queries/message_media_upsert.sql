-- ?1: message_id, ?2: media_id
INSERT INTO message_media (
  message_id,
  media_id,
  created_at
)
VALUES (?1, ?2, datetime('now'))
ON CONFLICT (message_id) DO UPDATE SET
  media_id = excluded.media_id;

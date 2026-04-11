INSERT OR IGNORE INTO conversation (
  id,
  platform,
  external_conversation_id,
  kind,
  title
)
VALUES (?1, 'local', ?1, 'dm', 'local')

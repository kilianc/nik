SELECT EXISTS(
  SELECT 1 FROM message
  WHERE platform = ?1
  AND external_message_id = ?2
)

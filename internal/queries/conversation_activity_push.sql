-- ?1: conversation_id, ?2: state
UPDATE conversation SET
  activity = json_insert(activity, '$[#]', ?2),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1
  AND NOT EXISTS (SELECT 1 FROM json_each(activity) WHERE value = ?2)

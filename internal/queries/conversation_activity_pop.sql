-- ?1: conversation_id, ?2: state
UPDATE conversation SET
  activity = COALESCE(
    (SELECT json_group_array(value) FROM json_each(activity) WHERE value != ?2),
    '[]'
  ),
  updated_at = NOW_ISO8601_MS()
WHERE id = ?1

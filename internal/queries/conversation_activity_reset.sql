UPDATE conversation SET
  activity = '[]',
  updated_at = NOW_ISO8601_MS()
WHERE activity != '[]'

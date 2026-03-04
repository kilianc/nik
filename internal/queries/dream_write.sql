INSERT INTO dream (date, pass, content, completed_at)
VALUES (?1, ?2, ?3, datetime('now'))
ON CONFLICT(date, pass) DO UPDATE SET
  content = excluded.content,
  completed_at = excluded.completed_at

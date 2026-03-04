INSERT INTO journal (date, content, completed_at)
VALUES (?1, ?2, datetime('now'))
ON CONFLICT(date) DO UPDATE SET
  content = excluded.content,
  completed_at = excluded.completed_at

INSERT INTO task (id, conversation_id, contact_id, retry_for_task_id, retry_number, goal, plan, thinking, status, created_at)
VALUES (?1, ?2, ?3, ?4, ?5, ?6, ?7, ?8, ?9, ISO8601_MS(?10))

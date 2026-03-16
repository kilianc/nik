INSERT INTO activation_detail (
  id,
  activation_id,
  instructions,
  user_input,
  tools,
  reasoning_summaries
)
VALUES (?1, ?2, ?3, ?4, ?5, ?6)
ON CONFLICT(activation_id) DO UPDATE SET
  instructions = excluded.instructions,
  user_input = excluded.user_input,
  tools = excluded.tools,
  reasoning_summaries = excluded.reasoning_summaries

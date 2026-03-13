INSERT OR REPLACE INTO activation_detail (
  activation_id,
  instructions,
  user_input,
  tools,
  reasoning_summaries
)
VALUES (?1, ?2, ?3, ?4, ?5)

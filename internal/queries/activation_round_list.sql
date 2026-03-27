SELECT
  id,
  activation_id,
  round,
  user_input,
  model_output,
  messages,
  reasoning_summaries,
  input_tokens,
  output_tokens,
  cached_tokens,
  reasoning_tokens,
  created_at
FROM activation_round
WHERE activation_id = ?1
  AND (?2 IS NULL OR round < ?2)
ORDER BY round ASC

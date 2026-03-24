SELECT
  tc.name,
  tc.input,
  tc.output,
  ar.round,
  ar.id
FROM tool_call tc
JOIN activation_round ar ON ar.id = tc.activation_round_id
WHERE tc.activation_id = ?1
  AND (?2 IS NULL OR ar.round = ?2)
ORDER BY ar.round ASC, tc.created_at ASC;

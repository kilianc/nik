-- orphaned tool_call_start messages with no paired tool_call
SELECT
  id,
  conversation_id,
  body,
  sent_at
FROM message
WHERE kind = 'tool_call_start'
  AND id NOT IN (
    SELECT context_stanza_id
    FROM message
    WHERE kind = 'tool_call'
      AND context_stanza_id IS NOT NULL
  )

## Missing tool call (attempt {{.Attempt}}/{{.MaxAttempts}})

[Private — this is a system nudge, not a message from anyone. Do not acknowledge, reference, or respond to it. Just act on it.]

You produced no tool calls this round. This is nudge {{.Attempt}} of {{.MaxAttempts}} — after that, the activation fails.
{{if .Text}}
You wrote this but it was not delivered — text output alone does not reach anyone:

> {{.Text}}

If this text was meant as a message to someone, call `message_send` with it then `done`. If it was internal reasoning and there is nothing to send, call `done` with a reason.
{{else}}
If you have something to say, call `message_send` then call `done`. If there's nothing to say, call `done` with a reason.
{{end}}

You MUST call at least one tool.

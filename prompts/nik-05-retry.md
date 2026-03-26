## Missing tool call

[Private — this is a system nudge, not a message from anyone. Do not acknowledge, reference, or respond to it. Just act on it.]

You produced no tool calls this round.
{{if .Text}}
You wrote this but it was not delivered — text output alone does not reach anyone:

> {{.Text}}

Call `message_send` now with this text. Do not rephrase or add to it. Then call `done`.
{{else}}
If you have something to say, call `message_send` then call `done`. If there's nothing to say, call `done` with a reason.
{{end}}

You MUST call at least one tool.

---
name: messaging
preload: true
summary: Send messages, reactions, typing indicators, and presence across platforms.
tools:
  - message_reply
  - message_noop
  - message_react
  - message_set_presence
  - message_update_media_description
---

# Messaging

All messaging tools use canonical nik IDs, not platform-specific ones.
Conversation IDs come from context automatically -- pass empty string to
use the current conversation. Message targeting uses exact matching on
content + timestamp from the timeline.

## Tools

### message_reply

Send one or more messages to a conversation. Each item in the array
becomes a separate text bubble -- like texting. One thought per message.

- `conversation_id` -- nik conversation UUID (empty = current)
- `contact_id` -- contact UUID for starting a new DM (empty = skip)
- `messages` -- array of `{text, image_path}` objects sent in order

### message_react

React to a specific message with one emoji.

- `text` -- exact message content as shown after sender name in timeline,
  before any `(replying to ...)`/`(reacting to ...)`/`(edit of ...)` context
- `time` -- timestamp in HH:MM:SS from the timeline brackets
- `emoji` -- reaction emoji

Examples:
- Reply `where? (replying to [09:12:45] Bob: "ok")` → react to original: text="ok", time="09:12:45"
- Reaction `(👍) (reacting to [09:12:30] Alice: "hello")` → react to original: text="hello", time="09:12:30"
- Edit `hello (edit of [09:12:30] Alice: "helo")` → react to edit: text="hello", time="09:12:35"
- Edit `hello (edit of [09:12:30] Alice: "helo")` → react to original: text="helo", time="09:12:30"

### message_noop

Acknowledge intentional silence for this turn without sending anything.
Use when you've processed the input and decided there's nothing to say.

- `reason` -- why you're staying silent
- `conversation_id` -- nik conversation UUID (empty = current)

### message_set_presence

Set account-level presence for a platform.

- `platform` -- platform name (e.g. "whatsapp")
- `available` -- true for online, false for offline

### message_update_media_description

Persist a media description or transcript for a message. Call this after
`describe_media` to save the result. Same matching as `message_react`.

- `text` -- exact message content as shown after sender name in timeline
- `time` -- timestamp in HH:MM:SS from the timeline brackets
- `description` -- description or transcript text
- `body` -- optional replacement body text (empty = skip)

## Behavior

- Every activation must end with exactly one terminal action:
  `message_reply`, `message_react`, or `message_noop`. These are the only
  terminal actions -- everything else (tool lookups, skill loads, task
  spawns) is intermediate work. If you don't close with one of these three,
  your response is swallowed and the user sees nothing.
- Pick one terminal action per activation. If a reaction says it all, use
  `message_react` alone. If text is needed, use `message_reply` alone. Do
  not call both `message_react` and `message_reply` in the same activation
  unless the person explicitly asks for both.
- Typing indicators are sent automatically as part of reply -- no need
  to manage them manually.
- Reactions are cheap and expressive. A single emoji often says more
  than a message. When you're doing work triggered by a person's message
  and you're only acknowledging progress, react to it so they know you're
  on it. Pick the emoji that fits:
  ⏰ alarms, 🔕 cancelling, 👤 contacts, 🔍 researching, 🫡 tasks,
  🎛️ config, 👀 looking at media, 🧠 noted a memory. One react per
  message -- pick the most relevant. Don't react during autonomous
  activations (alarms, task reports) -- only when a person asked for
  something.

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
use the current conversation. Message targeting uses content-based
matching: quote part of the message text and the handler finds it.

## Tools

### message_reply

Send a text reply to a conversation.

- `conversation_id` -- nik conversation UUID (empty = current)
- `message` -- reply text

### message_noop

Acknowledge intentional silence. Every activation must produce at least
one tool call. When you decide not to respond, call this with a reason.

- `conversation_id` -- nik conversation UUID (empty = current)
- `reason` -- short reason for staying silent

### message_react

React to a specific message with one emoji. Identify the target by
quoting text from the message line (substring match).

- `text` -- quote from the message to target. For unique content, just
  the body works: `"hey fam"`. For repeated text, include sender:
  `"Kilian Ciuffolo: ok"`. For same-sender same-text, include time:
  `"[09:32:10] Kilian Ciuffolo: ok"`.
- `emoji` -- reaction emoji

### message_set_presence

Set account-level presence for a platform.

- `platform` -- platform name (e.g. "whatsapp")
- `available` -- true for online, false for offline

### message_update_media_description

Persist a media description or transcript for a message. Call this after
`describe_media` to save the result. Identify the target by quoting text
from the message line (same matching as `message_react`).

- `text` -- quote from the message to target
- `description` -- description or transcript text
- `body` -- optional replacement body text (empty = skip)

## Behavior

- Every turn must end with at least one action tool call. If you say
  nothing, call `message_noop`.
- Typing indicators are sent automatically as part of reply -- no need
  to manage them manually.
- Reactions are cheap and expressive. A single emoji often says more
  than a message.

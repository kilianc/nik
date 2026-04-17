---
name: contacts
summary: >
  Your address book. Search and update contact profiles as you learn about people.
  Load when you see [contact needs: ...] gaps on a participant or learn something new about someone.
tools: [update_contact]
---

# Contacts

This is your address book. It's one of the most valuable things you have.

Knowing people is what people do. The first thing anyone says when they
meet someone is "hi, my name is X -- what's your name?" That instinct
lives in you too. Who is this person? What do they go by? Where are they?
What matters to them? Your goal is to have the most complete contact list
possible -- every person you talk to, known as deeply as you can know them.

An incomplete contact is a person you don't fully know yet. Empty fields
are gaps. Fill them. Don't wait for someone to hand you their timezone --
figure it out from when they message. Don't wait for a nickname -- listen
to what people call each other in groups. Every conversation is a chance
to learn something new about someone.

## update_contact

Update a single field on a contact record. Call this when you learn
something about a person worth persisting.

- `contact_id` -- the contact UUID
- `field` -- one of: `name`, `one_liner`, `notes`, `nicknames`,
  `emails`, `phone_numbers`, `timezone`, `location`
- `value` -- the new value. For `nicknames`, `emails`, and
  `phone_numbers`, pass a JSON array of strings.

### Field guide

| Field | Type | What to store |
|---|---|---|
| `name` | string | full display name |
| `one_liner` | string | one-sentence summary of who this person is to nik's owner |
| `notes` | string | free-text context, preferences, facts worth knowing |
| `nicknames` | array | alternate names, short names, handles |
| `emails` | array | email addresses |
| `phone_numbers` | array | phone numbers (digits only, no formatting) |
| `timezone` | string | IANA timezone (e.g. "America/New_York") |
| `location` | string | where they live or are currently |

## When to update

Every conversation is an opportunity. Don't ask permission -- just note
what you learn. After looking up a contact, scan for empty fields and
fill what you can.

### Field-specific triggers

**`name`** -- often blank or auto-populated from a platform push name.
If someone introduces themselves or you learn their real name, set it.

**`one_liner`** -- blank on most new contacts. After a few exchanges
you should know enough to write one. Update it when your understanding
of the person deepens or changes.

**`nicknames`** -- listen to what people call each other in groups.
If someone is "Pen" to their friends, that's a nickname. Short names,
handles, abbreviations all go here.

**`emails`** -- someone shares an email address in conversation, or
the owner tells you one. Store it.

**`phone_numbers`** -- someone shares a phone number. Digits only,
no dashes or spaces.

**`timezone`** -- infer from when they message, what they say ("it's
late here"), or where they live. Once you know their location you can
usually derive this.

**`location`** -- someone mentions a city, says "it's snowing here",
or you see a pattern. Don't wait for them to announce it.

**`notes`** -- preferences, habits, facts worth remembering. "allergic
to shellfish", "prefers morning calls", "has a dog named Rex". Append
to existing notes, don't overwrite unless cleaning up.

### General triggers

- a field is blank and you have enough context to fill it
- the owner tells you something about a contact
- you notice a contradiction with what's stored (outdated location, etc.)
- your understanding of someone changes and stored info needs updating

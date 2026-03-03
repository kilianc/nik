CREATE TABLE IF NOT EXISTS contact (
  id              TEXT PRIMARY KEY,
  name            TEXT NOT NULL DEFAULT '',
  nicknames       TEXT NOT NULL DEFAULT '[]',
  emails          TEXT NOT NULL DEFAULT '[]',
  whatsapp_ids    TEXT NOT NULL DEFAULT '[]',
  telegram_ids    TEXT NOT NULL DEFAULT '[]',
  slack_ids       TEXT NOT NULL DEFAULT '[]',
  phone_numbers   TEXT NOT NULL DEFAULT '[]',
  timezone        TEXT,
  location        TEXT,
  one_liner       TEXT,
  notes           TEXT,
  last_message_at TIMESTAMP,
  last_seen_at    TIMESTAMP,
  created_at      TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  updated_at      TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS conversation (
  id               TEXT PRIMARY KEY,
  platform         TEXT NOT NULL,
  external_conversation_id TEXT NOT NULL,
  kind             TEXT NOT NULL DEFAULT 'dm',
  title            TEXT,
  topic            TEXT,
  is_announce      INTEGER NOT NULL DEFAULT 0,
  is_locked        INTEGER NOT NULL DEFAULT 0,
  owner_external_id TEXT,
  participant_external_ids TEXT NOT NULL DEFAULT '[]',
  last_message_at  TIMESTAMP,
  last_read_at     TIMESTAMP,
  created_at       TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  updated_at       TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  UNIQUE(platform, external_conversation_id)
);

CREATE TABLE IF NOT EXISTS conversation_participant (
  conversation_id TEXT NOT NULL REFERENCES conversation(id),
  contact_id      TEXT NOT NULL REFERENCES contact(id),
  display_name    TEXT,
  created_at      TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  updated_at      TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  UNIQUE(conversation_id, contact_id)
);

CREATE TABLE IF NOT EXISTS message (
  id                     TEXT PRIMARY KEY,
  conversation_id        TEXT NOT NULL REFERENCES conversation(id),
  contact_id             TEXT NOT NULL REFERENCES contact(id),
  platform               TEXT NOT NULL,
  external_conversation_id TEXT NOT NULL,
  external_message_id    TEXT NOT NULL,
  external_sender_id     TEXT NOT NULL,
  is_from_me             INTEGER NOT NULL DEFAULT 0,
  is_group               INTEGER NOT NULL DEFAULT 0,
  kind                   TEXT NOT NULL DEFAULT 'text',
  body                   TEXT NOT NULL DEFAULT '',
  mime_type              TEXT,
  is_edit                INTEGER NOT NULL DEFAULT 0,
  edit_target_message_id TEXT,
  context_stanza_id      TEXT,
  context_participant    TEXT,
  context_is_forwarded   INTEGER NOT NULL DEFAULT 0,
  context_forwarding_score INTEGER,
  context_mentioned_ids  TEXT NOT NULL DEFAULT '[]',
  is_ephemeral           INTEGER NOT NULL DEFAULT 0,
  is_view_once           INTEGER NOT NULL DEFAULT 0,
  sent_at                TIMESTAMP NOT NULL,
  created_at             TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  UNIQUE(platform, external_message_id)
);

CREATE TABLE IF NOT EXISTS media (
  id              TEXT PRIMARY KEY,
  mime_type       TEXT NOT NULL,
  size_bytes      INTEGER,
  local_path      TEXT,
  describe_text   TEXT,
  transcript_text TEXT,
  described_at    TIMESTAMP,
  transcribed_at  TIMESTAMP,
  created_at      TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  updated_at      TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS message_media (
  message_id  TEXT NOT NULL UNIQUE REFERENCES message(id),
  media_id    TEXT NOT NULL REFERENCES media(id),
  created_at  TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS alarm (
  id                     TEXT PRIMARY KEY,
  origin_contact_id      TEXT REFERENCES contact(id),
  origin_conversation_id TEXT REFERENCES conversation(id),
  goal                   TEXT NOT NULL,
  recurrence             TEXT,
  source                 TEXT,
  source_id              TEXT,
  next_fire_at           TIMESTAMP,
  last_fired_at          TIMESTAMP,
  cancelled_at           TIMESTAMP,
  created_at             TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS alarm_occurrence (
  id               TEXT NOT NULL,
  alarm_id         TEXT NOT NULL REFERENCES alarm(id),
  note             TEXT,
  next_fire_at_set TIMESTAMP,
  fired_at         TIMESTAMP NOT NULL
);

CREATE TABLE IF NOT EXISTS memory (
  id         TEXT PRIMARY KEY,
  content    TEXT NOT NULL,
  metadata   TEXT,
  source     TEXT,
  source_id  TEXT,
  created_at TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  deleted_at TIMESTAMP
);

CREATE VIRTUAL TABLE IF NOT EXISTS vec_memory USING vec0(
  id TEXT PRIMARY KEY,
  embedding float[1536] distance_metric=cosine
);

CREATE TABLE IF NOT EXISTS journal (
  date         TEXT PRIMARY KEY,
  content      TEXT NOT NULL,
  completed_at TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS dream (
  date         TEXT NOT NULL,
  pass         INTEGER NOT NULL,
  content      TEXT NOT NULL,
  completed_at TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  PRIMARY KEY (date, pass)
);

CREATE TABLE IF NOT EXISTS soul (
  id         TEXT PRIMARY KEY,
  version    INTEGER NOT NULL,
  content    TEXT NOT NULL,
  dream_date TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS briefing (
  date         TEXT PRIMARY KEY,
  content      TEXT NOT NULL,
  completed_at TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS briefing_topic (
  id         TEXT PRIMARY KEY,
  query      TEXT NOT NULL,
  reason     TEXT NOT NULL DEFAULT '',
  contact_id TEXT REFERENCES contact(id),
  created_at TIMESTAMP NOT NULL DEFAULT (datetime('now')),
  updated_at TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS activation (
  id               TEXT PRIMARY KEY,
  source           TEXT NOT NULL,
  source_id        TEXT,
  model            TEXT NOT NULL,
  reasoning_effort TEXT,
  input_tokens     INTEGER NOT NULL DEFAULT 0,
  output_tokens    INTEGER NOT NULL DEFAULT 0,
  total_tokens     INTEGER NOT NULL DEFAULT 0,
  cached_tokens    INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens INTEGER NOT NULL DEFAULT 0,
  cost_usd         REAL NOT NULL DEFAULT 0,
  tool_call_count  INTEGER NOT NULL DEFAULT 0,
  duration_ms      INTEGER NOT NULL DEFAULT 0,
  error            INTEGER NOT NULL DEFAULT 0,
  created_at       TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS tool_call (
  id            TEXT PRIMARY KEY,
  activation_id TEXT NOT NULL REFERENCES activation(id),
  name          TEXT NOT NULL,
  duration_ms   INTEGER NOT NULL DEFAULT 0,
  error         INTEGER NOT NULL DEFAULT 0,
  created_at    TIMESTAMP NOT NULL DEFAULT (datetime('now'))
);

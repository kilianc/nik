-- people nik knows (name, nicknames, emails, whatsapp_ids, phone_numbers, timezone, location, notes)
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
  created_at      TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at      TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (last_message_at IS NULL OR IS_ISO8601_MS(last_message_at)),
  CHECK (last_seen_at IS NULL OR IS_ISO8601_MS(last_seen_at)),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- chat threads (platform, kind, title, topic, participants)
CREATE TABLE IF NOT EXISTS conversation (
  id                       TEXT PRIMARY KEY,
  platform                 TEXT NOT NULL CHECK(platform IN ('whatsapp')),
  external_conversation_id TEXT NOT NULL,
  kind                     TEXT NOT NULL DEFAULT 'dm' CHECK(kind IN ('dm', 'group')),
  title                    TEXT,
  topic                    TEXT,
  is_announce              INTEGER NOT NULL DEFAULT 0,
  is_locked                INTEGER NOT NULL DEFAULT 0,
  owner_external_id        TEXT,
  participant_external_ids TEXT NOT NULL DEFAULT '[]',
  last_message_at          TIMESTAMP,
  last_read_at             TIMESTAMP,
  created_at               TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at               TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (last_message_at IS NULL OR IS_ISO8601_MS(last_message_at)),
  CHECK (last_read_at IS NULL OR IS_ISO8601_MS(last_read_at)),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at)),
  UNIQUE(platform, external_conversation_id)
);

-- links contacts to conversations (display_name)
CREATE TABLE IF NOT EXISTS conversation_participant (
  id              TEXT PRIMARY KEY,
  conversation_id TEXT NOT NULL REFERENCES conversation(id),
  contact_id      TEXT NOT NULL REFERENCES contact(id),
  display_name    TEXT,
  created_at      TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at      TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at)),
  UNIQUE(conversation_id, contact_id)
);

-- all messages (conversation_id, contact_id, platform, kind, body, sent_at)
CREATE TABLE IF NOT EXISTS message (
  id                       TEXT PRIMARY KEY,
  conversation_id          TEXT NOT NULL REFERENCES conversation(id),
  contact_id               TEXT NOT NULL REFERENCES contact(id),
  platform                 TEXT NOT NULL CHECK(platform IN ('whatsapp', 'system')),
  external_conversation_id TEXT NOT NULL,
  external_message_id      TEXT NOT NULL,
  external_sender_id       TEXT NOT NULL,
  is_from_me               INTEGER NOT NULL DEFAULT 0,
  is_group                 INTEGER NOT NULL DEFAULT 0,
  kind                     TEXT NOT NULL DEFAULT 'text' CHECK(kind IN (
    'text', 'image', 'audio', 'video', 'ptv', 'document',
    'sticker', 'reaction', 'location', 'contact', 'poll',
    'task_report', 'task_spawned', 'task_retry', 'task_cancelled',
    'alarm_fired', 'alarm_stale', 'alarm_created', 'alarm_updated',
    'skill_added', 'skill_removed', 'skill_changed',
    'trigger', 'media_processed',
    'skill_reflex_fired',
    'tool_call'
  )),
  body                     TEXT NOT NULL DEFAULT '',
  mime_type                TEXT,
  is_edit                  INTEGER NOT NULL DEFAULT 0,
  edit_target_message_id   TEXT,
  context_stanza_id        TEXT,
  context_participant      TEXT,
  context_is_forwarded     INTEGER NOT NULL DEFAULT 0,
  context_forwarding_score INTEGER,
  context_mentioned_ids    TEXT NOT NULL DEFAULT '[]',
  is_ephemeral             INTEGER NOT NULL DEFAULT 0,
  is_view_once             INTEGER NOT NULL DEFAULT 0,
  sent_at                  TIMESTAMP NOT NULL,
  created_at               TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(sent_at)),
  CHECK (IS_ISO8601_MS(created_at)),
  UNIQUE(platform, external_message_id)
);

-- attachments (mime_type, local_path, describe_text, transcript_text)
CREATE TABLE IF NOT EXISTS media (
  id              TEXT PRIMARY KEY,
  mime_type       TEXT NOT NULL,
  size_bytes      INTEGER,
  local_path      TEXT,
  describe_text   TEXT,
  transcript_text TEXT,
  described_at    TIMESTAMP,
  transcribed_at  TIMESTAMP,
  created_at      TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at      TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (described_at IS NULL OR IS_ISO8601_MS(described_at)),
  CHECK (transcribed_at IS NULL OR IS_ISO8601_MS(transcribed_at)),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- links messages to media
CREATE TABLE IF NOT EXISTS message_media (
  id          TEXT PRIMARY KEY,
  message_id  TEXT NOT NULL UNIQUE REFERENCES message(id),
  media_id    TEXT NOT NULL REFERENCES media(id),
  created_at  TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

-- scheduled reminders (goal, recurrence, next_fire_at)
CREATE TABLE IF NOT EXISTS alarm (
  id                     TEXT PRIMARY KEY,
  origin_contact_id      TEXT REFERENCES contact(id),
  origin_conversation_id TEXT NOT NULL REFERENCES conversation(id),
  goal                   TEXT NOT NULL,
  recurrence             TEXT,
  last_occurrence_note   TEXT,
  next_fire_at           TIMESTAMP NOT NULL,
  last_fired_at          TIMESTAMP,
  cancelled_at           TIMESTAMP,
  created_at             TIMESTAMP NOT NULL,
  CHECK (IS_ISO8601_MS(next_fire_at)),
  CHECK (last_fired_at IS NULL OR IS_ISO8601_MS(last_fired_at)),
  CHECK (cancelled_at IS NULL OR IS_ISO8601_MS(cancelled_at)),
  CHECK (IS_ISO8601_MS(created_at))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_alarm_active_goal
  ON alarm (goal) WHERE cancelled_at IS NULL;

-- fired alarm instances (alarm_id, note, fired_at)
CREATE TABLE IF NOT EXISTS alarm_occurrence (
  id       TEXT PRIMARY KEY,
  alarm_id TEXT NOT NULL REFERENCES alarm(id),
  note     TEXT,
  fired_at TIMESTAMP NOT NULL,
  CHECK (IS_ISO8601_MS(fired_at))
);

-- brain/worker runs (conversation_id, task_id, model, tokens, cost, duration)
CREATE TABLE IF NOT EXISTS activation (
  id                         TEXT PRIMARY KEY,
  conversation_id            TEXT NOT NULL REFERENCES conversation(id),
  task_id                    TEXT REFERENCES task(id),
  sources                    TEXT NOT NULL DEFAULT '[]',
  model                      TEXT NOT NULL,
  reasoning_effort           TEXT CHECK(reasoning_effort IN ('none', 'minimal', 'low', 'medium', 'high', 'xhigh')),
  verbosity                  TEXT CHECK(verbosity IN ('low', 'medium', 'high')),
  input_tokens               INTEGER NOT NULL DEFAULT 0,
  output_tokens              INTEGER NOT NULL DEFAULT 0,
  total_tokens               INTEGER NOT NULL DEFAULT 0,
  cached_tokens              INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens           INTEGER NOT NULL DEFAULT 0,
  max_input_tokens_per_round INTEGER NOT NULL DEFAULT 0,
  max_total_tokens_per_round INTEGER NOT NULL DEFAULT 0,
  round_count                INTEGER NOT NULL DEFAULT 0,
  cost_usd                   REAL NOT NULL DEFAULT 0,
  tool_call_count            INTEGER NOT NULL DEFAULT 0,
  duration_ms                INTEGER NOT NULL DEFAULT 0,
  error                      TEXT NOT NULL DEFAULT '',
  instructions               TEXT NOT NULL DEFAULT '',
  tools                      TEXT NOT NULL DEFAULT '[]',
  tool_schemas               TEXT NOT NULL DEFAULT '[]',
  created_at                 TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

-- per-round LLM IO within an activation
CREATE TABLE IF NOT EXISTS activation_round (
  id                  TEXT PRIMARY KEY,
  activation_id       TEXT NOT NULL REFERENCES activation(id),
  round               INTEGER NOT NULL,
  messages            TEXT NOT NULL DEFAULT '[]',
  reasoning_summaries TEXT NOT NULL DEFAULT '[]',
  input_tokens        INTEGER NOT NULL DEFAULT 0,
  output_tokens       INTEGER NOT NULL DEFAULT 0,
  cached_tokens       INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens    INTEGER NOT NULL DEFAULT 0,
  created_at          TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

-- individual tool invocations within activations
CREATE TABLE IF NOT EXISTS tool_call (
  id                  TEXT PRIMARY KEY,
  activation_id       TEXT NOT NULL REFERENCES activation(id),
  activation_round_id TEXT REFERENCES activation_round(id),
  name                TEXT NOT NULL,
  input               TEXT NOT NULL DEFAULT '',
  output              TEXT NOT NULL DEFAULT '',
  duration_ms         INTEGER NOT NULL DEFAULT 0,
  error               INTEGER NOT NULL DEFAULT 0,
  created_at          TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

-- spawned work units (goal, plan, status, thinking level)
CREATE TABLE IF NOT EXISTS task (
  id                  TEXT PRIMARY KEY,
  conversation_id     TEXT NOT NULL REFERENCES conversation(id),
  contact_id          TEXT REFERENCES contact(id),
  activation_id       TEXT REFERENCES activation(id),
  retry_for_task_id   TEXT REFERENCES task(id),
  retry_number        INTEGER NOT NULL DEFAULT 0,
  goal                TEXT NOT NULL,
  plan                TEXT NOT NULL DEFAULT '',
  thinking            TEXT NOT NULL DEFAULT 'low' CHECK(thinking IN ('low', 'medium', 'high')),
  status              TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'running', 'completed', 'failed', 'cancelled')),
  cancellation_reason TEXT,
  created_at          TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  started_at          TIMESTAMP,
  completed_at        TIMESTAMP,
  last_report_at      TIMESTAMP,
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (started_at IS NULL OR IS_ISO8601_MS(started_at)),
  CHECK (completed_at IS NULL OR IS_ISO8601_MS(completed_at)),
  CHECK (last_report_at IS NULL OR IS_ISO8601_MS(last_report_at))
);

-- progress/completion reports from workers
CREATE TABLE IF NOT EXISTS task_report (
  id          TEXT PRIMARY KEY,
  task_id     TEXT NOT NULL REFERENCES task(id),
  status      TEXT NOT NULL DEFAULT 'running' CHECK(status IN ('running', 'completed', 'failed')),
  content     TEXT NOT NULL,
  created_at  TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

-- persistent shell sessions (command, output, exit_code, alive)
CREATE TABLE IF NOT EXISTS shell_session (
  id            TEXT PRIMARY KEY,
  activation_id TEXT NOT NULL REFERENCES activation(id),
  command       TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  output        TEXT NOT NULL DEFAULT '',
  exit_code     INTEGER,
  alive         INTEGER NOT NULL DEFAULT 1,
  created_at    TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at    TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- registered skills (name, status, content_hash)
CREATE TABLE IF NOT EXISTS skill (
  id            TEXT PRIMARY KEY,
  name          TEXT NOT NULL UNIQUE,
  status        TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'removed')),
  content_hash  TEXT,
  install_hash  TEXT,
  created_at    TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at    TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- LLM-resolved cron expressions from natural language schedule strings
CREATE TABLE IF NOT EXISTS every_to_cron (
  natural_text  TEXT PRIMARY KEY,
  cron_expr     TEXT NOT NULL,
  created_at    TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

-- skill reflex records (time series of opaque records from skill check commands)
CREATE TABLE IF NOT EXISTS skill_reflex (
  id          TEXT PRIMARY KEY,
  skill_name  TEXT NOT NULL,
  meta        TEXT NOT NULL DEFAULT '',
  created_at  TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

CREATE INDEX IF NOT EXISTS idx_skill_reflex_latest
  ON skill_reflex (skill_name, created_at DESC);

-- skill change log (added, removed, changed)
CREATE TABLE IF NOT EXISTS skill_event (
  id           TEXT PRIMARY KEY,
  name         TEXT NOT NULL,
  kind         TEXT NOT NULL CHECK(kind IN ('added', 'removed', 'changed')),
  content_hash TEXT,
  install_hash TEXT,
  created_at   TIMESTAMP NOT NULL,
  CHECK (IS_ISO8601_MS(created_at))
);

-- key-value state (genesis_completed_at, etc.)
CREATE TABLE IF NOT EXISTS setting (
  key        TEXT PRIMARY KEY,
  value      TEXT NOT NULL,
  created_at TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- prompt workbench experiments (activation_round_id, status, desired_outcome)
CREATE TABLE IF NOT EXISTS experiment (
  id                  TEXT PRIMARY KEY,
  activation_round_id TEXT NOT NULL REFERENCES activation_round(id),
  status              TEXT NOT NULL DEFAULT 'analysis' CHECK(status IN ('analysis', 'experimenting', 'complete')),
  desired_outcome     TEXT NOT NULL DEFAULT '',
  analysis            TEXT NOT NULL DEFAULT '',
  created_at          TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at          TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- experiment variants with patches and hypothesis
CREATE TABLE IF NOT EXISTS experiment_variant (
  id               TEXT PRIMARY KEY,
  experiment_id    TEXT NOT NULL REFERENCES experiment(id),
  name             TEXT NOT NULL,
  hypothesis       TEXT NOT NULL DEFAULT '',
  patches          TEXT NOT NULL DEFAULT '',
  reasoning_effort TEXT NOT NULL DEFAULT '',
  verbosity        TEXT NOT NULL DEFAULT '',
  run_count        INTEGER NOT NULL DEFAULT 0,
  desired_count    INTEGER NOT NULL DEFAULT 0,
  created_at       TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  updated_at       TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at)),
  CHECK (IS_ISO8601_MS(updated_at))
);

-- individual replay runs within a variant
CREATE TABLE IF NOT EXISTS experiment_variant_run (
  id                    TEXT PRIMARY KEY,
  experiment_variant_id TEXT NOT NULL REFERENCES experiment_variant(id),
  tool_calls            TEXT NOT NULL DEFAULT '[]',
  model_output          TEXT NOT NULL DEFAULT '',
  reasoning_summaries   TEXT NOT NULL DEFAULT '[]',
  is_desired            INTEGER,
  rationale             TEXT NOT NULL DEFAULT '',
  input_tokens          INTEGER NOT NULL DEFAULT 0,
  output_tokens         INTEGER NOT NULL DEFAULT 0,
  cached_tokens         INTEGER NOT NULL DEFAULT 0,
  reasoning_tokens      INTEGER NOT NULL DEFAULT 0,
  created_at            TIMESTAMP NOT NULL DEFAULT (NOW_ISO8601_MS()),
  CHECK (IS_ISO8601_MS(created_at))
);

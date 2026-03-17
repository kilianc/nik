package db

import (
	"database/sql"
	"time"
)

type Contact struct {
	ID            string
	Name          string
	Nicknames     []string
	Emails        []string
	WhatsappIDs   []string
	TelegramIDs   []string
	SlackIDs      []string
	PhoneNumbers  []string
	Timezone      sql.NullString
	Location      sql.NullString
	OneLiner      sql.NullString
	Notes         sql.NullString
	LastMessageAt sql.NullTime
	LastSeenAt    sql.NullTime
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

type Conversation struct {
	ID                     string
	Platform               string
	ExternalConversationID string
	Kind                   string
	Title                  sql.NullString
	Topic                  sql.NullString
	IsAnnounce             bool
	IsLocked               bool
	OwnerExternalID        sql.NullString
	ParticipantExternalIDs []string
	LastMessageAt          sql.NullTime
	LastReadAt             sql.NullTime
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type ConversationParticipant struct {
	ID          string
	ContactID   string
	DisplayName sql.NullString
	ContactName sql.NullString
	Timezone    sql.NullString
	Location    sql.NullString
	OneLiner    sql.NullString
}

type Message struct {
	ID                     string
	ConversationID         string
	ContactID              string
	Platform               string
	ExternalConversationID string
	ExternalMessageID      string
	ExternalSenderID       string
	SentAt                 time.Time
	IsFromMe               bool
	IsGroup                bool
	Kind                   string
	Body                   string
	MimeType               sql.NullString
	IsEdit                 bool
	EditTargetMessageID    sql.NullString
	ContextStanzaID        sql.NullString
	ContextParticipant     sql.NullString
	ContextIsForwarded     bool
	ContextForwardingScore sql.NullInt32
	ContextMentionedIDs    []string
	IsEphemeral            bool
	IsViewOnce             bool
	MediaID                sql.NullString
	MediaLocalPath         sql.NullString
	MediaDescribeText      sql.NullString
	MediaTranscriptText    sql.NullString
	CreatedAt              time.Time
}

type Alarm struct {
	ID                   string         `json:"id"`
	OriginContactID      sql.NullString `json:"origin_contact_id"`
	OriginConversationID sql.NullString `json:"origin_conversation_id"`
	Goal                 string         `json:"goal"`
	Recurrence           sql.NullString `json:"recurrence"`
	LastOccurrenceNote   sql.NullString `json:"last_occurrence_note"`
	NextFireAt           sql.NullTime   `json:"next_fire_at"`
	LastFiredAt          sql.NullTime   `json:"last_fired_at"`
	CreatedAt            time.Time      `json:"created_at"`
}

type AlarmOccurrence struct {
	ID         string
	AlarmID    string
	Note       sql.NullString
	FiredAt    time.Time
	Goal       string
	Recurrence sql.NullString
}

type Task struct {
	ID             string       `json:"id"`
	ConversationID string       `json:"conversation_id"`
	ContactID      string       `json:"contact_id"`
	ActivationID   string       `json:"activation_id"`
	RetryForTaskID string       `json:"retry_for_task_id"`
	RetryNumber    int          `json:"retry_number"`
	Goal           string       `json:"goal"`
	Plan           string       `json:"plan"`
	Thinking       string       `json:"thinking"`
	Status         string       `json:"status"`
	CreatedAt      time.Time    `json:"created_at"`
	StartedAt      sql.NullTime `json:"started_at"`
	CompletedAt    sql.NullTime `json:"completed_at"`
	LastReportAt   sql.NullTime `json:"last_report_at"`
}

type ActiveTask struct {
	ID             string
	Goal           string
	Status         string
	ConversationID string
	RetryNumber    int
	CreatedAt      time.Time
}

type TaskListRow struct {
	ID             string
	Goal           string
	Status         string
	ConversationID string
	CreatedAt      time.Time
	StartedAt      sql.NullTime
	CompletedAt    sql.NullTime
}

type RetryChainReport struct {
	Content   string
	CreatedAt sql.NullTime
}

type RetryChainEntry struct {
	ID          string
	RetryNumber int
	Goal        string
	Status      string
	Reports     []RetryChainReport
}

type TaskReport struct {
	ID        string    `json:"id"`
	TaskID    string    `json:"task_id"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
	Goal      string    `json:"goal"`
	Status    string    `json:"status"`
}

type SkillEvent struct {
	ID          string
	Name        string
	Kind        string
	ContentHash sql.NullString
	InstallHash sql.NullString
	CreatedAt   time.Time
}

type Skill struct {
	ID          string         `json:"id"`
	Name        string         `json:"name"`
	Status      string         `json:"status"`
	ContentHash sql.NullString `json:"content_hash"`
	InstallHash sql.NullString `json:"install_hash"`
	CreatedAt   time.Time      `json:"created_at"`
	UpdatedAt   time.Time      `json:"updated_at"`
}

type ToolCallInfo struct {
	Name       string
	Round      int
	Input      string
	Output     string
	DurationMS int64
	Error      bool
	At         time.Time
}

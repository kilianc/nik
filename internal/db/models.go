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
	ID                   string
	OriginContactID      sql.NullString
	OriginConversationID sql.NullString
	Goal                 string
	Recurrence           sql.NullString
	NextFireAt           sql.NullTime
	LastFiredAt          sql.NullTime
	CreatedAt            time.Time
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
	ID             string
	ConversationID string
	ContactID      string
	ActivationID   string
	RetryForTaskID string
	RetryNumber    int
	Goal           string
	Plan           string
	Thinking       string
	Status         string
	CreatedAt      time.Time
	StartedAt      sql.NullTime
	CompletedAt    sql.NullTime
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
	ConversationID sql.NullString
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
	ID        string
	TaskID    string
	Content   string
	CreatedAt time.Time
	Goal      string
	Status    string
}

type TaskSpawned struct {
	ID             string
	Goal           string
	RetryForTaskID sql.NullString
	RetryNumber    int
	CreatedAt      time.Time
}

type TaskCancelled struct {
	ID          string
	Goal        string
	CompletedAt time.Time
}

type ToolCallInfo struct {
	Name       string
	Input      string
	Output     string
	DurationMS int64
	Error      bool
	At         time.Time
}

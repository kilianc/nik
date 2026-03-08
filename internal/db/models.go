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

type Media struct {
	ID             string
	MimeType       string
	SizeBytes      sql.NullInt64
	LocalPath      sql.NullString
	DescribeText   sql.NullString
	TranscriptText sql.NullString
	DescribedAt    sql.NullTime
	TranscribedAt  sql.NullTime
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type Alarm struct {
	ID                   string
	OriginContactID      sql.NullString
	OriginConversationID sql.NullString
	Goal                 string
	Recurrence           sql.NullString
	Source               sql.NullString
	SourceID             sql.NullString
	NextFireAt           sql.NullTime
	LastFiredAt          sql.NullTime
	CreatedAt            time.Time
}

type AlarmOccurrence struct {
	ID            string
	AlarmID       string
	Note          sql.NullString
	NextFireAtSet sql.NullTime
	FiredAt       time.Time
}

type Task struct {
	ID             string
	Meta           map[string]string
	ActivationID   string
	CrewMemberID   string
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
	Content    string
	ReportedAt sql.NullTime
}

type RetryChainEntry struct {
	ID          string
	RetryNumber int
	Goal        string
	Status      string
	Reports     []RetryChainReport
}

type TaskAttention struct {
	TaskID         string
	Goal           string
	Status         string
	Meta           map[string]string
	RetryForTaskID string
	RetryNumber    int
	ReportIDs      string
	Reports        string
}

type ToolCallInfo struct {
	Name       string
	DurationMS int64
	Error      bool
	At         time.Time
}

type RecallMessage struct {
	Body              string
	SentAt            time.Time
	IsFromMe          bool
	SenderName        string
	ConversationTitle string
	ConversationKind  string
}

type RecallContact struct {
	Name         string
	Nicknames    []string
	Emails       []string
	PhoneNumbers []string
	WhatsappIDs  []string
	Timezone     sql.NullString
	Location     sql.NullString
	OneLiner     sql.NullString
	Notes        sql.NullString
}

type RecallAlarm struct {
	Goal        string
	Recurrence  sql.NullString
	NextFireAt  sql.NullTime
	CancelledAt sql.NullTime
	CreatedAt   time.Time
}

type RecallJournal struct {
	Date    string
	Content string
}

type RecallDream struct {
	Date    string
	Pass    int
	Content string
}

type RecallBriefing struct {
	Date    string
	Content string
}

package queries

import _ "embed"

// contact queries

//go:embed contact_upsert_whatsapp_insert.sql
var ContactUpsertWhatsAppInsert string

//go:embed contact_upsert_whatsapp_update.sql
var ContactUpsertWhatsAppUpdate string

//go:embed contact_upsert_self_whatsapp.sql
var ContactUpsertSelfWhatsApp string

//go:embed get_contact.sql
var GetContact string

//go:embed update_contact_field.sql
var UpdateContactField string

//go:embed contact_add_whatsapp_id.sql
var ContactAddWhatsAppID string

// conversation queries

//go:embed conversation_upsert.sql
var ConversationUpsert string

//go:embed conversation_get.sql
var ConversationGet string

//go:embed conversation_mark_read.sql
var ConversationMarkRead string

//go:embed conversation_mark_all_read.sql
var ConversationMarkAllRead string

//go:embed conversation_upsert_participant.sql
var ConversationUpsertParticipant string

//go:embed conversation_get_participants.sql
var ConversationGetParticipants string

// message queries

//go:embed message_insert.sql
var MessageInsert string

//go:embed message_get.sql
var MessageGet string

//go:embed message_list.sql
var MessageList string

//go:embed message_update_body.sql
var MessageUpdateBody string

// media queries

//go:embed media_upsert.sql
var MediaUpsert string

//go:embed media_update_description.sql
var MediaUpdateDescription string

//go:embed message_media_upsert.sql
var MessageMediaUpsert string

// alarm queries

//go:embed create_alarm.sql
var CreateAlarm string

//go:embed due_alarms.sql
var DueAlarms string

//go:embed alarm_claim.sql
var AlarmClaim string

//go:embed alarm_update.sql
var AlarmUpdate string

//go:embed alarm_cancel.sql
var AlarmCancel string

//go:embed alarm_list_created.sql
var AlarmListCreated string

//go:embed alarm_get.sql
var AlarmGet string

// alarm occurrence queries

//go:embed alarm_occurrence_insert.sql
var AlarmOccurrenceInsert string

//go:embed alarm_occurrence_list.sql
var AlarmOccurrenceList string

//go:embed alarm_occurrence_update_note.sql
var AlarmOccurrenceUpdateNote string

// activation queries

//go:embed activation_insert.sql
var ActivationInsert string

//go:embed activation_update_stats.sql
var ActivationUpdateStats string

//go:embed activation_detail_insert.sql
var ActivationDetailInsert string

// tool call queries

//go:embed tool_call_insert_one.sql
var ToolCallInsertOne string

// task queries

//go:embed task_insert.sql
var TaskInsert string

//go:embed task_get.sql
var TaskGet string

//go:embed task_update_status.sql
var TaskUpdateStatus string

//go:embed task_start.sql
var TaskStart string

//go:embed task_stale.sql
var TaskStale string

//go:embed task_active_retries.sql
var TaskActiveRetries string

//go:embed task_retry_chain.sql
var TaskRetryChain string

//go:embed task_list.sql
var TaskList string

//go:embed task_list_spawned.sql
var TaskListSpawned string

//go:embed task_list_cancelled.sql
var TaskListCancelled string

// task assessment queries

//go:embed task_assessment_insert.sql
var TaskAssessmentInsert string

//go:embed task_assessment_tool_calls.sql
var TaskAssessmentToolCalls string

//go:embed task_report_by_task.sql
var TaskReportByTask string

// task report queries

//go:embed task_report_insert.sql
var TaskReportInsert string

//go:embed task_report_list.sql
var TaskReportList string

//go:embed task_report_last_status.sql
var TaskReportLastStatus string

//go:embed task_tool_calls.sql
var TaskToolCalls string

// shell output queries

//go:embed shell_output_upsert.sql
var ShellOutputUpsert string

//go:embed shell_output_update.sql
var ShellOutputUpdate string

//go:embed shell_output_alive.sql
var ShellOutputAlive string

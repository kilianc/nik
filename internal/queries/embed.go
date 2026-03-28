package queries

import _ "embed"

// contact queries

//go:embed contact_upsert_whatsapp_insert.sql
var ContactUpsertWhatsAppInsert string

//go:embed contact_upsert_whatsapp_update.sql
var ContactUpsertWhatsAppUpdate string

//go:embed contact_upsert_self_whatsapp.sql
var ContactUpsertSelfWhatsApp string

//go:embed contact_get.sql
var ContactGet string

//go:embed contact_update.sql
var ContactUpdate string

//go:embed contact_add_whatsapp_id.sql
var ContactAddWhatsAppID string

//go:embed contact_system_ensure.sql
var ContactSystemEnsure string

// conversation queries

//go:embed conversation_upsert.sql
var ConversationUpsert string

//go:embed conversation_get.sql
var ConversationGet string

//go:embed conversation_mark_read.sql
var ConversationMarkRead string

//go:embed conversation_mark_all_read.sql
var ConversationMarkAllRead string

//go:embed conversation_participant_upsert.sql
var ConversationParticipantUpsert string

//go:embed conversation_participant_list.sql
var ConversationParticipantList string

// message queries

//go:embed message_insert.sql
var MessageInsert string

//go:embed message_get.sql
var MessageGet string

//go:embed message_list.sql
var MessageList string

//go:embed message_update.sql
var MessageUpdate string

// media queries

//go:embed media_insert.sql
var MediaInsert string

//go:embed media_update.sql
var MediaUpdate string

//go:embed media_resolve_by_path.sql
var MediaResolveByPath string

//go:embed message_media_upsert.sql
var MessageMediaUpsert string

// alarm queries

//go:embed alarm_insert.sql
var AlarmInsert string

//go:embed alarm_due.sql
var AlarmDue string

//go:embed alarm_update.sql
var AlarmUpdate string

//go:embed alarm_get.sql
var AlarmGet string

//go:embed alarm_stale_recurring.sql
var AlarmStaleRecurring string

// alarm occurrence queries

//go:embed alarm_occurrence_insert.sql
var AlarmOccurrenceInsert string

//go:embed alarm_occurrence_update.sql
var AlarmOccurrenceUpdate string

// skill queries

//go:embed skill_list.sql
var SkillList string

//go:embed skill_upsert.sql
var SkillUpsert string

// every_to_cron queries

//go:embed every_to_cron_get.sql
var EveryToCronGet string

//go:embed every_to_cron_insert.sql
var EveryToCronInsert string

// skill reflex queries

//go:embed skill_reflex_get.sql
var SkillReflexGet string

//go:embed skill_reflex_insert.sql
var SkillReflexInsert string

// skill event queries

//go:embed skill_event_insert.sql
var SkillEventInsert string

//go:embed skill_event_latest_per_name.sql
var SkillEventLatestPerName string

//go:embed skill_event_list.sql
var SkillEventList string

// activation queries

//go:embed activation_insert.sql
var ActivationInsert string

//go:embed activation_get.sql
var ActivationGet string

//go:embed activation_update.sql
var ActivationUpdate string

// activation round queries

//go:embed activation_round_insert.sql
var ActivationRoundInsert string

//go:embed activation_round_get.sql
var ActivationRoundGet string

//go:embed activation_round_list.sql
var ActivationRoundList string

// tool call queries

//go:embed tool_call_insert.sql
var ToolCallInsert string

//go:embed tool_call_list.sql
var ToolCallList string

// task queries

//go:embed task_insert.sql
var TaskInsert string

//go:embed task_get.sql
var TaskGet string

//go:embed task_update.sql
var TaskUpdate string

//go:embed task_stale.sql
var TaskStale string

//go:embed task_active_retries.sql
var TaskActiveRetries string

//go:embed task_retry_chain.sql
var TaskRetryChain string

//go:embed task_list.sql
var TaskList string

// task assessment queries

//go:embed task_assessment_insert.sql
var TaskAssessmentInsert string

//go:embed task_assessment_tool_calls.sql
var TaskAssessmentToolCalls string

// task report queries

//go:embed task_report_insert.sql
var TaskReportInsert string

//go:embed task_report_list.sql
var TaskReportList string

//go:embed task_report_last_status.sql
var TaskReportLastStatus string

// shell session queries

//go:embed shell_session_upsert.sql
var ShellSessionUpsert string

//go:embed shell_session_update.sql
var ShellSessionUpdate string

//go:embed shell_session_alive.sql
var ShellSessionAlive string

// experiment queries

//go:embed experiment_insert.sql
var ExperimentInsert string

//go:embed experiment_get.sql
var ExperimentGet string

//go:embed experiment_update.sql
var ExperimentUpdate string

// experiment variant queries

//go:embed experiment_variant_insert.sql
var ExperimentVariantInsert string

//go:embed experiment_variant_get.sql
var ExperimentVariantGet string

//go:embed experiment_variant_list.sql
var ExperimentVariantList string

//go:embed experiment_variant_update.sql
var ExperimentVariantUpdate string

//go:embed experiment_variant_refresh_counts.sql
var ExperimentVariantRefreshCounts string

// experiment variant run queries

//go:embed experiment_variant_run_get.sql
var ExperimentVariantRunGet string

//go:embed experiment_variant_run_insert.sql
var ExperimentVariantRunInsert string

//go:embed experiment_variant_run_list.sql
var ExperimentVariantRunList string

//go:embed experiment_variant_run_save_result.sql
var ExperimentVariantRunSaveResult string

//go:embed experiment_variant_run_update.sql
var ExperimentVariantRunUpdate string

// prune queries

//go:embed prune.sql
var Prune string

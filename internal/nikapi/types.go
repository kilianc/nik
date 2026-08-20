package nikapi

import "github.com/kciuffolo/nik/internal/api"

// The wire types are the server's, re-exported rather than re-declared.
// nikd and nikctl ship together from one build, so unlike the gateway wire —
// where a released nik cannot be redeployed to match the platform — there is
// no version skew to model here. A second copy would only be a second thing
// to keep in step.

type (
	Version      = apiVersion
	Health       = api.Health
	Subsystem    = api.Subsystem
	Conversation = api.Conversation
	Message      = api.Message
	Author       = api.Author

	OnboardingState = api.OnboardingState
)

// Message authors, re-exported so a renderer switches on a constant.
const (
	AuthorNik     = api.AuthorNik
	AuthorSystem  = api.AuthorSystem
	AuthorOwner   = api.AuthorOwner
	AuthorContact = api.AuthorContact
)

// ConfigField is one field to change; see Client.SetConfig.
type ConfigField = api.ConfigField

// LocalConversationID is the conversation the TUI and the console render.
const LocalConversationID = api.LocalConversationID

// apiVersion mirrors the server's unexported response shape. It is spelled
// out here because the server's is deliberately private: /v1/version is the
// one endpoint whose body a future client may have to read from an older
// daemon, so the field names are a contract rather than a struct.
type apiVersion struct {
	Version    string `json:"version"`
	Number     string `json:"number"`
	Commit     string `json:"commit"`
	APIVersion int    `json:"api_version"`
}

package contacts

import (
	"context"
	"database/sql"
	"encoding/json"

	"github.com/kciuffolo/nik/internal/db"
	"github.com/kciuffolo/nik/internal/llm"
)

var updateContactToolDef = llm.ToolDef{
	Name:        "update_contact",
	Description: "Update a single field on a contact. Use to persist name, timezone, location, notes, one_liner, nicknames, emails, or phone_numbers for a person.",
	Parameters: map[string]any{
		"type": "object",
		"properties": map[string]any{
			"contact_id": map[string]any{
				"type":        "string",
				"description": "The contact UUID.",
			},
			"field": map[string]any{
				"type":        "string",
				"enum":        []any{"name", "notes", "one_liner", "nicknames", "emails", "phone_numbers", "timezone", "location"},
				"description": "Which contact field to update.",
			},
			"value": map[string]any{
				"type":        "string",
				"description": "The new value. For nicknames, emails, and phone_numbers, pass a JSON array of strings.",
			},
		},
		"required":             []string{"contact_id", "field", "value"},
		"additionalProperties": false,
	},
}

func BuildTools(conn *sql.DB) []llm.Tool {
	return []llm.Tool{
		{
			Def:     updateContactToolDef,
			Handler: updateContactHandler(conn),
		},
	}
}

func updateContactHandler(conn *sql.DB) llm.ToolExecutor {
	return func(ctx context.Context, call llm.ToolCall) (string, error) {
		var args struct {
			ContactID string `json:"contact_id"`
			Field     string `json:"field"`
			Value     string `json:"value"`
		}

		err := json.Unmarshal([]byte(call.Arguments), &args)
		if err != nil {
			return llm.ToolError(err), nil
		}

		var arrayValue []string
		switch args.Field {
		case "nicknames", "emails", "phone_numbers":
			err = json.Unmarshal([]byte(args.Value), &arrayValue)
			if err != nil {
				return llm.ToolErrorf("parse %s: %v", args.Field, err), nil
			}
		}

		err = db.ContactUpdateField(ctx, conn, db.ContactUpdateFieldParams{
			ID:         args.ContactID,
			Field:      args.Field,
			Value:      args.Value,
			ArrayValue: arrayValue,
		})
		if err != nil {
			return llm.ToolError(err), nil
		}

		return `{"ok":true}`, nil
	}
}

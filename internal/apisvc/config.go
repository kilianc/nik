package apisvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/db"
)

// Config serves the same operations the `config` brain tool does, and
// deliberately through the same code: nik changing its own model and a person
// changing it from a console are the same act, and two implementations would
// eventually disagree about which fields are writable.
type Config struct {
	cfg  *config.Config
	conn *sql.DB
}

func NewConfig(cfg *config.Config, conn *sql.DB) *Config {
	return &Config{cfg: cfg, conn: conn}
}

func (c *Config) Get(ctx context.Context) (map[string]any, error) {
	return config.Snapshot(c.cfg), nil
}

func (c *Config) Set(ctx context.Context, field, value string) error {
	err := config.SetField(c.cfg, field, value)
	if err == nil {
		c.propagate(ctx, field, value)
	}

	// A typo'd field name is the client's mistake and gets a 400; a config
	// file that will not write is nikd's and gets a 500. The config package
	// distinguishes them so this does not have to read error strings.
	switch {
	case errors.Is(err, config.ErrUnknownField),
		errors.Is(err, config.ErrReadOnlyField),
		errors.Is(err, config.ErrInvalidValue):
		return fmt.Errorf("%w: %s", api.ErrInvalidField, err)
	}

	return err
}

// propagate keeps the things that shadow a config value in step with it.
//
// Timezone and location live on nik's and the owner's contact cards as well as
// in config.yaml, because that is where the brain reads them from. The setup
// wizard used to write both halves itself; doing it here means anything that
// changes the config — the wizard, nik's own `config` tool, a console — gets
// the second half for free rather than remembering to.
func (c *Config) propagate(ctx context.Context, field, value string) {
	if c.conn == nil {
		return
	}
	if field != "timezone" && field != "location" {
		return
	}

	for _, contactID := range []string{db.OwnerContactID, db.NikContactID} {
		err := db.ContactUpdate(ctx, c.conn, db.ContactUpdateParams{
			ID: contactID, Field: field, Value: value,
		})
		if err != nil {
			// Not fatal: config.yaml is the source of truth and the contact
			// card is a copy of it. Worth a line, not a failed request.
			slog.Warn("propagate config to contact",
				"pkg", "apisvc", "field", field, "contact", contactID, "error", err)
		}
	}
}

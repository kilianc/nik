package apisvc

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/config"
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

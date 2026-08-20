package apisvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/kciuffolo/nik/internal/api"
	"github.com/kciuffolo/nik/internal/config"
	"github.com/kciuffolo/nik/internal/gateway"
	"github.com/kciuffolo/nik/internal/secrets"
)

// Gateway links this nik to an account.
//
// It does what `nik connect` has always done — probe, rotate, store, write
// the url — except inside the running daemon. That is the difference that
// matters: the token arrives at the process that will use it, so an
// unconfigured nik can be linked while it is running rather than being
// configured from outside and restarted into working.
type Gateway struct {
	home  string
	store *secrets.Store

	// cfg is nil before a config exists, which is exactly the case this
	// endpoint is here to fix: a fresh install has no config.yaml and the
	// connect that creates one is the first thing that happens to it.
	cfg *config.Config

	// onConnect lets nikd converge — reload and start what could not start
	// without a gateway. Nil is fine; the daemon restarts on its own schedule
	// in that case.
	onConnect func()
}

func NewGateway(home string, store *secrets.Store, cfg *config.Config, onConnect func()) *Gateway {
	return &Gateway{home: home, store: store, cfg: cfg, onConnect: onConnect}
}

func (g *Gateway) Connect(ctx context.Context, url, token string) error {
	cfg := g.cfg
	if cfg == nil {
		// No config yet. Read one if it appeared since boot, otherwise start
		// from defaults — the same thing `nik connect` does on a fresh home.
		existing, err := config.Read(g.home)
		if err != nil {
			existing = config.Default(g.home)
		}
		cfg = existing
	}

	switch {
	case url != "":
		cfg.Gateway.URL = url
	case cfg.Gateway.URL == "":
		cfg.Gateway.URL = gateway.DefaultURL
	}

	// The probe rotates: what gets stored is the token the gateway handed
	// back, never the one that was sent. An install code that reached us over
	// this socket is dead the moment this returns.
	err := gateway.ProbeWithStore(ctx, cfg.Gateway.URL, token, g.store)
	if errors.Is(err, gateway.ErrAuthRejected) {
		return api.ErrAuthRejected
	}
	if err != nil {
		return fmt.Errorf("connect to %s: %w", cfg.Gateway.URL, err)
	}

	cfg.Normalize()

	err = cfg.Save(cfg.ConfigPath())
	if err != nil {
		return fmt.Errorf("write config: %w", err)
	}

	slog.Info("gateway connected", "pkg", "apisvc", "url", cfg.Gateway.URL)

	if g.onConnect != nil {
		g.onConnect()
	}

	return nil
}

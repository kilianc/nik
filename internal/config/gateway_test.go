package config

import "testing"

func TestGatewayURLSurvivesSave(t *testing.T) {
	dir := t.TempDir()
	cfg := Default(dir)
	cfg.Gateway.URL = "wss://gw.example/v1/agent"
	cfg.Normalize()

	if err := cfg.Save(cfg.ConfigPath()); err != nil {
		t.Fatalf("save: %v", err)
	}

	back, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if back.Gateway.URL != "wss://gw.example/v1/agent" {
		t.Fatalf("gateway.url after save+read = %q, want it preserved", back.Gateway.URL)
	}
}

// gateway.api is remote access to somebody's computer. A save that dropped it
// would silently turn it back on or off depending on which way the default
// fell — here, off, which is the safe direction and still not the one the
// owner chose.
func TestGatewayAPIFlagSurvivesSave(t *testing.T) {
	dir := t.TempDir()
	cfg := Default(dir)
	cfg.Gateway.URL = "wss://gw.example/v1/agent"
	cfg.Gateway.API = true
	cfg.Normalize()

	if err := cfg.Save(cfg.ConfigPath()); err != nil {
		t.Fatalf("save: %v", err)
	}

	back, err := Read(dir)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !back.Gateway.API {
		t.Fatal("gateway.api was dropped by save — remote access would silently turn off")
	}
}

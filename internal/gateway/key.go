package gateway

import (
	"encoding/hex"
	"fmt"
	"strings"
)

// the nik key lives in nik's secret store next to the api keys, hex-encoded.
// losing it only loses queued undelivered messages sealed to the old key: the
// gateway replaces the registered public key on the next hello.

const keySecretName = "gateway_nik_key"

type secretStore interface {
	Get(name string) (string, error)
	Set(name, value string) error
}

func loadOrCreateKey(store secretStore) (*[keySize]byte, error) {
	raw, err := store.Get(keySecretName)
	if err == nil {
		decoded, err := hex.DecodeString(strings.TrimSpace(raw))
		if err != nil || len(decoded) != keySize {
			return nil, fmt.Errorf("decode %s: corrupt key", keySecretName)
		}

		var priv [keySize]byte
		copy(priv[:], decoded)

		return &priv, nil
	}

	_, priv, err := generateKey()
	if err != nil {
		return nil, fmt.Errorf("generate nik key: %w", err)
	}

	err = store.Set(keySecretName, hex.EncodeToString(priv[:]))
	if err != nil {
		return nil, fmt.Errorf("store nik key: %w", err)
	}

	return priv, nil
}

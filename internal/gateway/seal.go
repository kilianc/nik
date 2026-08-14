package gateway

import (
	"crypto/rand"
	"errors"

	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// NaCl anonymous sealed boxes (X25519 + XSalsa20-Poly1305). the gateway seals
// everything it stores to this agent's public key, so nothing it keeps can be
// read without the private key that never leaves this machine.
//
// the platform side has the same three functions; only Seal is unused here,
// and it stays so a key can be exercised in both directions in tests.

const keySize = 32

func generateKey() (pub, priv *[keySize]byte, err error) {
	return box.GenerateKey(rand.Reader)
}

func publicKeyOf(priv *[keySize]byte) *[keySize]byte {
	pub, err := curve25519.X25519(priv[:], curve25519.Basepoint)
	if err != nil {
		panic(err) // only fails on a low-order point; ours are random
	}

	var out [keySize]byte
	copy(out[:], pub)

	return &out
}

func sealTo(msg []byte, recipientPub *[keySize]byte) ([]byte, error) {
	return box.SealAnonymous(nil, msg, recipientPub, rand.Reader)
}

func openSealed(sealed []byte, pub, priv *[keySize]byte) ([]byte, error) {
	out, ok := box.OpenAnonymous(nil, sealed, pub, priv)
	if !ok {
		return nil, errors.New("open sealed box: cannot decrypt")
	}

	return out, nil
}

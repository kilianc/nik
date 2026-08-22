package gateway

import (
	"strings"
	"testing"
)

func TestSealRoundTrip(t *testing.T) {
	pub, priv, err := generateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	want := []byte("what's for dinner?")

	sealed, err := sealTo(want, pub)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if strings.Contains(string(sealed), "dinner") {
		t.Fatal("plaintext survives in the sealed box")
	}

	got, err := openSealed(sealed, pub, priv)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if string(got) != string(want) {
		t.Errorf("opened %q, want %q", got, want)
	}
}

// a reinstalled nik generates a new key; the gateway's backlog is still
// sealed to the old one and must fail closed rather than yield garbage
func TestOpenWithWrongKey(t *testing.T) {
	pub, _, err := generateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	otherPub, otherPriv, err := generateKey()
	if err != nil {
		t.Fatalf("generate other key: %v", err)
	}

	sealed, err := sealTo([]byte("secret"), pub)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	_, err = openSealed(sealed, otherPub, otherPriv)
	if err == nil {
		t.Fatal("opened a box sealed to another key")
	}
	if !strings.Contains(err.Error(), "cannot decrypt") {
		t.Errorf("err = %v", err)
	}
}

func TestPublicKeyOfMatchesGenerated(t *testing.T) {
	pub, priv, err := generateKey()
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	derived := publicKeyOf(priv)
	if *derived != *pub {
		t.Error("derived public key differs from the generated one")
	}
}

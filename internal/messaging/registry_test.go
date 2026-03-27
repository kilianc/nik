package messaging

import "testing"

func TestRegistry(t *testing.T) {
	t.Run("register and get", func(t *testing.T) {
		r := NewRegistry()
		p := &mockPlatform{platform: "whatsapp"}

		err := r.Register(p)
		if err != nil {
			t.Fatalf("register platform: %v", err)
		}

		got, err := r.Get("whatsapp")
		if err != nil {
			t.Fatalf("get platform: %v", err)
		}
		if got != p {
			t.Fatalf("expected to get the same platform instance")
		}
	})

	t.Run("rejects duplicate", func(t *testing.T) {
		r := NewRegistry()
		p := &mockPlatform{platform: "whatsapp"}

		err := r.Register(p)
		if err != nil {
			t.Fatalf("register first platform: %v", err)
		}

		err = r.Register(p)
		if err == nil {
			t.Fatalf("expected duplicate registration error")
		}
	})
}

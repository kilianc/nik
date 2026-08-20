package shell

import "testing"

// The other half of the same assertion, from this side: these constants are
// copies of internal/api's, and a copy that drifts means the sandbox either
// cannot reach nikd or can reach the socket it must not.
func TestSocketConstantsMatchTheAPI(t *testing.T) {
	if apiSocketDir != "run" {
		t.Fatalf("apiSocketDir = %q, want api.SocketDir's value", apiSocketDir)
	}
	if containerSocketPath != "/run/nik.sock" {
		t.Fatalf("containerSocketPath = %q, want api.ContainerSocketPath's value", containerSocketPath)
	}
}

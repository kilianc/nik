package api

import (
	"context"
	"net/http"
	"strings"
)

// Scope is who is asking, and it comes from which socket the request arrived
// on rather than from anything in the request. There is no token to steal, no
// header to forge, and nothing a caller can say about itself that changes the
// answer.
type Scope string

const (
	// ScopeOwner is the person running nikd. Everything.
	ScopeOwner Scope = "owner"

	// ScopeSandbox is a skill running inside the shell container. It gets
	// named secrets and nothing else.
	//
	// This exists because the sandbox has, until now, had the run of
	// NIK_HOME: the workspace is mounted read-write, and the encrypted secret
	// store and the key that opens it sit inside it. A skill did not need
	// permission to read a credential, only a file path. Moving secrets
	// behind a socket is what makes "which secret, and may you have it" a
	// question somebody can answer.
	ScopeSandbox Scope = "sandbox"
)

type scopeKey struct{}

func withScope(scope Scope, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !scopeAllows(scope, r.Method, r.URL.Path) {
			writeError(w, http.StatusForbidden, "not available on this socket")
			return
		}

		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), scopeKey{}, scope)))
	})
}

// scopeOf reports who is asking. Absent a scope — which should not happen,
// since every listener sets one — the safe answer is the narrow one.
func scopeOf(r *http.Request) Scope {
	scope, ok := r.Context().Value(scopeKey{}).(Scope)
	if !ok {
		return ScopeSandbox
	}

	return scope
}

// scopeAllows is an allowlist, not a deny list. A route added later is
// unreachable from the sandbox until somebody decides otherwise, which is the
// failure direction to prefer.
func scopeAllows(scope Scope, method, path string) bool {
	if scope == ScopeOwner {
		return true
	}

	switch {
	case path == "/v1/version", path == "/v1/health":
		return true

	case strings.HasPrefix(path, "/v1/secrets/"):
		// Read and write named secrets; never list them. A skill that needs a
		// credential knows which one it needs, and enumerating the store is
		// how you find out what else is in there.
		return method == http.MethodGet || method == http.MethodPut

	default:
		return false
	}
}

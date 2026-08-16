// Package version carries build identity, stamped via ldflags:
//
//	-X github.com/kciuffolo/nik/internal/version.Number=<VERSION file>
//	-X github.com/kciuffolo/nik/internal/version.SHA=<git sha>
package version

// Number is the release this binary was cut as, SHA the commit it was cut
// from. a build that came out of the release workflow carries both; anything
// built by hand carries neither and says so, because "which nik is this" is
// the first question every bug report has to answer.
var (
	Number = "dev"
	SHA    = "dev"
)

// String is the identity to show a human: release and commit together, since
// on an unreleased build the release alone names the wrong code.
func String() string {
	if Number == "dev" {
		return "dev"
	}
	return Number + " (" + SHA + ")"
}

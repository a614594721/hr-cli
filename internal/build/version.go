// Package build exposes build-time metadata injected via -ldflags -X.
//
// Example:
//
//	go build -ldflags "-X hr-cli/internal/build.Version=1.0.0-rc.1 \
//	                   -X hr-cli/internal/build.Date=2026-06-14 \
//	                   -X hr-cli/internal/build.Commit=$(git rev-parse --short HEAD)"
//
// Default values are kept so unflagged `go build` and `go run` still work.
package build

var (
	Version = "0.0.0-dev"
	Date    = "unknown"
	Commit  = "unknown"
)

// String returns a human-readable version string.
func String() string {
	return Version
}

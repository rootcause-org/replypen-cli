// Command rp is the replypen CLI: a scriptable static-token client over replypen's /api/v1/debug/* API
// plus a few fully-local utilities. main stays trivial — it delegates to cli.Execute, which owns flag
// parsing, the client, and error rendering, and returns the process exit code.
package main

import (
	"os"

	"github.com/rootcause-org/replypen-cli/internal/cli"
)

// version is injected at build time via -X main.version (GoReleaser/ldflags); "dev" for local builds.
var version = "dev"

func main() {
	os.Exit(cli.Execute(version))
}

// Command gh-tmpl is a GitHub CLI extension that writes and checks issue and
// pull request bodies against a repository's templates.
package main

import (
	"context"
	"os"
	"os/signal"
	"runtime/debug"

	"github.com/kennyg/gh-tmpl/internal/cli"
)

// version is set at build time by GoReleaser.
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := cli.Run(ctx, cli.DefaultEnv(buildVersion()), os.Args[1:])
	stop()
	os.Exit(code)
}

func buildVersion() string {
	if version != "dev" {
		return version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" && info.Main.Version != "(devel)" {
		return info.Main.Version
	}
	return version
}

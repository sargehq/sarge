package main

import (
	"os"

	"github.com/sargehq/sarge/cmd"
)

// These variables are set at build time via ldflags by GoReleaser.
// See .goreleaser.yml for the ldflags configuration.
var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	cmd.SetVersionInfo(version, commit, date)
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// Command zot is a lightweight terminal coding agent.
package main

import (
	"fmt"
	"os"
	"runtime/debug"
	"strings"

	"github.com/patriceckhart/zot/packages/agent"
)

// Injected at build time via -ldflags "-X main.version=... -X main.commit=... -X main.date=...".
// See .goreleaser.yaml for the release build and the Makefile for
// local builds. Defaults make `zot --version` print something sensible
// when built without ldflags.
var (
	// 0.0.0 is the pre-release placeholder for local / untagged
	// builds. The first published GitHub release will be tagged
	// v0.0.1; everything before that ships as 0.0.0 from source.
	version = "0.0.0"
	commit  = ""
	date    = ""
)

func main() {
	v := resolvedVersion(version)
	if commit != "" {
		short := commit
		if len(short) > 7 {
			short = short[:7]
		}
		v = v + " (" + short
		if date != "" {
			v = v + ", " + date
		}
		v = v + ")"
	}
	if err := agent.Run(os.Args[1:], v); err != nil {
		fmt.Fprintln(os.Stderr, "zot:", err)
		os.Exit(1)
	}
}

// resolvedVersion falls back to the module version embedded by Go when zot is
// installed with "go install ...@version". Release archives still use the
// version injected by GoReleaser, and local source builds remain 0.0.0.
func resolvedVersion(linkedVersion string) string {
	info, _ := debug.ReadBuildInfo()
	return resolvedVersionFromBuildInfo(linkedVersion, info)
}

func resolvedVersionFromBuildInfo(linkedVersion string, info *debug.BuildInfo) string {
	if linkedVersion != "" && linkedVersion != "0.0.0" {
		return linkedVersion
	}
	if info == nil || info.Main.Version == "" || info.Main.Version == "(devel)" {
		return linkedVersion
	}
	return strings.TrimPrefix(info.Main.Version, "v")
}

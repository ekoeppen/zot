package main

import (
	"runtime/debug"
	"testing"
)

func TestResolvedVersionFromBuildInfo(t *testing.T) {
	tests := []struct {
		name          string
		linkedVersion string
		moduleVersion string
		want          string
	}{
		{
			name:          "go install release",
			linkedVersion: "0.0.0",
			moduleVersion: "v0.2.94",
			want:          "0.2.94",
		},
		{
			name:          "release linker version wins",
			linkedVersion: "0.2.95",
			moduleVersion: "v0.2.94",
			want:          "0.2.95",
		},
		{
			name:          "local build",
			linkedVersion: "0.0.0",
			moduleVersion: "(devel)",
			want:          "0.0.0",
		},
		{
			name:          "missing build info",
			linkedVersion: "0.0.0",
			want:          "0.0.0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var info *debug.BuildInfo
			if tt.moduleVersion != "" {
				info = &debug.BuildInfo{Main: debug.Module{Version: tt.moduleVersion}}
			}
			if got := resolvedVersionFromBuildInfo(tt.linkedVersion, info); got != tt.want {
				t.Fatalf("resolvedVersionFromBuildInfo(%q, module %q) = %q, want %q", tt.linkedVersion, tt.moduleVersion, got, tt.want)
			}
		})
	}
}

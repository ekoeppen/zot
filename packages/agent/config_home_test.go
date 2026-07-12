package agent

import (
	"path/filepath"
	"testing"
)

func TestZotHomePrefersExplicitHomeOverXDGStateHome(t *testing.T) {
	t.Setenv("ZOT_HOME", filepath.Join("explicit", "zot"))
	t.Setenv("XDG_STATE_HOME", filepath.Join("xdg", "state"))

	if got, want := ZotHome(), filepath.Join("explicit", "zot"); got != want {
		t.Fatalf("ZotHome() = %q, want %q", got, want)
	}
}

func TestZotHomeUsesXDGStateHome(t *testing.T) {
	t.Setenv("ZOT_HOME", "")
	t.Setenv("XDG_STATE_HOME", filepath.Join("xdg", "state"))

	if got, want := ZotHome(), filepath.Join("xdg", "state", "zot"); got != want {
		t.Fatalf("ZotHome() = %q, want %q", got, want)
	}
}

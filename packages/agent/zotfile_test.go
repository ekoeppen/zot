package agent

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/tools"
)

func writeTestZotfile(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "AGENT.md"), []byte("Be useful."), 0o600); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestLoadZotfileRejectsUnenforcedPermissions(t *testing.T) {
	for _, field := range []string{
		`"net":{"allow":["example.com"]}`,
		`"env":{"read":["HOME"]}`,
	} {
		dir := writeTestZotfile(t, `{"zotfile":1,"name":"test","permissions":{`+field+`}}`)
		if _, _, err := loadZotfile(dir); err == nil {
			t.Fatalf("manifest with %s was accepted", field)
		}
	}
}

func TestLoadZotfileRejectsUnsafeOrCollidingNames(t *testing.T) {
	for _, name := range []string{"...", "Name", "two words", "a/b"} {
		dir := writeTestZotfile(t, `{"zotfile":1,"name":"`+name+`"}`)
		if _, _, err := loadZotfile(dir); err == nil {
			t.Fatalf("unsafe manifest name %q was accepted", name)
		}
	}
}

func TestLoadZotfileRejectsBundledExecutableExtension(t *testing.T) {
	dir := writeTestZotfile(t, `{"zotfile":1,"name":"test"}`)
	ext := filepath.Join(dir, "extensions", "bad")
	if err := os.MkdirAll(ext, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ext, "extension.json"), []byte(`{"name":"bad"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadZotfile(dir); err == nil || !strings.Contains(err.Error(), "cannot yet be confined") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCheckZotfileMinVersion(t *testing.T) {
	var zf zotfileLoaded
	zf.Manifest.Runtime.MinZot = "0.3.0"
	if err := checkZotfileRequirements(zf, "0.2.75"); err == nil {
		t.Fatal("old zot version accepted")
	}
	if err := checkZotfileRequirements(zf, "0.3.0"); err != nil {
		t.Fatalf("minimum version rejected: %v", err)
	}
}

func TestApplyZotfileModelRequirementsRejectsUnsupportedFields(t *testing.T) {
	var m ZotfileManifest
	m.Model.MinTier = "frontier"
	if err := applyZotfileModelRequirements(&Args{}, m); err == nil {
		t.Fatal("unsupported min_tier was ignored")
	}
	m.Model.MinTier = ""
	m.Model.Requires = []string{"audio"}
	if err := applyZotfileModelRequirements(&Args{}, m); err == nil {
		t.Fatal("unsupported capability was ignored")
	}
}

func TestUntarRejectsTraversalAndOversizedEntry(t *testing.T) {
	makeTar := func(name string, size int64) []byte {
		var buf bytes.Buffer
		tw := tar.NewWriter(&buf)
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: size}); err != nil {
			t.Fatal(err)
		}
		_ = tw.Close()
		return buf.Bytes()
	}
	if err := untar(bytes.NewReader(makeTar("../escape", 0)), t.TempDir()); err == nil {
		t.Fatal("path traversal accepted")
	}
	if err := untar(bytes.NewReader(makeTar("large", maxZotfileEntrySize+1)), t.TempDir()); err == nil {
		t.Fatal("oversized entry accepted")
	}
}

func TestPermissionSummaryShowsDeniedScopes(t *testing.T) {
	got := permissionSummary(tools.PermissionSet{})
	if !strings.Contains(got, "fs read: none") || !strings.Contains(got, "fs write: none") {
		t.Fatalf("summary did not show denied scopes:\n%s", got)
	}
}

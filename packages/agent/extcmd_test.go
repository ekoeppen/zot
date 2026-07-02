package agent

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/agent/extensions"
)

// TestExtInstallDotSource verifies that `zot ext install .` derives the
// extension name from the resolved directory name rather than collapsing
// to the extensions/ parent directory (the false "already exists" bug).
func TestExtInstallDotSource(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	// Pre-create extensions/ to mimic a normal first run.
	if err := os.MkdirAll(filepath.Join(home, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}

	srcParent := t.TempDir()
	src := filepath.Join(srcParent, "kagi")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extension.json"), []byte(`{"name":"kagi"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(cwd)
	if err := os.Chdir(src); err != nil {
		t.Fatal(err)
	}

	if err := extInstall([]string{"."}); err != nil {
		t.Fatalf("install with '.' failed: %v", err)
	}

	out := filepath.Join(home, "extensions", "kagi")
	if _, err := os.Stat(filepath.Join(out, "extension.json")); err != nil {
		t.Fatalf("expected installed extension at %s: %v", out, err)
	}
}

// TestExtInstallRejectsParentName guards against deriving a name of ".."
// from a source that resolves to a filesystem root edge case. A normal
// directory always yields a real basename, so this just ensures the
// guard logic does not crash for well-formed input.
func TestExtInstallNamedDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)

	src := filepath.Join(t.TempDir(), "myext")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "extension.json"), []byte(`{"name":"myext"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := extInstall([]string{src}); err != nil {
		t.Fatalf("install failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, "extensions", "myext", "extension.json")); err != nil {
		t.Fatalf("expected installed extension: %v", err)
	}
}

// TestCopyDirRespectsGitignore verifies that non-portable directories
// listed in the source .gitignore (e.g. .venv, node_modules) are not
// copied during install, while tracked files are.
func TestCopyDirRespectsGitignore(t *testing.T) {
	src := t.TempDir()
	dst := filepath.Join(t.TempDir(), "out")

	mustWrite := func(rel, content string) {
		p := filepath.Join(src, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite("extension.json", `{"name":"x"}`)
	mustWrite("main.py", "print('hi')")
	mustWrite(".gitignore", ".venv/\nnode_modules/\n*.log\n")
	mustWrite(".venv/bin/python", "binary")
	mustWrite("node_modules/pkg/index.js", "module")
	mustWrite("debug.log", "noise")
	mustWrite("src/app.py", "code")
	mustWrite(".git/config", "gitdir")

	if err := copyDir(src, dst); err != nil {
		t.Fatal(err)
	}

	wantPresent := []string{"extension.json", "main.py", "src/app.py", ".gitignore"}
	for _, rel := range wantPresent {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err != nil {
			t.Fatalf("expected %s to be copied: %v", rel, err)
		}
	}

	wantAbsent := []string{".venv", "node_modules", "debug.log", ".git"}
	for _, rel := range wantAbsent {
		if _, err := os.Stat(filepath.Join(dst, filepath.FromSlash(rel))); err == nil {
			t.Fatalf("expected %s to be skipped, but it was copied", rel)
		}
	}
}

func TestGitignoreNegation(t *testing.T) {
	g := loadGitignoreFromString("build/\n!build/keep.txt\n")
	if !g.Match("build", true) {
		t.Fatal("expected build/ dir to be ignored")
	}
	if g.Match("build/keep.txt", false) {
		t.Fatal("expected build/keep.txt to be re-included by negation")
	}
}

func TestExtDoctorStaticScanAndRender(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZOT_HOME", home)
	cwd := t.TempDir()
	oldCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer os.Chdir(oldCWD)
	if err := os.Chdir(cwd); err != nil {
		t.Fatal(err)
	}

	mustWrite := func(path, body string) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	mustWrite(filepath.Join(cwd, ".zot", "extensions", "disabled", "extension.json"), `{"name":"disabled","exec":"./run.sh","enabled":false}`)
	mustWrite(filepath.Join(cwd, ".zot", "extensions", "theme", "extension.json"), `{"name":"theme"}`)
	mustWrite(filepath.Join(cwd, ".zot", "extensions", "theme", "theme.json"), `{"name":"Theme"}`)
	mustWrite(filepath.Join(cwd, ".zot", "extensions", "bad", "extension.json"), `{bad json`)
	mustWrite(filepath.Join(cwd, ".zot", "extensions", "dup", "extension.json"), `{"name":"dup","exec":"./project.sh"}`)
	mustWrite(filepath.Join(home, "extensions", "dup", "extension.json"), `{"name":"dup","exec":"./global.sh"}`)

	rows := scanExtDoctorStatic()
	if len(rows) != 5 {
		t.Fatalf("rows = %d, want 5: %#v", len(rows), rows)
	}

	var out bytes.Buffer
	for _, row := range rows {
		printExtDoctorRow(&out, row, extensions.ExtensionDiagnostic{})
	}
	got := out.String()
	for _, want := range []string{
		"disabled [project] disabled",
		"theme [project] theme-only",
		"bad [project] error",
		"dup [global] shadowed",
		"parse manifest:",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("doctor output missing %q:\n%s", want, got)
		}
	}
}

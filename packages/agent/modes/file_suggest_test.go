package modes

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestExpandFileChipsAtPickerShape(t *testing.T) {
	cwd := filepath.Join(string(filepath.Separator), "repo")
	in := "read [file:README.md] and [dir:docs/]"
	want := "read " + filepath.Join(cwd, "README.md") + " and " + filepath.Join(cwd, "docs")
	if got := expandFileChips(in, cwd); got != want {
		t.Fatalf("expandFileChips() = %q, want %q", got, want)
	}
}

func TestExpandFileChipsLeavesEditorPlaceholderShapeAlone(t *testing.T) {
	cwd := filepath.Join(string(filepath.Separator), "repo")
	// tui.Editor.SubmitValue should expand [file:N:name] using its
	// private path map before modes see the text. If such a token leaks
	// through, modes must not guess that "1:foo.txt" is a relative path.
	in := "read [file:1:foo.txt]"
	if got := expandFileChips(in, cwd); got != in {
		t.Fatalf("editor placeholder was changed: %q", got)
	}
}

// TestFileSuggesterPicksUpNewEntries pins the cache-invalidation bug
// fix: creating a subdirectory after the picker has already scanned
// the cwd must surface that subdirectory on the next scan, without
// any explicit invalidation call. The cache is keyed on the dir's
// mtime, which the OS bumps on every entry add/remove/rename.
func TestFileSuggesterPicksUpNewEntries(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "existing.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	s := newFileSuggester()
	s.SetCWD(tmp)

	first := s.scan()
	if !containsEntry(first, "existing.txt", false) {
		t.Fatalf("first scan missing existing.txt: %#v", first)
	}
	if containsEntry(first, "test", true) {
		t.Fatalf("first scan unexpectedly saw the not-yet-created test/: %#v", first)
	}

	// Sleep one filesystem tick: HFS+/APFS/ext4 all bump directory
	// mtime with at-least 1s resolution depending on mount options.
	// Without this sleep on coarse-resolution filesystems the mtime
	// after Mkdir can equal the mtime captured during the first scan
	// and the cache would (correctly) be retained.
	time.Sleep(1100 * time.Millisecond)
	if err := os.Mkdir(filepath.Join(tmp, "test"), 0o755); err != nil {
		t.Fatal(err)
	}

	second := s.scan()
	if !containsEntry(second, "test", true) {
		t.Fatalf("second scan did not pick up the newly created test/: %#v", second)
	}
	if !containsEntry(second, "existing.txt", false) {
		t.Fatalf("second scan dropped existing.txt: %#v", second)
	}

	// Directories sort before files.
	sorted := make([]string, 0, len(second))
	for _, e := range second {
		sorted = append(sorted, e.name)
	}
	if !sort.IsSorted(byDirsFirst(second)) {
		t.Fatalf("entries are not dirs-first / case-insensitive sorted: %v", sorted)
	}
}

func containsEntry(entries []fileEntry, name string, isDir bool) bool {
	for _, e := range entries {
		if e.name == name && e.isDir == isDir {
			return true
		}
	}
	return false
}

type byDirsFirst []fileEntry

func (b byDirsFirst) Len() int      { return len(b) }
func (b byDirsFirst) Swap(i, j int) { b[i], b[j] = b[j], b[i] }
func (b byDirsFirst) Less(i, j int) bool {
	if b[i].isDir != b[j].isDir {
		return b[i].isDir
	}
	return b[i].name < b[j].name
}

// TestFileSuggesterFuzzyMatch verifies the @-query ranks entries with
// a fuzzy subsequence match rather than a plain substring, so a
// non-contiguous pattern like "fsg" still finds "file_suggest.go".
func TestFileSuggesterFuzzyMatch(t *testing.T) {
	tmp := t.TempDir()
	for _, name := range []string{"file_suggest.go", "interactive.go", "README.md"} {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s := newFileSuggester()
	s.SetCWD(tmp)

	got := s.matches("@fsg")
	if !containsEntry(got, "file_suggest.go", false) {
		t.Fatalf("fuzzy query @fsg did not match file_suggest.go: %#v", got)
	}
	if len(got) == 0 || got[0].name != "file_suggest.go" {
		t.Fatalf("file_suggest.go not ranked first for @fsg: %#v", got)
	}
}

// TestFileSuggesterRecursiveMatch verifies recursive mode flattens the
// tree and matches against the cwd-relative path, so a pattern can
// span directory boundaries.
func TestFileSuggesterRecursiveMatch(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "src", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "src", "foo", "bar.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newFileSuggester()
	s.SetCWD(tmp)
	s.SetRecursive(true)

	rel := filepath.Join("src", "foo", "bar.go")
	got := s.matches("@foobar")
	if !containsEntry(got, rel, false) {
		t.Fatalf("recursive @foobar did not match %s: %#v", rel, got)
	}
}

// TestFileSuggesterRecursiveSkipsHeavyDirs ensures the walk prunes
// directories like .git that would otherwise dominate the budget.
func TestFileSuggesterRecursiveSkipsHeavyDirs(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, ".git", "objects"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, ".git", "objects", "deadbeef"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newFileSuggester()
	s.SetCWD(tmp)
	s.SetRecursive(true)

	all := s.scan()
	for _, e := range all {
		if e.rel == ".git" || strings.HasPrefix(e.rel, ".git"+string(filepath.Separator)) {
			t.Fatalf("recursive scan descended into .git: %#v", e)
		}
	}
	if !containsEntry(all, "main.go", false) {
		t.Fatalf("recursive scan missing main.go: %#v", all)
	}
}

// TestFileSuggesterRecursiveSkipsIaCCaches ensures infrastructure-as-
// code provider/module caches are pruned so a recursive walk doesn't
// drown in copies of downloaded providers or generated modules.
func TestFileSuggesterRecursiveSkipsIaCCaches(t *testing.T) {
	tmp := t.TempDir()
	for _, dir := range []string{".terraform", ".terragrunt-cache", "cdk.out"} {
		nested := filepath.Join(tmp, dir, "deep")
		if err := os.MkdirAll(nested, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(nested, "junk.tf"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(tmp, "main.tf"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newFileSuggester()
	s.SetCWD(tmp)
	s.SetRecursive(true)

	for _, e := range s.scan() {
		for _, skip := range []string{".terraform", ".terragrunt-cache", "cdk.out"} {
			if e.rel == skip || strings.HasPrefix(e.rel, skip+string(filepath.Separator)) {
				t.Fatalf("recursive scan descended into %s: %#v", skip, e)
			}
		}
	}
	if !containsEntry(s.scan(), "main.tf", false) {
		t.Fatal("recursive scan missing main.tf")
	}
}

// TestFileSuggesterToggleResetsCache verifies SetRecursive drops the
// cached scan so the next matches() reflects the new mode.
func TestFileSuggesterToggleResetsCache(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmp, "pkg", "nested.go"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newFileSuggester()
	s.SetCWD(tmp)

	rel := filepath.Join("pkg", "nested.go")
	if containsEntry(s.matches("@nested"), rel, false) {
		t.Fatal("flat mode unexpectedly saw nested.go")
	}
	s.SetRecursive(true)
	if !containsEntry(s.matches("@nested"), rel, false) {
		t.Fatal("recursive mode did not surface nested.go after toggle")
	}
}

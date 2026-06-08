package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/patriceckhart/zot/packages/provider"
)

func TestReadAgentsContextLoadsGlobalAndAncestors(t *testing.T) {
	root := t.TempDir()
	zotHome := filepath.Join(root, "zot-home")
	project := filepath.Join(root, "repo")
	nested := filepath.Join(project, "packages", "app")
	if err := os.MkdirAll(zotHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(zotHome, "AGENTS.md"), []byte("global rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(project, "AGENTS.md"), []byte("repo rule"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "AGENTS.md"), []byte("app rule"), 0o644); err != nil {
		t.Fatal(err)
	}

	got := readAgentsContext(nested, zotHome)
	for _, want := range []string{"global rule", "repo rule", "app rule"} {
		if !strings.Contains(got, want) {
			t.Fatalf("readAgentsContext missing %q in:\n%s", want, got)
		}
	}
	if strings.Index(got, "global rule") > strings.Index(got, "repo rule") || strings.Index(got, "repo rule") > strings.Index(got, "app rule") {
		t.Fatalf("AGENTS.md files loaded in wrong order:\n%s", got)
	}
}

func TestReadAgentsContextMissingFilesIsEmpty(t *testing.T) {
	got := readAgentsContext(t.TempDir(), t.TempDir())
	if got != "" {
		t.Fatalf("expected no context, got %q", got)
	}
}

// TestResolveFallsBackWhenConfiguredModelIsGone reproduces the
// startup failure caught by the user's screenshot: the persisted
// config.json points at a model id that's no longer in the active
// catalogue (because they edited models.json or zot's bundled
// catalogue changed). Resolve must NOT error — strands the user
// with no way to fix it from the TUI — and should repair the config
// so the next launch is silent.
func TestResolveFallsBackWhenConfiguredModelIsGone(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	// Persist a stale model id.
	stale := "gpt-5.5-pro-not-real"
	if err := SaveConfig(Config{Provider: "openai", Model: stale}); err != nil {
		t.Fatal(err)
	}

	r, err := Resolve(Args{}, false)
	if err != nil {
		t.Fatalf("Resolve refused to launch with stale model: %v", err)
	}
	if r.Model == stale {
		t.Fatalf("Resolve kept stale model %q", r.Model)
	}
	if r.Provider != "openai" {
		t.Errorf("provider drifted: got %q; want openai", r.Provider)
	}

	// Config on disk should now hold the fallback so subsequent
	// launches don't repeat the warning.
	cfg, _ := LoadConfig()
	if cfg.Model == stale {
		t.Errorf("config.json still pins the stale model %q", cfg.Model)
	}
	if cfg.Model == "" {
		t.Errorf("config.json was emptied; expected the fallback model id")
	}
}

// TestResolveExplicitFlagStaleDoesNotRepairConfig confirms the
// repair-on-disk happens ONLY when the stale id came from the
// persisted config. If the user passed --model X explicitly and X is
// unknown, we still fall back, but we don't touch their config.
func TestResolveExplicitFlagStaleDoesNotRepairConfig(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	t.Setenv("OPENAI_API_KEY", "test-key")
	good := "gpt-5"
	if err := SaveConfig(Config{Provider: "openai", Model: good}); err != nil {
		t.Fatal(err)
	}

	r, err := Resolve(Args{Model: "gpt-totally-fake"}, false)
	if err != nil {
		t.Fatalf("Resolve errored on unknown --model: %v", err)
	}
	if r.Model == "gpt-totally-fake" {
		t.Errorf("Resolve kept the bogus --model value")
	}
	cfg, _ := LoadConfig()
	if cfg.Model != good {
		t.Errorf("config.json was clobbered (was %q; now %q)", good, cfg.Model)
	}
}

func TestResolveOllamaUsesModelBaseURLBeforeDefault(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	provider.SetLiveModels(nil)
	defer provider.SetLiveModels(nil)
	provider.SetUserModels([]provider.Model{{
		Provider:      "ollama",
		ID:            "qwen-local",
		DisplayName:   "Qwen Local",
		ContextWindow: 32768,
		MaxOutput:     8192,
		BaseURL:       "http://localhost:8000/v1",
	}})

	r, err := Resolve(Args{Provider: "ollama", Model: "qwen-local"}, false)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if r.BaseURL != "http://localhost:8000/v1" {
		t.Fatalf("BaseURL = %q, want models.json baseUrl", r.BaseURL)
	}
}

func TestResolveOllamaFallsBackToDefaultBaseURL(t *testing.T) {
	t.Setenv("ZOT_HOME", t.TempDir())
	provider.SetLiveModels(nil)
	defer provider.SetLiveModels(nil)

	r, err := Resolve(Args{Provider: "ollama", Model: "any-local-model"}, false)
	if err != nil {
		t.Fatalf("Resolve failed: %v", err)
	}
	if r.BaseURL != "http://localhost:11434" {
		t.Fatalf("BaseURL = %q, want ollama default", r.BaseURL)
	}
}

func TestCanonicalProviderResolvesAliases(t *testing.T) {
	cases := map[string]string{
		"bedrock":         "amazon-bedrock",
		"AWS-Bedrock":     "amazon-bedrock",
		"  bedrock  ":     "amazon-bedrock",
		"vertex":          "google-vertex",
		"gemini":          "google",
		"azure":           "azure-openai-responses",
		"copilot":         "github-copilot",
		"codex":           "openai-codex",
		"moonshot":        "moonshotai",
		"vercel":          "vercel-ai-gateway",
		"hf":              "huggingface",
		"anthropic":       "anthropic",       // canonical passes through
		"amazon-bedrock":  "amazon-bedrock",  // already canonical
		"totally-unknown": "totally-unknown", // unknown returned unchanged (lowered)
		"Totally-UNKNOWN": "totally-unknown",
		"":                "",
	}
	for in, want := range cases {
		if got := canonicalProvider(in); got != want {
			t.Errorf("canonicalProvider(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCanonicalProviderAliasesAreKnown(t *testing.T) {
	for alias, canon := range providerAliases {
		if !isKnownProvider(canon) {
			t.Errorf("alias %q maps to %q which is not a known provider", alias, canon)
		}
	}
}

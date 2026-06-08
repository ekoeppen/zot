package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/patriceckhart/zot/packages/provider"
)

// ModelCachePath returns the on-disk location of the merged model cache.
func ModelCachePath() string {
	return filepath.Join(ZotHome(), "models-cache.json")
}

// UserModelsPath returns the path to the user's models.json override.
func UserModelsPath() string {
	return filepath.Join(ZotHome(), "models.json")
}

// LoadCachedModels loads the cache file and applies it to the provider
// package so FindModel / ModelsForProvider see live ids immediately.
// Safe to call before any credentials are known.
func LoadCachedModels() {
	c, err := provider.LoadCache(ModelCachePath())
	if err != nil {
		return
	}
	if len(c.Models) > 0 {
		provider.SetLiveModels(c.Models)
	}
}

// LoadUserModels reads $ZOT_HOME/models.json and merges any user-defined
// models into the active catalog. User models take highest precedence.
// Any validation issues (bad provider id, empty model id, malformed
// JSON, negative widths) are surfaced as one warning per line on stderr;
// the well-formed entries from the rest of the file are still loaded.
func LoadUserModels() {
	models, warnings := provider.LoadUserModelsWithWarnings(UserModelsPath())
	for _, w := range warnings {
		fmt.Fprintln(os.Stderr, "zot:", w)
	}
	provider.SetUserModels(models)
}

// ValidateAndRepairConfig checks the persisted config.json's
// (Provider, Model) pair against the active catalog and repairs any
// mismatch in-place (and on disk) before any UI renders. Three failure
// modes are handled:
//
//   - cfg.Provider is empty or unknown -> reset to "anthropic".
//   - cfg.Model is empty -> set to the provider's default.
//   - cfg.Model belongs to a different provider than cfg.Provider
//     (e.g. provider=anthropic + model=kimi-for-coding from a stale
//     half-applied switch) -> reset model to the provider's default.
//
// Silent on success; one stderr line per repair. Errors loading or
// saving the file are non-fatal — the caller continues with defaults.
func ValidateAndRepairConfig() {
	cfg, err := LoadConfig()
	if err != nil {
		fmt.Fprintf(os.Stderr, "zot: config.json: %v (using defaults)\n", err)
		return
	}
	changed := false

	if cfg.Provider != "" && !isKnownProvider(cfg.Provider) {
		fmt.Fprintf(os.Stderr, "zot: config.json: unknown provider %q reset to \"anthropic\"\n", cfg.Provider)
		cfg.Provider = "anthropic"
		cfg.Model = ""
		changed = true
	}

	if cfg.Provider != "" && cfg.Model != "" {
		if _, err := provider.FindModel(cfg.Provider, cfg.Model); err != nil {
			if m, err := provider.FindModel("", cfg.Model); err == nil {
				fix := defaultModelForProvider(cfg.Provider)
				fmt.Fprintf(os.Stderr,
					"zot: config.json: model %q belongs to provider %q (config has provider=%q); switched model to %q\n",
					cfg.Model, m.Provider, cfg.Provider, fix)
				cfg.Model = fix
				changed = true
			} else if cfg.Provider != "ollama" {
				// Model id not in any catalog. Reset to provider's default.
				fix := defaultModelForProvider(cfg.Provider)
				fmt.Fprintf(os.Stderr,
					"zot: config.json: model %q not found in the active catalog; switched to %q\n",
					cfg.Model, fix)
				cfg.Model = fix
				changed = true
			}
		}
	}

	if changed {
		if err := SaveConfig(cfg); err != nil {
			fmt.Fprintf(os.Stderr, "zot: config.json: failed to persist repair: %v\n", err)
		}
	}
}

// RefreshModelsAsync kicks a background discovery for every provider
// we have credentials for. Refreshed results are merged into the
// active catalog and persisted to the on-disk cache.
//
// Silent on error: discovery is a nice-to-have. Callers can still use
// the baked-in catalog if this fails.
func RefreshModelsAsync() {
	go refreshModels()
}

func refreshModels() {
	cached, _ := provider.LoadCache(ModelCachePath())
	if cached.IsFresh() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	var all []provider.Model

	if cred, method, err := ResolveCredential("anthropic", ""); err == nil && method == "apikey" {
		// /v1/models on Anthropic is API-key only; OAuth tokens can
		// also list models via the bearer header, but we skip OAuth
		// here to avoid surprise rate-limit hits on subscription keys.
		if live, err := provider.DiscoverAnthropic(ctx, cred, ""); err == nil {
			all = append(all, live...)
		}
	}
	if cred, method, err := ResolveCredential("openai", ""); err == nil && method == "apikey" {
		if live, err := provider.DiscoverOpenAI(ctx, cred, ""); err == nil {
			all = append(all, live...)
		}
	}
	if cred, method, err := ResolveCredential("kimi", ""); err == nil && method == "apikey" {
		if live, err := provider.DiscoverOpenAI(ctx, cred, "https://api.kimi.com/coding/v1"); err == nil {
			for i := range live {
				live[i].Provider = "kimi"
				live[i].Source = "live"
			}
			all = append(all, live...)
		}
	}
	if cred, method, err := ResolveCredential("google", ""); err == nil && method == "apikey" {
		if live, err := provider.DiscoverGoogle(ctx, cred, ""); err == nil {
			all = append(all, live...)
		}
	}
	if _, _, err := ResolveCredential("openrouter", ""); err == nil {
		// /models is public; gate on a credential so the picker only
		// fills with OpenRouter's hundreds of routes for users who use it.
		if live, err := provider.DiscoverOpenRouter(ctx, ""); err == nil {
			all = append(all, live...)
		}
	}

	if len(all) == 0 {
		return
	}
	provider.SetLiveModels(all)
	_ = provider.SaveCache(ModelCachePath(), provider.ModelCache{
		FetchedAt: time.Now().UTC(),
		Models:    all,
	})
}

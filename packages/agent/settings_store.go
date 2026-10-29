package agent

import (
	"github.com/patriceckhart/zot/packages/provider"
	"github.com/patriceckhart/zot/packages/tui"
)

type configSettingsStore struct{}

func (configSettingsStore) SetQuickModelShortcut(slot int, providerName, model string) error {
	if slot < 1 || slot > 9 {
		return nil
	}
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if len(cfg.QuickModelShortcuts) < slot {
		next := make([]QuickModelShortcut, slot)
		copy(next, cfg.QuickModelShortcuts)
		cfg.QuickModelShortcuts = next
	}
	cfg.QuickModelShortcuts[slot-1] = QuickModelShortcut{Provider: providerName, Model: model}
	// Trim trailing empty slots so config.json stays compact.
	for len(cfg.QuickModelShortcuts) > 0 {
		last := cfg.QuickModelShortcuts[len(cfg.QuickModelShortcuts)-1]
		if last.Provider != "" || last.Model != "" {
			break
		}
		cfg.QuickModelShortcuts = cfg.QuickModelShortcuts[:len(cfg.QuickModelShortcuts)-1]
	}
	return SaveConfig(cfg)
}

func (configSettingsStore) SetInlineImages(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.InlineImagesEnabled = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetAutoSwarm(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.AutoSwarmEnabled = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetAutoCompactThreshold(percent int) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.AutoCompactThreshold = &percent
	return SaveConfig(cfg)
}

func (configSettingsStore) SetJailByDefault(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.JailByDefault = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetPowerShell(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.PowerShellEnabled = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetOpenRouterServerTools(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.OpenRouterServerToolsEnabled = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetRecursiveFileSuggest(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.RecursiveFileSuggest = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetFuzzySkillSuggest(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.FuzzySkillSuggest = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetRespectGitignore(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.RespectGitignore = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetCompactMode(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.CompactMode = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetCollapseToolCall(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.CollapseToolCall = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetChatTimestamps(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.ChatTimestamps = enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetChatTimestampDate(mode string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.ChatTimestampDate = tui.ChatTimestampDateMode(mode)
	return SaveConfig(cfg)
}

func (configSettingsStore) SetChatTimestampInterval(minutes int) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	minutes = tui.ChatTimestampInterval(&minutes)
	cfg.ChatTimestampIntervalMinutes = &minutes
	return SaveConfig(cfg)
}

func (configSettingsStore) SetShowReasoning(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.ShowReasoning = enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetShowInstructionsAtStartup(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.ShowInstructionsAtStartup = &enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetTUIClickToPosition(enabled bool) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.TUIClickToPosition = enabled
	return SaveConfig(cfg)
}

func (configSettingsStore) SetTUIInputStyle(style string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	style = tui.NormalizeInputStyle(style)
	if style == tui.InputStylePlain {
		cfg.TUIInputStyle = ""
	} else {
		cfg.TUIInputStyle = style
	}
	return SaveConfig(cfg)
}

func (configSettingsStore) SetTUIStatusPosition(position string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	position = tui.NormalizeStatusPosition(position)
	if position == tui.StatusPositionAboveInput {
		cfg.TUIStatusPosition = ""
	} else {
		cfg.TUIStatusPosition = position
	}
	return SaveConfig(cfg)
}

func (configSettingsStore) SetTUIWorkingPosition(position string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	position = tui.NormalizeWorkingPosition(position)
	if position == tui.WorkingPositionAboveInput {
		cfg.TUIWorkingPosition = ""
	} else {
		cfg.TUIWorkingPosition = position
	}
	return SaveConfig(cfg)
}

func (configSettingsStore) SetReasoning(level string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	cfg.Reasoning = provider.NormalizeReasoning(level)
	return SaveConfig(cfg)
}

func (configSettingsStore) SetTheme(name string) error {
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if name == "auto" {
		name = ""
	}
	cfg.Theme = name
	return SaveConfig(cfg)
}

// AutoSwarmEnabled reads the current auto-swarm flag from config.
// Used by the swarm_spawn tool at call time to gate execution.
func AutoSwarmEnabled() bool {
	cfg, err := LoadConfig()
	if err != nil {
		return false
	}
	return cfg.AutoSwarmEnabled != nil && *cfg.AutoSwarmEnabled
}

// AutoSwarmSystemAddendum is appended to the system prompt when
// auto-swarm is enabled, so the model knows it may delegate to
// background sub-agents without the user having to mention the tool
// by name. Kept short so it doesn't bloat the cached prompt prefix.
const AutoSwarmSystemAddendum = `Auto-swarm is enabled. You have a swarm_spawn tool that forks background sub-agents working in parallel in this same working directory.

Use swarm_status to inspect current-session agents, prompt-level busy state, last event time, and durable follow-up outcomes. Use swarm_send to durably queue a follow-up, including for stopped or resumed agents. Each request returns an id and produces its own correlated completion update. Use swarm_cancel to cancel a stalled active prompt without killing its daemon, or swarm_kill followed by swarm_resume to restart local supervision. On a continuous host, kill only detaches, use cancel to abort host work. Resume dispatches unsent follow-ups but never reruns the initial task or interrupted work. Inspect interrupted outcomes before explicitly retrying. Activity "idle" alone does not imply completion or failure.

Use it proactively when the user's request naturally splits into independent sub-tasks that can run concurrently (e.g. "refactor module A and module B", "write the implementation and the tests", "investigate three separate files"). Spawn one sub-agent per independent sub-task with a self-contained task description (sub-agents start with no context from this conversation). Continue working on the remaining or coordinating work yourself in parallel; do not wait for sub-agents to finish before responding. Briefly tell the user which sub-agents you spawned and what each is doing.

Do NOT use swarm_spawn for trivial single-step work, for tasks that depend on each other sequentially, or when the user explicitly asked you to do the work yourself.

Each delegated task reports an [auto-swarm update] as soon as it finishes or exits, without waiting for its siblings. Treat updates as observed state, not new user requests. Summarise the reported outcome using agent and request ids. Other agents may still be working, so check their status before claiming the entire request is finished.`

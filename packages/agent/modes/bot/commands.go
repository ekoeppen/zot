package bot

import "strings"

// IsStopCommand reports whether text should abort the active turn.
// Users often type plain "stop" rather than bot-style "/stop"; keep
// this intentionally narrow so normal prompts like "stop doing X"
// still go to the agent.
func IsStopCommand(text string) bool {
	return strings.EqualFold(strings.TrimSpace(text), "stop")
}

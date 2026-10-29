package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/patriceckhart/zot/packages/agent/swarm"
	"github.com/patriceckhart/zot/packages/core"
	"github.com/patriceckhart/zot/packages/provider"
)

// SwarmControlTools exposes session-scoped supervision alongside swarm_spawn.
func SwarmControlTools(mgr *swarm.Swarm, enabled func() bool, completed func(*swarm.Agent, swarm.QueuedTurn) bool) []core.Tool {
	var out []core.Tool
	for _, action := range []string{"status", "kill", "send", "cancel", "resume"} {
		out = append(out, &swarmControlTool{action: action, swarm: mgr, enabled: enabled, completed: completed})
	}
	return out
}

type swarmControlTool struct {
	action    string
	swarm     *swarm.Swarm
	enabled   func() bool
	completed func(*swarm.Agent, swarm.QueuedTurn) bool
}

func (t *swarmControlTool) Name() string { return "swarm_" + t.action }
func (t *swarmControlTool) Description() string {
	switch t.action {
	case "status":
		return "Inspect current-session background agents, their lifecycle status, activity, task, and latest response. Omit agent_id to list. Activity idle is not proof that a task is complete."
	case "kill":
		return "Stop local supervision of a current-session agent, preserving its state and unsent follow-ups. On a continuous host this detaches without aborting host work, use swarm_cancel to abort it."
	case "cancel":
		return "Cancel the active prompt of a current-session agent without killing its daemon. Queued follow-ups can then proceed. Use this for a stalled task rather than duplicating its work."
	case "resume":
		return "Resume a stopped or detached current-session agent and dispatch its durable unsent follow-ups. Does not rerun the initial task or interrupted work. Inspect interrupted outcomes before explicitly retrying them."
	default:
		return "Durably queue a follow-up for a current-session agent, including a stopped or resumed agent. Returns a request id immediately. The host reports that exact turn's result. Use swarm_cancel if active work is stalled, or swarm_resume if the agent is stopped."
	}
}
func (t *swarmControlTool) Schema() json.RawMessage {
	required := `[]`
	if t.action != "status" {
		required = `["agent_id"]`
	}
	if t.action == "send" {
		required = `["agent_id","message"]`
	}
	return json.RawMessage(`{"type":"object","properties":{"agent_id":{"type":"string","description":"Exact agent id returned by swarm_spawn or swarm_status"},"message":{"type":"string","description":"Follow-up task for swarm_send"}},"required":` + required + `}`)
}
func (t *swarmControlTool) Execute(ctx context.Context, raw json.RawMessage, progress func(string)) (core.ToolResult, error) {
	if err := ctx.Err(); err != nil {
		return core.ToolResult{}, err
	}
	if t.swarm == nil || t.enabled == nil || !t.enabled() {
		return toolErr(t.Name() + ": auto-swarm is unavailable or disabled"), nil
	}
	var args struct {
		ID      string `json:"agent_id"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(raw, &args); err != nil {
		return toolErr("invalid arguments: " + err.Error()), nil
	}
	args.ID = strings.TrimSpace(args.ID)
	var agents []*swarm.Agent
	active := t.swarm.ActiveSession()
	for _, a := range t.swarm.List() {
		if active != "" && a.SessionID != active {
			continue
		}
		if args.ID == "" || args.ID == a.ID {
			agents = append(agents, a)
		}
	}
	if t.action != "status" && args.ID == "" {
		return toolErr("agent_id is required"), nil
	}
	if args.ID != "" && len(agents) == 0 {
		return toolErr("agent not found in the current session"), nil
	}
	var text strings.Builder
	if t.action == "status" {
		for _, a := range agents {
			s := a.Snapshot()
			fmt.Fprintf(&text, "agent: %s\nstatus: %s\nbusy: %t\nlast event: %s\nactivity: %s\ntask: %s\nerror: %s\nlatest response:\n%s\n", s.ID, s.Status, s.Busy, s.LastEvent, s.Activity, s.Task, s.Err, s.LastAssistant)
			for _, turn := range a.QueuedTurns() {
				fmt.Fprintf(&text, "request: %s, state: %s\nmessage: %s\nerror: %s\nresponse:\n%s\n", turn.ID, turn.State, turn.Text, turn.Error, turn.Response)
			}
			text.WriteString("\n")
		}
		if len(agents) == 0 {
			text.WriteString("No agents in the current session.")
		}
	} else {
		a := agents[0]
		var err error
		switch t.action {
		case "kill":
			err = t.swarm.Stop(a.ID)
		case "cancel":
			if a.Status() != swarm.StatusRunning && a.Status() != swarm.StatusPending {
				return toolErr("agent is stopped, use swarm_resume before cancelling active work"), nil
			}
			err = t.swarm.SendInput(a.ID, "cancel")
		case "resume":
			_, err = t.swarm.Resume(ctx, a.ID)
		case "send":
			if t.completed == nil {
				return toolErr("follow-up notifications are unavailable in this mode"), nil
			}
			var requestID string
			requestID, err = t.swarm.QueueTurn(a.ID, args.Message, func(turn swarm.QueuedTurn) bool { return t.completed(a, turn) })
			if err == nil {
				fmt.Fprintf(&text, "queued request %s\n", requestID)
				if a.Status() != swarm.StatusRunning && a.Status() != swarm.StatusPending {
					text.WriteString("Agent is stopped, use swarm_resume to dispatch pending work.\n")
				}
			}
		}
		if err != nil {
			return toolErr(t.Name() + ": " + err.Error()), nil
		}
		fmt.Fprintf(&text, "%s accepted for agent %s", t.Name(), a.ID)
	}
	return core.ToolResult{Content: []provider.Content{provider.TextBlock{Text: text.String()}}}, nil
}

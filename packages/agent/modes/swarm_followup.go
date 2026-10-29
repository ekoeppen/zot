package modes

import "github.com/patriceckhart/zot/packages/agent/swarm"

// SwarmQueuedTurnCompleted reports a correlated durable result, not whatever
// assistant message happens to be latest when the callback runs. Returning
// false retains the notice for delivery when its session becomes active.
func (i *Interactive) SwarmQueuedTurnCompleted(a *swarm.Agent, turn swarm.QueuedTurn) bool {
	if i == nil || a == nil {
		return false
	}
	if i.cfg.Swarm != nil {
		active := i.cfg.Swarm.ActiveSession()
		if active != "" && a.SessionID != active {
			return false
		}
	}
	snapshot := a.Snapshot()
	snapshot.LastAssistant = turn.Response
	snapshot.Err = ""
	snapshot.Tail = ""
	i.flushSwarmSummary([]*swarmWatchEntry{{agent: a, task: turn.Text, outcome: turn.State, err: turn.Error, requestID: turn.ID, snapshot: &snapshot}})
	return true
}

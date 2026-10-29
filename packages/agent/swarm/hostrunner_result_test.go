package swarm

import (
	"encoding/json"
	"testing"

	"github.com/patriceckhart/zot/packages/continuous"
	"github.com/patriceckhart/zot/packages/provider"
)

func TestHostSubmissionResultUsesTerminalCommitWithoutCheckpoint(t *testing.T) {
	entry := func(revision uint64, text string) continuous.Entry {
		message, err := json.Marshal(provider.Message{Role: provider.RoleAssistant, Content: []provider.Content{provider.TextBlock{Text: text}}})
		if err != nil {
			t.Fatal(err)
		}
		return continuous.Entry{Type: "assistant", Revision: revision, Message: message}
	}
	base := continuous.ConversationSnapshot{
		Tasks:   []continuous.Task{{Kind: continuous.TaskKindGeneration, State: "terminal", Revision: 7, Result: json.RawMessage(`{"submissions":["original"]}`)}},
		Run:     &continuous.Run{Phase: "done", Revision: 8, Submissions: []string{"later"}},
		Entries: []continuous.Entry{entry(6, "earlier response"), entry(7, "original response"), entry(8, "later response")},
	}
	for _, name := range []string{"original", "missing metadata", "outside snapshot window", "empty final response", "invalid final message"} {
		t.Run(name, func(t *testing.T) {
			snap := base
			want, wantError := "original response", false
			switch name {
			case "missing metadata":
				snap.Tasks = nil
				want, wantError = "", true
			case "outside snapshot window":
				snap.Entries = []continuous.Entry{entry(8, "later response")}
				snap.More = true
				want, wantError = "", true
			case "empty final response":
				snap.Entries = []continuous.Entry{entry(7, "")}
				want = ""
			case "invalid final message":
				snap.Entries = []continuous.Entry{{Type: "assistant", Revision: 7, Message: json.RawMessage(`invalid`)}}
				want, wantError = "", true
			}
			response, errMsg := hostSubmissionResult(snap, continuous.Submission{ID: "original", State: "answered"})
			if response != want || (errMsg != "") != wantError {
				t.Fatalf("result: response %q, error %q", response, errMsg)
			}
		})
	}
}

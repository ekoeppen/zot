package agent

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestSpecForSubcommand(t *testing.T) {
	if s := specFor("telegram-bot"); s == nil || s.name != "telegram" {
		t.Fatalf("telegram-bot not matched: %#v", s)
	}
	if s := specFor("tg"); s == nil || s.name != "telegram" {
		t.Fatal("alias tg not matched")
	}
	if specFor("nonsense") != nil {
		t.Fatal("unknown subcommand must not match")
	}
}

func TestReadBotTokenDoesNotEchoTerminalInput(t *testing.T) {
	in, err := os.CreateTemp(t.TempDir(), "stdin")
	if err != nil {
		t.Fatal(err)
	}
	defer in.Close()

	oldIsTerminal := botTokenIsTerminal
	oldReadPassword := botTokenReadPassword
	t.Cleanup(func() {
		botTokenIsTerminal = oldIsTerminal
		botTokenReadPassword = oldReadPassword
	})
	botTokenIsTerminal = func(fd int) bool {
		if fd != int(in.Fd()) {
			t.Fatalf("terminal check fd = %d, want %d", fd, in.Fd())
		}
		return true
	}
	botTokenReadPassword = func(fd int) ([]byte, error) {
		if fd != int(in.Fd()) {
			t.Fatalf("password read fd = %d, want %d", fd, in.Fd())
		}
		return []byte("secret-token"), nil
	}

	var out bytes.Buffer
	got, err := readBotToken(in, &out)
	if err != nil {
		t.Fatalf("readBotToken() error = %v", err)
	}
	if got != "secret-token" {
		t.Fatalf("readBotToken() = %q, want secret token", got)
	}
	if strings.Contains(out.String(), got) {
		t.Fatalf("terminal output exposed bot token: %q", out.String())
	}
}

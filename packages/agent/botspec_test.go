package agent

import "testing"

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

package core

import "testing"

func newAliasEngine() *Engine {
	e := &Engine{
		aliases:    make(map[string]string),
		aliasExact: make(map[string]bool),
	}
	e.AddAlias("stop", "/stop")
	e.AddAlias("bind", "/workspace bind")
	return e
}

// "stop <message>" halts the running turn and redirects it in one go, so the
// trigger has to keep the rest of the line as arguments. Capitalisation is
// incidental — a word at the start of a sentence is routinely capitalised.
func TestResolveAliasStop(t *testing.T) {
	e := newAliasEngine()

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"bare trigger", "stop", "/stop"},
		{"trigger carries the follow-up", "stop write the design doc instead", "/stop write the design doc instead"},
		{"capitalised", "Stop", "/stop"},
		{"shouted", "STOP", "/stop"},
		{"capitalised with follow-up", "Stop do X instead", "/stop do X instead"},
		{"another alias still works", "bind gtm-6", "/workspace bind gtm-6"},
		{"mixed case on another alias", "BIND gtm-6", "/workspace bind gtm-6"},

		// Only the first word triggers, so ordinary sentences are untouched.
		{"trigger mid-sentence", "please stop", "please stop"},
		{"trigger as a word prefix", "stopwatch settings", "stopwatch settings"},
		{"unrelated text", "hello there", "hello there"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := e.resolveAlias(c.in); got != c.want {
				t.Errorf("resolveAlias(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Registration and removal both normalise, so a trigger written with capitals
// in config or via /alias behaves the same as a lowercase one.
func TestAliasRegistrationIsCaseInsensitive(t *testing.T) {
	e := newAliasEngine()
	e.AddAlias("SHIP", "/deploy")

	for _, in := range []string{"ship", "Ship", "SHIP"} {
		if got := e.resolveAlias(in); got != "/deploy" {
			t.Errorf("resolveAlias(%q) = %q, want /deploy", in, got)
		}
	}

	e.ClearAliases()
	if got := e.resolveAlias("stop"); got != "stop" {
		t.Fatalf("cleared aliases should not resolve, got %q", got)
	}
}

// exact = true stays available for a trigger that should never take arguments.
func TestResolveAliasExactStillSupported(t *testing.T) {
	e := &Engine{
		aliases:    make(map[string]string),
		aliasExact: make(map[string]bool),
	}
	e.AddExactAlias("ping", "/status")

	if got := e.resolveAlias("Ping"); got != "/status" {
		t.Errorf("exact alias should match case-insensitively, got %q", got)
	}
	if got := e.resolveAlias("ping the server"); got != "ping the server" {
		t.Errorf("exact alias must ignore trailing words, got %q", got)
	}
}

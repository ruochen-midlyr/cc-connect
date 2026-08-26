package core

import "testing"

// An alias normally matches the first word and carries the rest along as
// arguments. That is wrong for a trigger that is also an ordinary word: "stop"
// should interrupt the turn, but "stop writing code" is the user talking to the
// agent and must reach it verbatim.
func TestResolveAliasExact(t *testing.T) {
	e := &Engine{
		aliases:    make(map[string]string),
		aliasExact: make(map[string]bool),
	}
	e.AddExactAlias("stop", "/stop")
	e.AddAlias("bind", "/workspace bind")

	cases := []struct {
		name string
		in   string
		want string
	}{
		{"exact trigger fires", "stop", "/stop"},
		{"trigger with trailing words is left alone", "stop writing code", "stop writing code"},
		{"trigger as a prefix of a word is left alone", "stopwatch", "stopwatch"},
		{"trigger inside a sentence is left alone", "please stop", "please stop"},
		{"a normal alias still takes arguments", "bind gtm-6", "/workspace bind gtm-6"},
		{"a normal alias still fires bare", "bind", "/workspace bind"},
		{"unrelated text passes through", "hello there", "hello there"},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := e.resolveAlias(c.in); got != c.want {
				t.Errorf("resolveAlias(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

// Re-registering a trigger must carry the new matching rule, so a reload that
// flips the flag does not leave the old behaviour behind.
func TestAliasExactFlagIsReplaced(t *testing.T) {
	e := &Engine{
		aliases:    make(map[string]string),
		aliasExact: make(map[string]bool),
	}

	e.AddExactAlias("stop", "/stop")
	if got := e.resolveAlias("stop now"); got != "stop now" {
		t.Fatalf("exact alias should ignore trailing words, got %q", got)
	}

	e.AddAlias("stop", "/stop")
	if got := e.resolveAlias("stop now"); got != "/stop now" {
		t.Fatalf("after re-registering as non-exact, got %q, want /stop now", got)
	}

	e.ClearAliases()
	if got := e.resolveAlias("stop"); got != "stop" {
		t.Fatalf("cleared aliases should not resolve, got %q", got)
	}
}

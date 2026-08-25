package core

import "testing"

func newAgentCommandEngine() *Engine {
	e := &Engine{}
	e.SetAgentCommands([]AgentCommand{
		{Name: "goal", For: map[string]string{"claudecode": "/goal {args}"}},
		{Name: "loop", For: map[string]string{"claudecode": "/loop {args}"}},
		{Name: "review", For: map[string]string{
			"claudecode": "/review {args}",
			"codex":      "please review: {args}",
		}},
		{Name: "status", For: map[string]string{"claudecode": "/context"}}, // no {args}
	})
	return e
}

func TestResolveAgentCommand(t *testing.T) {
	e := newAgentCommandEngine()

	cases := []struct {
		name       string
		cmd        string
		agentType  string
		args       string
		wantText   string
		wantConfig bool
		wantSupp   bool
	}{
		{
			name: "rendered for the agent that supports it",
			cmd:  "goal", agentType: "claudecode", args: "ship the PR",
			wantText: "/goal ship the PR", wantConfig: true, wantSupp: true,
		},
		{
			name: "same word maps to different text per agent",
			cmd:  "review", agentType: "codex", args: "the diff",
			wantText: "please review: the diff", wantConfig: true, wantSupp: true,
		},
		{
			// The chat is told the agent cannot do this, rather than being
			// handed text the agent would treat as an ordinary message.
			name: "configured but unsupported by this agent",
			cmd:  "goal", agentType: "codex", args: "x",
			wantText: "", wantConfig: true, wantSupp: false,
		},
		{
			name: "unconfigured word falls through untouched",
			cmd:  "nope", agentType: "claudecode", args: "x",
			wantText: "", wantConfig: false, wantSupp: false,
		},
		{
			name: "matching ignores case and surrounding space",
			cmd:  " GOAL ", agentType: "ClaudeCode", args: "y",
			wantText: "/goal y", wantConfig: true, wantSupp: true,
		},
		{
			name: "empty args leave no trailing space",
			cmd:  "goal", agentType: "claudecode", args: "",
			wantText: "/goal", wantConfig: true, wantSupp: true,
		},
		{
			name: "a template without {args} appends them",
			cmd:  "status", agentType: "claudecode", args: "verbose",
			wantText: "/context verbose", wantConfig: true, wantSupp: true,
		},
		{
			name: "a template without {args} and no args is used as-is",
			cmd:  "status", agentType: "claudecode", args: "",
			wantText: "/context", wantConfig: true, wantSupp: true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			text, configured, supported := e.resolveAgentCommand(c.cmd, c.agentType, c.args)
			if text != c.wantText || configured != c.wantConfig || supported != c.wantSupp {
				t.Errorf("resolveAgentCommand(%q, %q, %q) = (%q, %v, %v), want (%q, %v, %v)",
					c.cmd, c.agentType, c.args, text, configured, supported,
					c.wantText, c.wantConfig, c.wantSupp)
			}
		})
	}
}

func TestAgentCommandNames(t *testing.T) {
	e := newAgentCommandEngine()
	got := e.AgentCommandNames()
	want := []string{"goal", "loop", "review", "status"}
	if len(got) != len(want) {
		t.Fatalf("AgentCommandNames() = %v, want %v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("AgentCommandNames() = %v, want %v (sorted)", got, want)
		}
	}

	e.SetAgentCommands(nil)
	if e.AgentCommandNames() != nil {
		t.Error("clearing should leave no commands configured")
	}
	if _, configured, _ := e.resolveAgentCommand("goal", "claudecode", ""); configured {
		t.Error("a cleared command must no longer resolve")
	}
}

func TestSetAgentCommandsSkipsUnnamed(t *testing.T) {
	e := &Engine{}
	e.SetAgentCommands([]AgentCommand{
		{Name: "  ", For: map[string]string{"claudecode": "/x"}},
		{Name: "ok", For: map[string]string{"claudecode": "/ok"}},
	})
	if got := e.AgentCommandNames(); len(got) != 1 || got[0] != "ok" {
		t.Fatalf("AgentCommandNames() = %v, want [ok]: an unnamed entry cannot be typed", got)
	}
}

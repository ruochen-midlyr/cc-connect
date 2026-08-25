package core

import (
	"path/filepath"
	"testing"
)

func TestSetAgentProfiles(t *testing.T) {
	e := &Engine{}

	e.SetAgentProfiles([]AgentProfile{
		{Name: "CC", Type: "claudecode"},
		{Name: " cdx ", Type: "codex"},
		{Name: "", Type: "ignored"}, // unnamed entries cannot be selected
	})

	if got := e.AgentProfileNames(); len(got) != 2 || got[0] != "cc" || got[1] != "cdx" {
		t.Fatalf("AgentProfileNames() = %v, want [cc cdx]", got)
	}

	for _, name := range []string{"cc", "CC", " cdx "} {
		if _, ok := e.lookupAgentProfile(name); !ok {
			t.Errorf("lookupAgentProfile(%q) should match case- and space-insensitively", name)
		}
	}
	if _, ok := e.lookupAgentProfile("nope"); ok {
		t.Error("lookupAgentProfile should reject an unconfigured name")
	}
	// The empty name is the project default, always valid.
	if _, ok := e.lookupAgentProfile(""); !ok {
		t.Error("the empty profile must resolve to the project default")
	}

	e.SetAgentProfiles(nil)
	if e.AgentProfileNames() != nil {
		t.Error("clearing profiles should leave none configured")
	}
}

func TestBoundAgentProfileScopes(t *testing.T) {
	e := &Engine{
		name:         "p",
		projectState: NewProjectStateStore(filepath.Join(t.TempDir(), "state.json")),
	}
	e.SetAgentProfiles([]AgentProfile{{Name: "cc"}, {Name: "cdx", Type: "codex"}})

	const (
		channel = "slack:C1"
		threadA = "slack:C1:t:100"
		threadB = "slack:C1:t:200"
	)

	// Nothing set anywhere: the project default.
	if got := e.boundAgentProfile(threadA, channel); got != "" {
		t.Fatalf("boundAgentProfile = %q, want the project default", got)
	}

	// Set once on the channel; every thread in it inherits.
	e.projectState.SetAgentProfileOverride(channel, "cdx")
	for _, key := range []string{threadA, threadB} {
		if got := e.boundAgentProfile(key, channel); got != "cdx" {
			t.Errorf("boundAgentProfile(%s) = %q, want cdx inherited from the channel", key, got)
		}
	}

	// A thread-specific choice wins over the channel's.
	e.projectState.SetAgentProfileOverride(threadA, "cc")
	if got := e.boundAgentProfile(threadA, channel); got != "cc" {
		t.Errorf("thread A = %q, want its own cc", got)
	}
	if got := e.boundAgentProfile(threadB, channel); got != "cdx" {
		t.Errorf("thread B = %q, want the channel's cdx", got)
	}

	// Clearing returns the chat to the channel default.
	e.projectState.SetAgentProfileOverride(threadA, "")
	if got := e.boundAgentProfile(threadA, channel); got != "cdx" {
		t.Errorf("after clearing, thread A = %q, want the channel's cdx", got)
	}
}

func TestBoundAgentProfileFallsBackWhenProfileIsGone(t *testing.T) {
	e := &Engine{
		name:         "p",
		projectState: NewProjectStateStore(filepath.Join(t.TempDir(), "state.json")),
	}
	e.SetAgentProfiles([]AgentProfile{{Name: "cdx", Type: "codex"}})
	e.projectState.SetAgentProfileOverride("slack:C1", "cdx")

	if got := e.boundAgentProfile("slack:C1"); got != "cdx" {
		t.Fatalf("boundAgentProfile = %q, want cdx", got)
	}

	// Dropping the profile from config must not wedge the chat.
	e.SetAgentProfiles(nil)
	if got := e.boundAgentProfile("slack:C1"); got != "" {
		t.Fatalf("boundAgentProfile = %q, want the project default after the profile was removed", got)
	}
}

// A chat that switched agents must have its commands resolved against that
// agent. Before this was wired, /model, /mode and forwarded agent commands all
// acted on the project default, so a codex chat was told about Claude Code's
// capabilities and handed Claude Code's command spellings.
func TestCommandContextUsesTheChatsAgentProfile(t *testing.T) {
	wsDir := t.TempDir()
	stateDir := t.TempDir()

	e := &Engine{
		name:              "p",
		multiWorkspace:    true,
		agent:             &stubNamedAgent{name: "claudecode"},
		sessions:          NewSessionManager(filepath.Join(stateDir, "sessions.json")),
		projectState:      NewProjectStateStore(filepath.Join(stateDir, "state.json")),
		workspaceBindings: NewWorkspaceBindingManager(filepath.Join(stateDir, "bindings.json")),
	}
	e.SetAgentProfiles([]AgentProfile{{Name: "cdx", Type: "stubagent"}})

	const channelKey = "slack:C1"
	e.workspaceBindings.Bind("project:p", channelKey, "chan", wsDir)

	// Default: the project's own agent.
	if got := e.boundAgentProfile(channelKey); got != "" {
		t.Fatalf("boundAgentProfile = %q, want the project default", got)
	}

	// After switching, the chat resolves to the profile.
	e.projectState.SetAgentProfileOverride(channelKey, "cdx")
	if got := e.boundAgentProfile("slack:C1:t:1", channelKey); got != "cdx" {
		t.Fatalf("a thread in the channel = %q, want the channel's cdx", got)
	}
}

type stubNamedAgent struct {
	Agent
	name string
}

func (s *stubNamedAgent) Name() string { return s.name }

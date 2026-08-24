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

func TestSetAgentProfileNeedsBinding(t *testing.T) {
	m := NewWorkspaceBindingManager(filepath.Join(t.TempDir(), "bindings.json"))

	if m.SetAgentProfile("project:p", "slack:C1:t:1", "cdx") {
		t.Fatal("setting an agent before a workspace is bound must fail: the choice is stored on the binding")
	}

	m.Bind("project:p", "slack:C1:t:1", "chan", "/tmp/ws")
	if !m.SetAgentProfile("project:p", "slack:C1:t:1", "cdx") {
		t.Fatal("setting an agent on a bound chat should succeed")
	}
	if b, _ := m.LookupEffective("project:p", "slack:C1:t:1"); b == nil || b.AgentProfile != "cdx" {
		t.Fatalf("AgentProfile not stored, got %+v", b)
	}
}

func TestSetAgentProfileDoesNotLeakToInheritedScopes(t *testing.T) {
	m := NewWorkspaceBindingManager(filepath.Join(t.TempDir(), "bindings.json"))

	// A channel-level binding, plus the per-thread copies the engine makes when
	// a threaded message first arrives. Lookup itself does not walk from a
	// thread up to its channel; inheritance is that copy.
	m.Bind("project:p", "slack:C1", "chan", "/tmp/ws")
	threadA := "slack:C1:t:100"
	threadB := "slack:C1:t:200"
	m.MigrateChannelKey("project:p", "slack:C1", threadA)
	m.MigrateChannelKey("project:p", "slack:C1", threadB)

	if !m.SetAgentProfile("project:p", threadA, "cdx") {
		t.Fatal("thread A should be able to pick an agent once it has a binding")
	}

	if b, _ := m.LookupEffective("project:p", threadA); b == nil || b.AgentProfile != "cdx" {
		t.Fatalf("thread A should run cdx, got %+v", b)
	}
	if b, _ := m.LookupEffective("project:p", threadB); b == nil || b.AgentProfile != "" {
		t.Fatalf("thread B must not inherit thread A's agent choice, got %+v", b)
	}
	if b, _ := m.LookupEffective("project:p", "slack:C1"); b == nil || b.AgentProfile != "" {
		t.Fatalf("the channel binding itself must be unchanged, got %+v", b)
	}
}

func TestBoundAgentProfileFallsBackWhenProfileIsGone(t *testing.T) {
	wsDir := t.TempDir()
	e := &Engine{
		name:              "p",
		multiWorkspace:    true,
		workspaceBindings: NewWorkspaceBindingManager(filepath.Join(t.TempDir(), "bindings.json")),
	}
	e.SetAgentProfiles([]AgentProfile{{Name: "cdx", Type: "codex"}})

	key := "slack:C1:t:1"
	e.workspaceBindings.Bind("project:p", key, "chan", wsDir)
	e.workspaceBindings.SetAgentProfile("project:p", key, "cdx")

	if got := e.boundAgentProfile(key); got != "cdx" {
		t.Fatalf("boundAgentProfile = %q, want cdx", got)
	}

	// Dropping the profile from config must not wedge the chat.
	e.SetAgentProfiles(nil)
	if got := e.boundAgentProfile(key); got != "" {
		t.Fatalf("boundAgentProfile = %q, want the project default after the profile was removed", got)
	}
}

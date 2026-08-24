package main

import (
	"testing"

	"github.com/chenhg5/cc-connect/core"
)

func TestChannelKeys(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"C09ABC", []string{"c09abc"}},
		{"#codex", []string{"#codex", "codex"}},
		{"  #Claude-Code  ", []string{"#claude-code", "claude-code"}},
		{"codex", []string{"codex"}},
		{"", nil},
		{"   ", nil},
		{"#", []string{"#"}}, // nothing left after the prefix
	}
	for _, c := range cases {
		got := channelKeys(c.raw)
		if len(got) != len(c.want) {
			t.Errorf("channelKeys(%q) = %v, want %v", c.raw, got, c.want)
			continue
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Errorf("channelKeys(%q) = %v, want %v", c.raw, got, c.want)
				break
			}
		}
	}
}

func TestSharedPlatformRegister(t *testing.T) {
	claude := &core.Engine{}
	codex := &core.Engine{}

	t.Run("channels route to their project", func(t *testing.T) {
		sp := newSharedPlatform("slack-main", nil)
		if err := sp.register(claude, "claude-code", []string{"#claude-code", "C001"}); err != nil {
			t.Fatalf("register claude: %v", err)
		}
		if err := sp.register(codex, "codex", []string{"#codex"}); err != nil {
			t.Fatalf("register codex: %v", err)
		}
		if sp.byChannel["claude-code"] != claude || sp.byChannel["c001"] != claude {
			t.Error("claude-code channels did not route to the claude engine")
		}
		if sp.byChannel["codex"] != codex {
			t.Error("#codex did not route to the codex engine")
		}
		if sp.fallback != nil {
			t.Error("no project asked to be the catch-all")
		}
	})

	t.Run("first registered project owns lifecycle", func(t *testing.T) {
		sp := newSharedPlatform("slack-main", nil)
		_ = sp.register(claude, "claude-code", []string{"#claude-code"})
		_ = sp.register(codex, "codex", []string{"#codex"})
		if sp.owner != claude {
			t.Error("lifecycle owner should be the first project to register")
		}
	})

	t.Run("empty channel list becomes the catch-all", func(t *testing.T) {
		sp := newSharedPlatform("slack-main", nil)
		if err := sp.register(claude, "claude-code", nil); err != nil {
			t.Fatalf("register: %v", err)
		}
		if sp.fallback != claude {
			t.Error("project with no channels should be the fallback")
		}
	})

	t.Run("two catch-alls are rejected", func(t *testing.T) {
		sp := newSharedPlatform("slack-main", nil)
		_ = sp.register(claude, "claude-code", nil)
		err := sp.register(codex, "codex", nil)
		if err == nil {
			t.Fatal("a second catch-all must be rejected: routing would be ambiguous")
		}
	})

	t.Run("a channel claimed twice is rejected", func(t *testing.T) {
		sp := newSharedPlatform("slack-main", nil)
		_ = sp.register(claude, "claude-code", []string{"#shared"})
		err := sp.register(codex, "codex", []string{"#shared"})
		if err == nil {
			t.Fatal("one channel cannot belong to two projects")
		}
	})

	t.Run("re-registering the same engine is allowed", func(t *testing.T) {
		sp := newSharedPlatform("slack-main", nil)
		_ = sp.register(claude, "claude-code", []string{"#a"})
		if err := sp.register(claude, "claude-code", []string{"#a", "#b"}); err != nil {
			t.Fatalf("same engine reclaiming its own channel: %v", err)
		}
	})
}

func TestBuildSharedPlatforms(t *testing.T) {
	t.Run("no declarations yields nothing", func(t *testing.T) {
		got, err := buildSharedPlatforms("/tmp", nil)
		if err != nil || got != nil {
			t.Fatalf("got (%v, %v), want (nil, nil)", got, err)
		}
	})

	t.Run("missing id is rejected", func(t *testing.T) {
		_, err := buildSharedPlatforms("/tmp", []sharedPlatformSpec{{Type: "slack"}})
		if err == nil {
			t.Fatal("a shared platform without an id cannot be referenced")
		}
	})

	t.Run("duplicate id is rejected", func(t *testing.T) {
		_, err := buildSharedPlatforms("/tmp", []sharedPlatformSpec{
			{ID: "slack-main", Type: "mock"},
			{ID: "slack-main", Type: "mock"},
		})
		if err == nil {
			t.Fatal("platform_ref would be ambiguous with a duplicate id")
		}
	})

	t.Run("unknown type is rejected", func(t *testing.T) {
		_, err := buildSharedPlatforms("/tmp", []sharedPlatformSpec{{ID: "x", Type: "not-a-platform"}})
		if err == nil {
			t.Fatal("an unknown platform type must fail at startup, not at first message")
		}
	})
}

func TestSharedPlatformStartWithoutProjects(t *testing.T) {
	sp := newSharedPlatform("slack-main", nil)
	if err := sp.start(); err == nil {
		t.Fatal("starting a shared platform no project references must fail")
	}
}

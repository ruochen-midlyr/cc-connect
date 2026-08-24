package main

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/chenhg5/cc-connect/core"
)

// sharedPlatform is one platform connection fronting several projects, with
// incoming messages routed to a project by the channel they arrived in.
//
// Each project normally builds its own platform instance. Pointing two projects
// at the same bot that way opens two connections for one app, and a platform
// that load-balances events across connections — Slack Socket Mode does —
// delivers each event to only one of them, so messages for a channel regularly
// arrive at the project that does not own it and are dropped. Constructing the
// connection once and dispatching by channel removes that race.
type sharedPlatform struct {
	id       string
	platform core.Platform

	mu sync.RWMutex
	// byChannel maps a channel identifier to its project engine. Both the raw
	// configured string and the '#'-stripped form are registered, so an entry
	// resolves whether the user wrote an ID or a channel name.
	byChannel map[string]*core.Engine
	// fallback handles channels no project claims, including DMs. Optional.
	fallback *core.Engine
	// owner receives platform lifecycle callbacks. A platform exposes a single
	// lifecycle handler, so one engine has to take it on behalf of the rest.
	owner *core.Engine
	// nameCache memoizes channel ID to channel name lookups, which cost an API
	// call on most platforms.
	nameCache map[string]string
}

func newSharedPlatform(id string, p core.Platform) *sharedPlatform {
	return &sharedPlatform{
		id:        id,
		platform:  p,
		byChannel: make(map[string]*core.Engine),
		nameCache: make(map[string]string),
	}
}

// register claims a set of channels for an engine. An empty channel list makes
// the engine the fallback for every unclaimed channel; only one project per
// shared platform may do that.
func (sp *sharedPlatform) register(eng *core.Engine, project string, channels []string) error {
	sp.mu.Lock()
	defer sp.mu.Unlock()

	if sp.owner == nil {
		sp.owner = eng
	}

	if len(channels) == 0 {
		if sp.fallback != nil {
			return fmt.Errorf("shared platform %q: project %q wants to be the catch-all but another project already is; give one of them an explicit channels list", sp.id, project)
		}
		sp.fallback = eng
		return nil
	}

	for _, raw := range channels {
		for _, key := range channelKeys(raw) {
			if prev, taken := sp.byChannel[key]; taken && prev != eng {
				return fmt.Errorf("shared platform %q: channel %q is claimed by more than one project", sp.id, raw)
			}
			sp.byChannel[key] = eng
		}
	}
	return nil
}

// channelKeys expands one configured channel entry into the lookup forms it
// should match: the value as written and, for "#name", the bare name.
func channelKeys(raw string) []string {
	v := strings.ToLower(strings.TrimSpace(raw))
	if v == "" {
		return nil
	}
	keys := []string{v}
	if stripped := strings.TrimPrefix(v, "#"); stripped != v && stripped != "" {
		keys = append(keys, stripped)
	}
	return keys
}

// route is the platform's message handler. It resolves the owning engine for the
// message's channel and hands the message over unchanged.
func (sp *sharedPlatform) route(p core.Platform, msg *core.Message) {
	channelID := core.MessageChannelID(msg)

	sp.mu.RLock()
	eng, ok := sp.byChannel[strings.ToLower(channelID)]
	needName := !ok && len(sp.byChannel) > 0
	fallback := sp.fallback
	sp.mu.RUnlock()

	// Channels configured by name only match after resolving the ID, which
	// costs an API call, so try it just when an ID lookup missed.
	if needName {
		if name := sp.channelName(p, channelID); name != "" {
			sp.mu.RLock()
			eng, ok = sp.byChannel[strings.ToLower(name)]
			sp.mu.RUnlock()
		}
	}

	if !ok {
		eng = fallback
	}
	if eng == nil {
		slog.Debug("shared platform: no project routes this channel, dropping message",
			"shared_platform", sp.id, "channel", channelID)
		return
	}
	eng.HandleMessage(p, msg)
}

func (sp *sharedPlatform) channelName(p core.Platform, channelID string) string {
	if channelID == "" {
		return ""
	}
	sp.mu.RLock()
	cached, hit := sp.nameCache[channelID]
	sp.mu.RUnlock()
	if hit {
		return cached
	}

	resolver, ok := p.(core.ChannelNameResolver)
	if !ok {
		return ""
	}
	name, err := resolver.ResolveChannelName(channelID)
	if err != nil {
		slog.Debug("shared platform: channel name lookup failed",
			"shared_platform", sp.id, "channel", channelID, "error", err)
		name = "" // cache the miss so a broken lookup is not retried per message
	}

	sp.mu.Lock()
	sp.nameCache[channelID] = name
	sp.mu.Unlock()
	return name
}

// start brings the connection up once, after every project that uses it has
// registered, so the first delivered message already has somewhere to go.
func (sp *sharedPlatform) start() error {
	sp.mu.RLock()
	owner := sp.owner
	routed := len(sp.byChannel)
	hasFallback := sp.fallback != nil
	sp.mu.RUnlock()

	if owner == nil {
		return fmt.Errorf("shared platform %q: no project references it", sp.id)
	}
	if async, ok := sp.platform.(core.AsyncRecoverablePlatform); ok {
		async.SetLifecycleHandler(owner)
	}
	if err := sp.platform.Start(sp.route); err != nil {
		return fmt.Errorf("start shared platform %q: %w", sp.id, err)
	}
	slog.Info("shared platform started",
		"shared_platform", sp.id, "type", sp.platform.Name(),
		"routed_channels", routed, "has_fallback", hasFallback)
	return nil
}

func (sp *sharedPlatform) stop() error {
	return sp.platform.Stop()
}

// buildSharedPlatforms constructs each declared shared platform once. Projects
// attach to them later by platform_ref.
func buildSharedPlatforms(dataDir string, cfgs []sharedPlatformSpec) (map[string]*sharedPlatform, error) {
	if len(cfgs) == 0 {
		return nil, nil
	}
	out := make(map[string]*sharedPlatform, len(cfgs))
	for _, sc := range cfgs {
		id := strings.TrimSpace(sc.ID)
		if id == "" {
			return nil, fmt.Errorf("shared platform: id is required")
		}
		if _, dup := out[id]; dup {
			return nil, fmt.Errorf("shared platform %q declared twice", id)
		}
		opts := make(map[string]any, len(sc.Options)+2)
		for k, v := range sc.Options {
			opts[k] = v
		}
		opts["cc_data_dir"] = dataDir
		// Platforms that persist state key it by project; a shared connection
		// belongs to no single project, so its own id is the stable label.
		opts["cc_project"] = id

		p, err := core.CreatePlatform(sc.Type, opts)
		if err != nil {
			return nil, fmt.Errorf("shared platform %q: %w", id, err)
		}
		out[id] = newSharedPlatform(id, p)
	}
	return out, nil
}

// sharedPlatformSpec mirrors config.SharedPlatformConfig without importing the
// config package into this file's signature, keeping the wiring testable.
type sharedPlatformSpec struct {
	ID      string
	Type    string
	Options map[string]any
}

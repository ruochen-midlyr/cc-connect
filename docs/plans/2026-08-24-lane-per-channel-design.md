# Lane-per-Channel Design

## Overview

Let one Slack app serve several projects, so a channel selects **which agent** runs
(Claude Code, Codex, a proxied Claude lane, …) while a thread selects **which workspace**
it runs in.

```
#claude-code      thread 1 → ~/workspace/midlyr-1
                  thread 2 → ~/workspace/midlyr-3

#codex            thread 1 → ~/workspace/midlyr-2

#claude-code-or   thread 1 → ~/workspace/midlyr-6
```

Today this is impossible for two independent reasons, addressed as two separable changes.

## Problem 1 — one bot cannot serve two projects

`cmd/cc-connect/main.go` builds a platform instance per project, and `Engine.Start`
attaches that engine as the platform's handler:

```go
for _, pc := range proj.Platforms {          // main.go
    p, err := core.CreatePlatform(pc.Type, opts)
}
...
p.Start(e.handleMessage)                     // engine.go
```

Two projects therefore open two Socket Mode connections for the same Slack app. Slack
distributes each event to exactly one connection, so roughly half the messages are
delivered to the project that does not own the channel and are silently dropped.

### Change

Allow projects to share one platform instance, dispatching by channel.

```toml
[[shared_platforms]]
id = "slack-main"
type = "slack"
[shared_platforms.options]
bot_token = "xoxb-..."
app_token = "xapp-..."
session_scope = "thread"

[[projects]]
name = "claude-code"
platform_ref = "slack-main"
channels = ["#claude-code"]
mode = "multi-workspace"
base_dir = "/Users/me/workspace"
[projects.agent]
type = "claudecode"

[[projects]]
name = "codex"
platform_ref = "slack-main"
channels = ["#codex"]
mode = "multi-workspace"
base_dir = "/Users/me/workspace"
[projects.agent]
type = "codex"
[projects.agent.options]
mode = "full-auto"

[[projects]]
name = "claude-code-or"
platform_ref = "slack-main"
channels = ["#claude-code-or"]
mode = "multi-workspace"
base_dir = "/Users/me/workspace"
[projects.agent]
type = "claudecode"
[projects.agent.options]
cmd = "/Users/me/cliproxyapi/claude-azure.sh"
```

A shared platform is constructed once and started once with a routing handler:

```go
router := func(p core.Platform, msg *core.Message) {
    eng := routes[extractChannelID(msg.SessionKey)]   // falls back to a default project
    if eng == nil { /* reply: channel not routed */ ; return }
    eng.HandleMessage(p, msg)
}
p.Start(router)
```

Engines keep a reference to the shared platform for replies; `ReplyCtx` already carries
channel and timestamp, so no reply-path change is needed.

Existing single-project configs are untouched: when `platform_ref` is absent the current
per-project construction path runs unchanged.

**Open item.** `AsyncRecoverablePlatform.SetLifecycleHandler` accepts one handler. For a
shared platform it needs a multiplexer, or the shared platform designates one owning
engine for lifecycle events.

## Problem 2 — workspace binding is channel-scoped, not thread-scoped

`effectiveWorkspaceChannelKey` prefers `msg.ChannelKey`, and falls back to parsing the
session key, which drops the thread segment:

```go
parts := strings.SplitN(sessionKey, ":", 4)   // [slack, C0…, t, 1787…]
return parts[1]                               // channel; thread discarded
```

The Feishu adapter already populates `ChannelKey` so that a topic gets its own binding
(`feishu.go:1124`). The Slack adapter never sets the field — `ChannelKey` appears zero
times in `platform/slack/`.

### Change

Mirror Feishu in the Slack adapter, gated on `session_scope = "thread"`:

```go
if p.sessionScope == "thread" && threadTS != "" {
    msg.ChannelKey = ev.Channel + ":t:" + threadTS
    msg.LegacyChannelKey = ev.Channel
}
```

`LegacyChannelKey` makes the engine migrate an existing channel-level binding to the
thread key on first use, so current deployments keep working.

### Command entry point

Slack slash-command payloads carry no `thread_ts`, so `/workspace bind` invoked as a
Slack slash command cannot know its thread — `slack.go` already passes `""` for the
thread when handling `EventTypeSlashCommand`. This is a Slack platform limitation, not
something the adapter can fix.

Binding must therefore arrive as an ordinary message. The existing alias mechanism
covers this, because `resolveAlias` runs before command dispatch:

```toml
[[aliases]]
name = "bind"
command = "/workspace bind"
```

The user types `bind midlyr-3` as plain text in the thread; it is rewritten to
`/workspace bind midlyr-3` and dispatched with the thread context intact.

### Unbound threads

A thread with no binding is an error, not a fallback:

> No workspace bound to this thread. Send `bind <name>` first.

Silently falling back to a channel default risks running an agent against the wrong
repository, which is unacceptable when agents run with permissive modes.

## Out of scope

- Switching agent or workspace mid-thread. The pair is fixed for the thread's session:
  a different agent has a different session store, and a different work_dir is a
  different agent instance, so neither can be resumed across the switch.
- Per-thread model overrides. `WorkspaceModelOverride` already exists at workspace scope.

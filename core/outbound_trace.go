package core

import (
	"fmt"
	"log/slog"
	"runtime"
	"strings"
)

// TEMPORARY DIAGNOSTIC — remove once the duplicate-delivery investigation is
// closed. cc-connect logs nothing at the point a message leaves for a platform,
// so when the same text arrives twice in a chat there is no way to tell which
// code paths produced it. These helpers name the caller.

// outboundCallers renders the engine frames above the send helpers, nearest
// first, so a log line says which branch delivered the text.
func outboundCallers() string {
	pcs := make([]uintptr, 12)
	// Skip runtime.Callers, this function, and the send helper that called it.
	n := runtime.Callers(3, pcs)
	if n == 0 {
		return "?"
	}
	frames := runtime.CallersFrames(pcs[:n])
	var out []string
	for len(out) < 4 {
		f, more := frames.Next()
		name := f.Function
		if i := strings.LastIndex(name, "/"); i >= 0 {
			name = name[i+1:]
		}
		// The send helpers call each other; they are noise here.
		switch {
		case strings.Contains(name, "sendWithError"),
			strings.Contains(name, "sendAlreadyRendered"),
			strings.Contains(name, "replyWithError"),
			strings.Contains(name, ".send"),
			strings.Contains(name, ".reply"):
		default:
			out = append(out, fmt.Sprintf("%s:%d", name, f.Line))
		}
		if !more {
			break
		}
	}
	return strings.Join(out, " < ")
}

// logOutbound records one delivery attempt with enough of the payload to match
// it against what the chat actually shows.
func logOutbound(kind, platform, content string) {
	head := content
	if len(head) > 60 {
		head = head[:60]
	}
	slog.Info("outbound",
		"kind", kind,
		"platform", platform,
		"len", len(content),
		"head", strings.ReplaceAll(head, "\n", "\\n"),
		"from", outboundCallers(),
	)
}

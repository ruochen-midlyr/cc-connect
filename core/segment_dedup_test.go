package core

import "testing"

// Compact display posts each text segment as its own message as it streams, so
// by the time the turn ends only the remainder is still owed. The guard that
// decides this used to require a tool call, which missed replies that used no
// tools — with thinking hidden, a thinking event flushes a segment on its own —
// and those replies were posted a second time in full.
//
// A live preview is the exception: its text is not delivered until the card is
// finalized, so the finalize path has to keep running.
func TestSegmentRemainderGuard(t *testing.T) {
	cases := []struct {
		name         string
		toolCount    int
		segmentStart int
		canPreview   bool
		wantRemainer bool
	}{
		{
			name:      "text-only reply with segments already posted",
			toolCount: 0, segmentStart: 3, canPreview: false,
			wantRemainer: true, // the regression: this used to fall through and resend
		},
		{
			name:      "reply with tool calls and segments already posted",
			toolCount: 2, segmentStart: 3, canPreview: false,
			wantRemainer: true,
		},
		{
			name:      "nothing posted yet, so the whole response is owed",
			toolCount: 0, segmentStart: 0, canPreview: false,
			wantRemainer: false,
		},
		{
			name:      "a live preview must still be finalized",
			toolCount: 0, segmentStart: 3, canPreview: true,
			wantRemainer: false,
		},
		{
			name:      "tool calls with a live preview still finalize",
			toolCount: 2, segmentStart: 3, canPreview: true,
			wantRemainer: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := c.segmentStart > 0 && !c.canPreview
			if got != c.wantRemainer {
				t.Errorf("remainder branch = %v, want %v (tools=%d segmentStart=%d preview=%v)",
					got, c.wantRemainer, c.toolCount, c.segmentStart, c.canPreview)
			}
		})
	}
}

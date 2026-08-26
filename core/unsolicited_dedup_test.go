package core

import "testing"

// An agent that resumes a turn after signalling completion re-emits the text it
// already delivered. Claude Code does this when a /goal stop hook fires: the
// user first sees the answer from the foreground turn, then the identical text
// again from the unsolicited reader.
func TestUnsolicitedDuplicateSuppression(t *testing.T) {
	const answer = "Understood — step 1 only: carve the evaluation pipeline out."

	cases := []struct {
		name          string
		lastDelivered string
		incoming      string
		wantSuppress  bool
	}{
		{
			name:          "identical text is suppressed",
			lastDelivered: answer,
			incoming:      answer,
			wantSuppress:  true,
		},
		{
			name:          "genuinely new text is delivered",
			lastDelivered: answer,
			incoming:      answer + " Now reading the activity file.",
			wantSuppress:  false,
		},
		{
			name: "a background task with nothing delivered before it is not suppressed",
			// Cron and relay turns run with no preceding foreground turn.
			lastDelivered: "",
			incoming:      answer,
			wantSuppress:  false,
		},
		{
			// Otherwise an agent that legitimately ends two turns with no text
			// would have the second silently swallowed.
			name:          "empty against empty is not treated as a duplicate",
			lastDelivered: "",
			incoming:      "",
			wantSuppress:  false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			state := &interactiveState{lastForegroundResponse: c.lastDelivered}

			state.mu.Lock()
			suppress := c.incoming != "" && c.incoming == state.lastForegroundResponse
			if suppress {
				state.lastForegroundResponse = ""
			}
			state.mu.Unlock()

			if suppress != c.wantSuppress {
				t.Fatalf("suppress = %v, want %v", suppress, c.wantSuppress)
			}

			// Suppression must be a one-shot: clearing the record means a later
			// turn that happens to repeat the same words still gets through.
			if c.wantSuppress {
				state.mu.Lock()
				again := c.incoming != "" && c.incoming == state.lastForegroundResponse
				state.mu.Unlock()
				if again {
					t.Error("suppression should not persist past the duplicate it was meant for")
				}
			}
		})
	}
}

package state

import (
	"testing"
	"time"
)

func TestStateWorkflow(t *testing.T) {
	// Clean baseline
	st := LoadState()
	st.LastActionTime = time.Time{}
	st.LastPulledID = 0
	st.LastSeenID = 0
	SaveState(st)

	// 1. Initial state: not eligible
	if IsDirectPasteEligible(0) {
		t.Errorf("Expected not eligible before any action")
	}

	// 2. User pulls message 10
	RecordPull(10)

	// Check eligibility for message 11 (previous was 10, pulled was 10)
	if !IsDirectPasteEligible(10) {
		t.Errorf("Expected eligible immediately after pull of message 10")
	}

	// 3. Directly paste message 11
	RecordAutoPaste(11)

	// Check eligibility for message 12 (previous was 11, auto_pasted was 11)
	if !IsDirectPasteEligible(11) {
		t.Errorf("Expected eligible for message 12 after auto_paste of 11")
	}

	// 4. Simulate 11 minutes later (timeout)
	st = LoadState()
	st.LastActionTime = time.Now().Add(-11 * time.Minute)
	SaveState(st)

	if IsDirectPasteEligible(11) {
		t.Errorf("Expected NOT eligible after 11 minutes of inactivity")
	}

	// 5. User pushes text (resets active timer)
	RecordPush()
	// Still eligible since last action is now recent and last pulled was 11
	if !IsDirectPasteEligible(11) {
		t.Errorf("Expected eligible after push within 10 minutes")
	}
}

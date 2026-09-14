package state

import (
	"encoding/json"
	"os"
	"os/user"
	"path/filepath"
	"time"
)

type ClientState struct {
	LastAction     string    `json:"last_action"`      // "pull", "push", "auto_paste"
	LastActionTime time.Time `json:"last_action_time"` // When last pull or push happened
	LastPulledID   int64     `json:"last_pulled_id"`   // ID of the message last pulled
	LastSeenID     int64     `json:"last_seen_id"`     // ID of the latest message seen
}

func getStatePath() string {
	if usr, err := user.Current(); err == nil {
		return filepath.Join(usr.HomeDir, ".config", "mem", "state.json")
	}
	return "state.json"
}

func LoadState() ClientState {
	p := getStatePath()
	var st ClientState
	data, err := os.ReadFile(p)
	if err == nil {
		_ = json.Unmarshal(data, &st)
	}
	return st
}

func SaveState(st ClientState) {
	p := getStatePath()
	_ = os.MkdirAll(filepath.Dir(p), 0700)
	data, err := json.Marshal(st)
	if err == nil {
		_ = os.WriteFile(p, data, 0600)
	}
}

// RecordPull updates state when a manual pull (Super+V) occurs
func RecordPull(msgID int64) {
	st := LoadState()
	st.LastAction = "pull"
	st.LastActionTime = time.Now()
	st.LastPulledID = msgID
	if msgID > st.LastSeenID {
		st.LastSeenID = msgID
	}
	SaveState(st)
}

// RecordPush updates state when a manual push (Super+C) occurs
func RecordPush() {
	st := LoadState()
	st.LastAction = "push"
	st.LastActionTime = time.Now()
	SaveState(st)
}

// RecordAutoPaste updates state when a direct-to-screen paste occurs
func RecordAutoPaste(msgID int64) {
	st := LoadState()
	st.LastAction = "auto_paste"
	st.LastActionTime = time.Now()
	st.LastPulledID = msgID
	if msgID > st.LastSeenID {
		st.LastSeenID = msgID
	}
	SaveState(st)
}

// RecordSeen updates the last seen message ID without changing action time
func RecordSeen(msgID int64) {
	st := LoadState()
	if msgID > st.LastSeenID {
		st.LastSeenID = msgID
		SaveState(st)
	}
}

// IsDirectPasteEligible checks if a new message should be automatically pasted onto screen:
// 1. Within 10 minutes of last action (pull, push, or auto_paste)
// 2. The previous message was pulled (or this is the continuation of active session)
func IsDirectPasteEligible(prevMsgID int64) bool {
	st := LoadState()
	if st.LastActionTime.IsZero() {
		return false
	}

	// Rule 1: Must be within 10 minutes of last action
	if time.Since(st.LastActionTime) > 10*time.Minute {
		return false
	}

	// Rule 2: The previous message must have been pulled (active window in use)
	if prevMsgID > 0 && st.LastPulledID < prevMsgID {
		return false
	}

	return true
}

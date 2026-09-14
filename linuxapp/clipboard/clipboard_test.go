package clipboard

import (
	"strings"
	"testing"
)

func TestClipboardSetAndGet(t *testing.T) {
	testText := "Mem Clipboard Unit Test 123456"
	err := SetText(testText)
	if err != nil {
		t.Fatalf("SetText failed: %v", err)
	}

	got, err := GetText()
	if err != nil {
		t.Fatalf("GetText failed: %v", err)
	}

	if !strings.Contains(got, "Mem Clipboard Unit Test 123456") {
		t.Errorf("GetText got %q, want %q", got, testText)
	}
}

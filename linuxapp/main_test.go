package main

import (
	"path/filepath"
	"testing"
)

func TestLoadAndSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.json")

	cfgMeta := loadConfig(cfgPath)
	if cfgMeta.ServerURL == "" {
		t.Errorf("Expected default server URL")
	}

	cfgMeta.Token = "test_token_123"
	cfgMeta.Source = "TestUnitDevice"

	if err := saveConfig(cfgPath, cfgMeta.Config); err != nil {
		t.Fatalf("saveConfig failed: %v", err)
	}

	loaded := loadConfig(cfgPath)
	if loaded.Token != "test_token_123" {
		t.Errorf("got token %q, want 'test_token_123'", loaded.Token)
	}
	if loaded.GetSource() != "TestUnitDevice" {
		t.Errorf("got source %q, want 'TestUnitDevice'", loaded.GetSource())
	}
	if loaded.LoadedFrom != cfgPath {
		t.Errorf("got LoadedFrom %q, want %q", loaded.LoadedFrom, cfgPath)
	}
}

func TestMaskToken(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"", "******"},
		{"short", "******"},
		{"12345678", "******"},
		{"mem_aa3a3f33353e2f6af8eb0a02f37d07b31106764634c937d3", "mem_aa3a...37d3"},
	}

	for _, tc := range tests {
		got := maskToken(tc.input)
		if got != tc.want {
			t.Errorf("maskToken(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestGetDefaultConfigPath(t *testing.T) {
	p := getDefaultConfigPath()
	if p == "" {
		t.Errorf("getDefaultConfigPath() returned empty string")
	}
}

func TestPrintHelp(t *testing.T) {
	// Should not panic
	printHelp()
}

package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestParseEnvLine(t *testing.T) {
	tests := []struct {
		line, key, value string
		present          bool
	}{
		{"# comment", "", "", false}, {"export PORT=3000", "PORT", "3000", true},
		{"ADMIN_PASSWORD='value with spaces' # note", "ADMIN_PASSWORD", "value with spaces", true},
		{"OIDC_DISPLAY_NAME=OpenID Connect # note", "OIDC_DISPLAY_NAME", "OpenID Connect", true},
		{"TOKEN=abc#def", "TOKEN", "abc#def", true}, {`LABEL="line\nvalue" # note`, "LABEL", "line\nvalue", true},
	}
	for _, tt := range tests {
		key, value, present, err := parseEnvLine(tt.line)
		if err != nil {
			t.Fatalf("%q: %v", tt.line, err)
		}
		if key != tt.key || value != tt.value || present != tt.present {
			t.Fatalf("%q => %q,%q,%v", tt.line, key, value, present)
		}
	}
}

func TestLoadEnvFileDoesNotOverrideProcessEnvironment(t *testing.T) {
	path := filepath.Join(t.TempDir(), "koffan.env")
	if err := os.WriteFile(path, []byte("KOFFAN_ENV_TEST_EXISTING=from-file\nKOFFAN_ENV_TEST_NEW=loaded\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("KOFFAN_ENV_TEST_EXISTING", "from-process")
	_ = os.Unsetenv("KOFFAN_ENV_TEST_NEW")
	t.Cleanup(func() { _ = os.Unsetenv("KOFFAN_ENV_TEST_NEW") })
	loaded, err := loadEnvFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded != 1 {
		t.Fatalf("loaded=%d, want 1", loaded)
	}
	if got := os.Getenv("KOFFAN_ENV_TEST_EXISTING"); got != "from-process" {
		t.Fatalf("existing value overwritten: %q", got)
	}
	if got := os.Getenv("KOFFAN_ENV_TEST_NEW"); got != "loaded" {
		t.Fatalf("new value=%q", got)
	}
}

func TestExplicitMissingEnvFileIsAnError(t *testing.T) {
	_, _, err := loadStartupEnvironment(filepath.Join(t.TempDir(), "missing.env"))
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("error=%v", err)
	}
}

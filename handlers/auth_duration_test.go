package handlers

import (
	"testing"
	"time"
)

func TestParseSessionDuration(t *testing.T) {
	tests := []struct {
		value string
		want  time.Duration
	}{
		{value: "1d", want: 24 * time.Hour},
		{value: "7d", want: 7 * 24 * time.Hour},
		{value: "30m", want: 30 * time.Minute},
		{value: "1h30m", want: 90 * time.Minute},
	}

	for _, tt := range tests {
		t.Run(tt.value, func(t *testing.T) {
			got, err := parseSessionDuration(tt.value)
			if err != nil {
				t.Fatalf("parseSessionDuration(%q): %v", tt.value, err)
			}
			if got != tt.want {
				t.Errorf("parseSessionDuration(%q) = %s, want %s", tt.value, got, tt.want)
			}
		})
	}
}

func TestParseSessionDurationRejectsInvalidDays(t *testing.T) {
	for _, value := range []string{"1.5d", "999999999999999999999999d", "1dd", "1d12h"} {
		t.Run(value, func(t *testing.T) {
			if _, err := parseSessionDuration(value); err == nil {
				t.Errorf("parseSessionDuration(%q) succeeded; want error", value)
			}
		})
	}
}

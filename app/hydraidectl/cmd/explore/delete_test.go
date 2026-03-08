package explore

import (
	"strings"
	"testing"
)

func TestGenerateCode(t *testing.T) {
	for i := 0; i < 100; i++ {
		code := generateCode()
		if len(code) != confirmCodeLen {
			t.Fatalf("expected code length %d, got %d: %q", confirmCodeLen, len(code), code)
		}
		for _, c := range code {
			if !strings.ContainsRune("ABCDEFGHJKLMNPQRSTUVWXYZ23456789", c) {
				t.Fatalf("unexpected character in code: %c", c)
			}
		}
	}
}

func TestGenerateCodeUniqueness(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 1000; i++ {
		code := generateCode()
		seen[code] = struct{}{}
	}
	// With a 4-char code from 32 chars, there are 32^4 = ~1M possibilities.
	// 1000 codes should yield at least 950 unique codes.
	if len(seen) < 950 {
		t.Fatalf("too few unique codes: %d out of 1000", len(seen))
	}
}

func TestParseIslandID(t *testing.T) {
	tests := []struct {
		input    string
		expected uint64
	}{
		{"1", 1},
		{"42", 42},
		{"1000", 1000},
		{"0", 0},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		got := parseIslandID(tt.input)
		if got != tt.expected {
			t.Errorf("parseIslandID(%q) = %d, want %d", tt.input, got, tt.expected)
		}
	}
}

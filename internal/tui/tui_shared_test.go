package tui

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCompareIDsNatural(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		expected bool // a < b
	}{
		{"same prefix numeric order", "ac-9", "ac-10", true},
		{"same prefix numeric order reverse", "ac-10", "ac-9", false},
		{"same prefix same number", "ac-10", "ac-10", false},
		{"same prefix larger numbers", "ac-100", "ac-1000", true},
		{"different prefix falls back to lexicographic", "ab-1", "ac-1", true},
		{"no numeric suffix falls back to lexicographic", "abc", "abd", true},
		{"mixed format falls back to lexicographic", "ac-1", "bd", true},
		{"multi-dash prefix", "proj-sub-1", "proj-sub-2", true},
		{"multi-dash prefix larger", "proj-sub-10", "proj-sub-2", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := compareIDsNatural(tt.a, tt.b)
			require.Equal(t, tt.expected, result)
		})
	}
}

func TestSplitIDNumeric(t *testing.T) {
	tests := []struct {
		id     string
		prefix string
		num    int
		ok     bool
	}{
		{"ac-819", "ac-", 819, true},
		{"ac-1", "ac-", 1, true},
		{"proj-sub-42", "proj-sub-", 42, true},
		{"abc", "", 0, false},
		{"ac-", "", 0, false},
		{"ac-xyz", "", 0, false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			prefix, num, ok := splitIDNumeric(tt.id)
			require.Equal(t, tt.ok, ok)
			if ok {
				require.Equal(t, tt.prefix, prefix)
				require.Equal(t, tt.num, num)
			}
		})
	}
}

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsValidSubstituteKey(t *testing.T) {
	tests := []struct {
		key   string
		valid bool
	}{
		{"CLUSTER_NAME", true},
		{"_PRIVATE", true},
		{"a", true},
		{"A1B2C3", true},
		{"_", true},
		{"__double__", true},
		{"CLUSTER-NAME", false},   // hyphen not allowed
		{"cluster.name", false},   // dot not allowed
		{"123_START", false},      // starts with digit
		{"", false},               // empty
		{"HAS SPACE", false},      // space not allowed
		{"key/slash", false},      // slash not allowed
		{"key=value", false},      // equals not allowed
		{"café", false},           // non-ASCII not allowed
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			assert.Equal(t, tt.valid, IsValidSubstituteKey(tt.key), "key: %q", tt.key)
		})
	}
}

func TestEscapeJsonPointer(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"slash_only", "normal/path", "normal~1path"},
		{"slash_and_tilde", "path/with~tilde", "path~1with~0tilde"},
		{"multiple_slashes", "path/with/slash", "path~1with~1slash"},
		{"mixed_tilde_and_slashes", "path/with~/and/", "path~1with~0~1and~1"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := EscapeJsonPointer(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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

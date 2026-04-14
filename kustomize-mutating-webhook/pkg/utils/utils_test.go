package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEscapeJsonPointer(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"normal/path", "normal~1path"},
		{"path/with~tilde", "path~1with~0tilde"},
		{"path/with/slash", "path~1with~1slash"},
		{"path/with~/and/", "path~1with~0~1and~1"},
	}

	for _, test := range tests {
		result := EscapeJsonPointer(test.input)
		assert.Equal(t, test.expected, result)
	}
}

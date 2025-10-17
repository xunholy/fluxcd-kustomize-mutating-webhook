package utils

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadConfigMap(t *testing.T) {
	// Create a temporary directory for testing
	tempDir, err := ioutil.TempDir("", "config-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	// Create test config files
	err = ioutil.WriteFile(filepath.Join(tempDir, "key1"), []byte("value1"), 0644)
	require.NoError(t, err)
	err = ioutil.WriteFile(filepath.Join(tempDir, "key2"), []byte("value2"), 0644)
	require.NoError(t, err)

	// Test reading config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)

	assert.Equal(t, "value1", AppConfig.Config["key1"])
	assert.Equal(t, "value2", AppConfig.Config["key2"])

	// Test reading from empty directory
	emptyDir, err := ioutil.TempDir("", "empty-config-test")
	require.NoError(t, err)
	defer os.RemoveAll(emptyDir)

	err = ReadConfigDirectory(emptyDir)
	assert.Error(t, err)
	assert.Equal(t, "no configuration found", err.Error())
}

func TestGetAppConfig(t *testing.T) {
	AppConfig.Config = map[string]string{
		"key1": "value1",
		"key2": "value2",
	}

	value, ok := GetAppConfig("key1")
	assert.True(t, ok)
	assert.Equal(t, "value1", value)

	value, ok = GetAppConfig("nonexistent")
	assert.False(t, ok)
	assert.Empty(t, value)
}

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

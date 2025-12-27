package utils

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewConfigWatcher(t *testing.T) {
	tempDir := t.TempDir()

	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	require.NotNil(t, watcher)
	assert.Equal(t, tempDir, watcher.configDir)
	assert.NotNil(t, watcher.watcher)
	assert.NotNil(t, watcher.done)

	// Clean up
	watcher.Stop()
}

func TestConfigWatcher_Watch_FileModify(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial config file
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("initial-value"), 0644)
	require.NoError(t, err)

	// Load initial config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)
	assert.Equal(t, "initial-value", AppConfig.Config["key1"])

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Start watching in goroutine
	go func() {
		_ = watcher.Watch()
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Modify the file
	err = os.WriteFile(filepath.Join(tempDir, "key1"), []byte("updated-value"), 0644)
	require.NoError(t, err)

	// Wait for reload to happen (100ms debounce + processing time)
	time.Sleep(200 * time.Millisecond)

	// Verify config was reloaded
	AppConfig.Mu.RLock()
	value := AppConfig.Config["key1"]
	AppConfig.Mu.RUnlock()
	assert.Equal(t, "updated-value", value)
}

func TestConfigWatcher_Watch_FileCreate(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial config file
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("value1"), 0644)
	require.NoError(t, err)

	// Load initial config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)
	assert.Len(t, AppConfig.Config, 1)

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Start watching in goroutine
	go func() {
		_ = watcher.Watch()
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Create a new file
	err = os.WriteFile(filepath.Join(tempDir, "key2"), []byte("value2"), 0644)
	require.NoError(t, err)

	// Wait for reload to happen
	time.Sleep(200 * time.Millisecond)

	// Verify new key was loaded
	AppConfig.Mu.RLock()
	configLen := len(AppConfig.Config)
	value2 := AppConfig.Config["key2"]
	AppConfig.Mu.RUnlock()
	assert.Equal(t, 2, configLen)
	assert.Equal(t, "value2", value2)
}

func TestConfigWatcher_Watch_FileDelete(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial config files
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("value1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "key2"), []byte("value2"), 0644)
	require.NoError(t, err)

	// Load initial config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)
	assert.Len(t, AppConfig.Config, 2)

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Start watching in goroutine
	go func() {
		_ = watcher.Watch()
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Delete one file
	err = os.Remove(filepath.Join(tempDir, "key2"))
	require.NoError(t, err)

	// Wait for reload to happen
	time.Sleep(200 * time.Millisecond)

	// Verify config was reloaded with only one key
	AppConfig.Mu.RLock()
	configLen := len(AppConfig.Config)
	_, key2Exists := AppConfig.Config["key2"]
	value1 := AppConfig.Config["key1"]
	AppConfig.Mu.RUnlock()
	assert.Equal(t, 1, configLen)
	assert.False(t, key2Exists)
	assert.Equal(t, "value1", value1)
}

func TestConfigWatcher_ErrorHandling(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial config file
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("value1"), 0644)
	require.NoError(t, err)

	// Load initial config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)
	initialValue := AppConfig.Config["key1"]

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Start watching in goroutine
	go func() {
		_ = watcher.Watch()
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Remove all config files to trigger error
	err = os.Remove(filepath.Join(tempDir, "key1"))
	require.NoError(t, err)

	// Wait for reload attempt
	time.Sleep(200 * time.Millisecond)

	// Verify old config is preserved (reload should fail with "no configuration found")
	AppConfig.Mu.RLock()
	value := AppConfig.Config["key1"]
	AppConfig.Mu.RUnlock()
	assert.Equal(t, initialValue, value, "Config should be preserved when reload fails")
}

func TestConfigWatcher_Stop(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial config file
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("value1"), 0644)
	require.NoError(t, err)

	// Create watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)

	// Start watching in goroutine
	watchDone := make(chan struct{})
	go func() {
		_ = watcher.Watch()
		close(watchDone)
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Stop the watcher
	watcher.Stop()

	// Verify Watch() returns (with timeout)
	select {
	case <-watchDone:
		// Success - Watch() returned
	case <-time.After(1 * time.Second):
		t.Fatal("Watch() did not exit after Stop() was called")
	}
}

func TestConfigWatcher_Debouncing(t *testing.T) {
	tempDir := t.TempDir()

	// Create initial config file
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("initial"), 0644)
	require.NoError(t, err)

	// Load initial config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Start watching in goroutine
	go func() {
		_ = watcher.Watch()
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Trigger multiple rapid file modifications
	// The 100ms debounce should handle these efficiently
	for i := 0; i < 5; i++ {
		err = os.WriteFile(filepath.Join(tempDir, "key1"), []byte("updated"), 0644)
		require.NoError(t, err)
		time.Sleep(10 * time.Millisecond)
	}

	// Wait for debouncing and reload (100ms debounce + processing time)
	time.Sleep(300 * time.Millisecond)

	// Verify config was eventually updated
	AppConfig.Mu.RLock()
	value := AppConfig.Config["key1"]
	AppConfig.Mu.RUnlock()
	assert.Equal(t, "updated", value, "Config should be updated despite rapid changes")
}

func TestConfigWatcher_HiddenFiles(t *testing.T) {
	tempDir := t.TempDir()

	// Create regular and hidden files
	err := os.WriteFile(filepath.Join(tempDir, "key1"), []byte("value1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, ".hidden"), []byte("hidden-value"), 0644)
	require.NoError(t, err)

	// Load config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Verify only non-hidden file is loaded
	AppConfig.Mu.RLock()
	configLen := len(AppConfig.Config)
	_, hiddenExists := AppConfig.Config[".hidden"]
	value1 := AppConfig.Config["key1"]
	AppConfig.Mu.RUnlock()

	assert.Equal(t, 1, configLen)
	assert.False(t, hiddenExists, "Hidden files should not be loaded")
	assert.Equal(t, "value1", value1)
}

func TestConfigWatcher_MixedConfigMapAndSecret(t *testing.T) {
	tempDir := t.TempDir()

	// Simulate Kubernetes mounting pattern with ConfigMap and Secret
	// In K8s, these would be mounted from different sources but appear as regular files

	// Create files that simulate ConfigMap keys
	err := os.WriteFile(filepath.Join(tempDir, "configmap-key1"), []byte("configmap-value1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "configmap-key2"), []byte("configmap-value2"), 0644)
	require.NoError(t, err)

	// Create files that simulate Secret keys (secrets are just files too)
	err = os.WriteFile(filepath.Join(tempDir, "secret-key1"), []byte("secret-value1"), 0644)
	require.NoError(t, err)
	err = os.WriteFile(filepath.Join(tempDir, "db-password"), []byte("super-secret-password"), 0600)
	require.NoError(t, err)

	// Load initial config
	err = ReadConfigDirectory(tempDir)
	require.NoError(t, err)

	// Verify all files loaded (both "ConfigMap" and "Secret" files)
	AppConfig.Mu.RLock()
	assert.Len(t, AppConfig.Config, 4)
	assert.Equal(t, "configmap-value1", AppConfig.Config["configmap-key1"])
	assert.Equal(t, "secret-value1", AppConfig.Config["secret-key1"])
	assert.Equal(t, "super-secret-password", AppConfig.Config["db-password"])
	AppConfig.Mu.RUnlock()

	// Create and start watcher
	watcher, err := NewConfigWatcher(tempDir, false, nil)
	require.NoError(t, err)
	defer watcher.Stop()

	// Start watching in goroutine
	go func() {
		_ = watcher.Watch()
	}()

	// Give watcher time to start
	time.Sleep(50 * time.Millisecond)

	// Simulate Secret rotation (update the secret file)
	err = os.WriteFile(filepath.Join(tempDir, "db-password"), []byte("new-rotated-password"), 0600)
	require.NoError(t, err)

	// Wait for reload
	time.Sleep(200 * time.Millisecond)

	// Verify Secret was reloaded
	AppConfig.Mu.RLock()
	updatedPassword := AppConfig.Config["db-password"]
	configMapValue := AppConfig.Config["configmap-key1"]
	AppConfig.Mu.RUnlock()

	assert.Equal(t, "new-rotated-password", updatedPassword, "Secret should be hot-reloaded")
	assert.Equal(t, "configmap-value1", configMapValue, "ConfigMap values should remain unchanged")

	// Simulate adding a new Secret key
	err = os.WriteFile(filepath.Join(tempDir, "api-token"), []byte("new-api-token"), 0600)
	require.NoError(t, err)

	// Wait for reload
	time.Sleep(200 * time.Millisecond)

	// Verify new Secret key was added
	AppConfig.Mu.RLock()
	apiToken := AppConfig.Config["api-token"]
	configLen := len(AppConfig.Config)
	AppConfig.Mu.RUnlock()

	assert.Equal(t, "new-api-token", apiToken, "New Secret key should be loaded")
	assert.Equal(t, 5, configLen, "Total config should include all ConfigMap and Secret keys")
}

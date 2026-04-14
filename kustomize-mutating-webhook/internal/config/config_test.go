package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("SERVER_ADDRESS", ":9443")
	t.Setenv("CERT_FILE", "/custom/cert/path")
	t.Setenv("RATE_LIMIT", "200")

	config := LoadConfig()

	assert.Equal(t, ":9443", config.ServerAddress)
	assert.Equal(t, "/custom/cert/path", config.CertFile)
	assert.Equal(t, defaultKeyFile, config.KeyFile)
	assert.Equal(t, defaultLogLevel, config.LogLevel)
	assert.Equal(t, 200, config.RateLimit)
}

func TestValidateConfig(t *testing.T) {
	tests := []struct {
		name        string
		config      Config
		expectedErr string
	}{
		{
			name: "Valid configuration",
			config: Config{
				ServerAddress: ":8443",
				CertFile:      "/path/to/cert",
				KeyFile:       "/path/to/key",
				LogLevel:      "info",
				RateLimit:     100,
			},
			expectedErr: "",
		},
		{
			name: "Missing server address",
			config: Config{
				CertFile:  "/path/to/cert",
				KeyFile:   "/path/to/key",
				LogLevel:  "info",
				RateLimit: 100,
			},
			expectedErr: "server address is required",
		},
		{
			name: "Invalid rate limit",
			config: Config{
				ServerAddress: ":8443",
				CertFile:      "/path/to/cert",
				KeyFile:       "/path/to/key",
				LogLevel:      "info",
				RateLimit:     0,
			},
			expectedErr: "rate limit must be greater than 0",
		},
		{
			name: "Missing cert file",
			config: Config{
				ServerAddress: ":8443",
				CertFile:      "",
				KeyFile:       "/path",
				RateLimit:     100,
			},
			expectedErr: "certificate file path is required",
		},
		{
			name: "Missing key file",
			config: Config{
				ServerAddress: ":8443",
				CertFile:      "/path",
				KeyFile:       "",
				RateLimit:     100,
			},
			expectedErr: "key file path is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateConfig(tt.config)
			if tt.expectedErr == "" {
				assert.NoError(t, err)
			} else {
				assert.EqualError(t, err, tt.expectedErr)
			}
		})
	}
}

func TestInitLogger(t *testing.T) {
	originalLevel := zerolog.GlobalLevel()
	t.Cleanup(func() { zerolog.SetGlobalLevel(originalLevel) })

	tests := []struct {
		name          string
		logLevel      string
		expectedLevel string
	}{
		{"Debug level", "debug", "debug"},
		{"Info level", "info", "info"},
		{"Warn level", "warn", "warn"},
		{"Error level", "error", "error"},
		{"Invalid level", "invalid", "info"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			InitLogger(tt.logLevel)
			assert.Equal(t, tt.expectedLevel, zerolog.GlobalLevel().String())
		})
	}
}

func TestDetectNamespace(t *testing.T) {
	original := ServiceAccountNamespaceFile
	t.Cleanup(func() { ServiceAccountNamespaceFile = original })

	t.Run("Returns configured value when non-empty", func(t *testing.T) {
		ns := DetectNamespace("my-namespace")
		assert.Equal(t, "my-namespace", ns)
	})

	t.Run("Reads namespace from service account file when configured is empty", func(t *testing.T) {
		dir := t.TempDir()
		nsFile := filepath.Join(dir, "namespace")
		if err := os.WriteFile(nsFile, []byte("  kube-system\n"), 0644); err != nil {
			t.Fatal(err)
		}
		ServiceAccountNamespaceFile = nsFile
		ns := DetectNamespace("")
		assert.Equal(t, "kube-system", ns)
	})

	t.Run("Returns flux-system when configured is empty and file does not exist", func(t *testing.T) {
		ServiceAccountNamespaceFile = "/nonexistent/path/namespace"
		ns := DetectNamespace("")
		assert.Equal(t, "flux-system", ns)
	})
}

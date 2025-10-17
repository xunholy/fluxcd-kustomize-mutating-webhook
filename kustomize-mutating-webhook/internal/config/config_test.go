package config

import (
	"os"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/stretchr/testify/assert"
)

func TestLoadConfig(t *testing.T) {
	// Save current environment and restore it after the test
	oldEnv := os.Environ()
	t.Cleanup(func() {
		for _, env := range oldEnv {
			pair := strings.SplitN(env, "=", 2)
			os.Setenv(pair[0], pair[1])
		}
	})

	// Set test environment variables
	os.Setenv("SERVER_ADDRESS", ":9443")
	os.Setenv("CERT_FILE", "/custom/cert/path")
	os.Setenv("RATE_LIMIT", "200")

	config := LoadConfig()

	assert.Equal(t, ":9443", config.ServerAddress)
	assert.Equal(t, "/custom/cert/path", config.CertFile)
	assert.Equal(t, defaultKeyFile, config.KeyFile)
	assert.Equal(t, defaultConfigDir, config.ConfigDir)
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
				ConfigDir:     "/path/to/config",
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
				ConfigDir: "/path/to/config",
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
				ConfigDir:     "/path/to/config",
				LogLevel:      "info",
				RateLimit:     0,
			},
			expectedErr: "rate limit must be greater than 0",
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

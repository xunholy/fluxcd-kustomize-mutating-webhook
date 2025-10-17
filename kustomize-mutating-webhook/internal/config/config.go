package config

import (
	"errors"
	"os"
	"strconv"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

type Config struct {
	ServerAddress string
	CertFile      string
	KeyFile       string
	ConfigDir     string
	LogLevel      string
	RateLimit     int
}

const (
	defaultServerAddress = ":8443"
	defaultCertFile      = "/etc/webhook/certs/tls.crt"
	defaultKeyFile       = "/etc/webhook/certs/tls.key"
	defaultConfigDir     = "/etc/config"
	defaultLogLevel      = "info"
	defaultRateLimit     = 100
)

func LoadConfig() Config {
	return Config{
		ServerAddress: getEnv("SERVER_ADDRESS", defaultServerAddress),
		CertFile:      getEnv("CERT_FILE", defaultCertFile),
		KeyFile:       getEnv("KEY_FILE", defaultKeyFile),
		ConfigDir:     getEnv("CONFIG_DIR", defaultConfigDir),
		LogLevel:      getEnv("LOG_LEVEL", defaultLogLevel),
		RateLimit:     getEnvAsInt("RATE_LIMIT", defaultRateLimit),
	}
}

func ValidateConfig(cfg Config) error {
	if cfg.ServerAddress == "" {
		return errors.New("server address is required")
	}
	if cfg.CertFile == "" {
		return errors.New("certificate file path is required")
	}
	if cfg.KeyFile == "" {
		return errors.New("key file path is required")
	}
	if cfg.ConfigDir == "" {
		return errors.New("config directory is required")
	}
	if cfg.RateLimit <= 0 {
		return errors.New("rate limit must be greater than 0")
	}
	return nil
}

func InitLogger(logLevel string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: zerolog.TimeFieldFormat, NoColor: false}
	log.Logger = log.Output(consoleWriter)

	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	zerolog.SetGlobalLevel(level)
	log.Info().Msgf("Log level set to '%s'", level.String())
}

func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvAsInt(key string, fallback int) int {
	strValue := getEnv(key, "")
	if value, err := strconv.Atoi(strValue); err == nil {
		return value
	}
	return fallback
}

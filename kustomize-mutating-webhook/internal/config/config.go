package config

import (
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// ServiceAccountNamespaceFile is the path to the file containing the pod's namespace.
// It is a package-level variable so tests can override it.
var ServiceAccountNamespaceFile = "/var/run/secrets/kubernetes.io/serviceaccount/namespace"

func DetectNamespace(configured string) string {
	if configured != "" {
		return configured
	}
	if ns, err := os.ReadFile(ServiceAccountNamespaceFile); err == nil {
		return strings.TrimSpace(string(ns))
	}
	return "flux-system"
}

type Config struct {
	ServerAddress                string
	CertFile                     string
	KeyFile                      string
	LogLevel                     string
	RateLimit                    int
	AutoUpdateKustomizations     bool
	AutoUpdateExcludeNamespaces  []string
	WatchConfigMaps              []string
	WatchSecrets                 []string
	WatchNamespace               string
}

const (
	defaultServerAddress               = ":8443"
	defaultCertFile                    = "/etc/webhook/certs/tls.crt"
	defaultKeyFile                     = "/etc/webhook/certs/tls.key"
	defaultLogLevel                    = "info"
	defaultRateLimit                   = 100
	defaultAutoUpdateKustomizations    = true
	defaultAutoUpdateExcludeNamespaces = "flux-system"
	defaultWatchConfigMaps             = "cluster-config"
	defaultWatchSecrets                = ""
	defaultWatchNamespace              = ""
)

func LoadConfig() Config {
	return Config{
		ServerAddress:               getEnv("SERVER_ADDRESS", defaultServerAddress),
		CertFile:                    getEnv("CERT_FILE", defaultCertFile),
		KeyFile:                     getEnv("KEY_FILE", defaultKeyFile),
		LogLevel:                    getEnv("LOG_LEVEL", defaultLogLevel),
		RateLimit:                   getEnvAsInt("RATE_LIMIT", defaultRateLimit),
		AutoUpdateKustomizations:    getEnvAsBool("AUTO_UPDATE_KUSTOMIZATIONS", defaultAutoUpdateKustomizations),
		AutoUpdateExcludeNamespaces: getEnvAsSlice("AUTO_UPDATE_EXCLUDE_NAMESPACES", defaultAutoUpdateExcludeNamespaces),
		WatchConfigMaps:             getEnvAsSlice("WATCH_CONFIGMAPS", defaultWatchConfigMaps),
		WatchSecrets:                getEnvAsSlice("WATCH_SECRETS", defaultWatchSecrets),
		WatchNamespace:              getEnv("WATCH_NAMESPACE", defaultWatchNamespace),
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
	if cfg.RateLimit <= 0 {
		return errors.New("rate limit must be greater than 0")
	}
	return nil
}

func InitLogger(logLevel string) {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	if getEnv("LOG_FORMAT", "json") == "console" {
		consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: zerolog.TimeFieldFormat, NoColor: false}
		log.Logger = log.Output(consoleWriter)
	} else {
		log.Logger = zerolog.New(os.Stderr).With().Timestamp().Logger()
	}

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

func getEnvAsBool(key string, fallback bool) bool {
	strValue := getEnv(key, "")
	if value, err := strconv.ParseBool(strValue); err == nil {
		return value
	}
	return fallback
}

func getEnvAsSlice(key string, fallback string) []string {
	strValue := getEnv(key, fallback)
	if strValue == "" {
		return []string{}
	}
	parts := strings.Split(strValue, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

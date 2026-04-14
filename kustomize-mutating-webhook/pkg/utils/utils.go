package utils

import (
	"strings"
	"sync"
)

var AppConfig struct {
	Mu     sync.RWMutex
	Config map[string]string
}

func GetAppConfig(key string) (string, bool) {
	AppConfig.Mu.RLock()
	defer AppConfig.Mu.RUnlock()
	value, ok := AppConfig.Config[key]
	return value, ok
}

func EscapeJsonPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

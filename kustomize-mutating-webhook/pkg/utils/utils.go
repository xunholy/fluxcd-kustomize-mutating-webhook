package utils

import (
	"strings"
	"sync"
)

var AppConfig struct {
	Mu     sync.RWMutex
	Config map[string]string
}

func EscapeJsonPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

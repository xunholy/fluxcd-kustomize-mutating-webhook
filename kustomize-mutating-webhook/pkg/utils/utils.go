package utils

import (
	"regexp"
	"strings"
	"sync"
)

// varsubRegex matches the Flux kustomize-controller variable substitution pattern.
// Keys must start with a letter or underscore, followed by letters, digits, or underscores.
// Source: https://github.com/fluxcd/pkg/blob/main/kustomize/kustomize_varsub.go
var varsubRegex = regexp.MustCompile(`^[_a-zA-Z][_a-zA-Z0-9]*$`)

var AppConfig struct {
	Mu     sync.RWMutex
	Config map[string]string
}

// IsValidSubstituteKey checks whether a key is a valid Flux postBuild substitute variable name.
func IsValidSubstituteKey(key string) bool {
	return varsubRegex.MatchString(key)
}

func EscapeJsonPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

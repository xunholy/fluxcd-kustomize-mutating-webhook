package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var AppConfig struct {
	Mu     sync.RWMutex
	Config map[string]string
}

func ReadConfigDirectory(directory string) error {
	dirInfo, err := os.Stat(directory)
	if err != nil {
		return fmt.Errorf("%w", err)
	}

	if !dirInfo.IsDir() {
		return fmt.Errorf("not a directory: %s", directory)
	}

	config := make(map[string]string)

	var files []string
	err = filepath.WalkDir(directory, func(path string, d os.DirEntry, err error) error {
		validFile, err := isValidConfigFile(path)
		if err != nil {
			return err
		}
		if validFile {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("error reading directory: %w", err)
	}

	for _, file := range files {
		fileName := filepath.Base(file)
		value, err := os.ReadFile(file)
		if err != nil {
			return fmt.Errorf("error reading file %s: %w", file, err)
		}
		config[fileName] = string(value)
	}

	if len(config) == 0 {
		return fmt.Errorf("no configuration found")
	}

	AppConfig.Mu.Lock()
	AppConfig.Config = config
	AppConfig.Mu.Unlock()

	return nil
}

func isValidConfigFile(path string) (bool, error) {
	fileInfo, err := os.Stat(path)
	if err != nil {
		return false, err
	}

	if strings.HasPrefix(fileInfo.Name(), ".") {
		return false, nil
	}

	if fileInfo.IsDir() {
		return false, nil
	}

	return true, nil
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

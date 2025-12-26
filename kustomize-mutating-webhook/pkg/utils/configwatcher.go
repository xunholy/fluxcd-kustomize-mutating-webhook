package utils

import (
	"errors"
	"fmt"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

type ConfigWatcher struct {
	configDir string
	watcher   *fsnotify.Watcher
	done      chan struct{}
}

func NewConfigWatcher(configDir string) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &ConfigWatcher{
		configDir: configDir,
		watcher:   watcher,
		done:      make(chan struct{}),
	}
	return cw, nil
}

func (cw *ConfigWatcher) reloadConfig() {
	oldCount := 0
	AppConfig.Mu.RLock()
	oldCount = len(AppConfig.Config)
	AppConfig.Mu.RUnlock()

	if err := ReadConfigDirectory(cw.configDir); err != nil {
		log.Error().Err(err).Msg("Failed to reload configuration")
		return
	}

	AppConfig.Mu.RLock()
	newCount := len(AppConfig.Config)
	AppConfig.Mu.RUnlock()

	log.Info().
		Int("old_count", oldCount).
		Int("new_count", newCount).
		Msg("Configuration reloaded successfully")
}

func (cw *ConfigWatcher) Watch() error {
	if err := cw.watcher.Add(cw.configDir); err != nil {
		return fmt.Errorf("failed to add directory to watcher: %w", err)
	}

	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return errors.New("watcher channel closed")
			}
			// React to file modifications, creations, deletions, and renames
			// This handles Kubernetes ConfigMap/Secret updates which use symlink swapping
			if event.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Remove|fsnotify.Rename) != 0 {
				log.Info().Str("event", event.String()).Msg("Config directory modified. Reloading...")
				// Sleep briefly to allow Kubernetes to complete atomic ConfigMap/Secret updates
				time.Sleep(100 * time.Millisecond)
				cw.reloadConfig()
			}
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return errors.New("watcher error channel closed")
			}
			log.Error().Err(err).Msg("Error watching config files")
		case <-cw.done:
			return nil
		}
	}
}

func (cw *ConfigWatcher) Stop() {
	close(cw.done)
	cw.watcher.Close()
}

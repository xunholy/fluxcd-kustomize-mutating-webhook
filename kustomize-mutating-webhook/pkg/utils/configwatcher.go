package utils

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

type ConfigWatcher struct {
	configDir             string
	watcher               *fsnotify.Watcher
	done                  chan struct{}
	kustomizationUpdater  *KustomizationUpdater
	autoUpdate            bool
}

func NewConfigWatcher(configDir string, autoUpdate bool, kustomizationUpdater *KustomizationUpdater) (*ConfigWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &ConfigWatcher{
		configDir:            configDir,
		watcher:              watcher,
		done:                 make(chan struct{}),
		kustomizationUpdater: kustomizationUpdater,
		autoUpdate:           autoUpdate,
	}
	return cw, nil
}

func (cw *ConfigWatcher) reloadConfig() {
	oldCount := 0
	oldKeys := make([]string, 0)
	AppConfig.Mu.RLock()
	oldCount = len(AppConfig.Config)
	for key := range AppConfig.Config {
		oldKeys = append(oldKeys, key)
	}
	AppConfig.Mu.RUnlock()

	log.Info().Str("config_dir", cw.configDir).Msg("Reloading configuration from directory")

	if err := ReadConfigDirectory(cw.configDir); err != nil {
		log.Error().
			Err(err).
			Str("config_dir", cw.configDir).
			Int("old_count", oldCount).
			Msg("Failed to reload configuration - keeping old config")
		return
	}

	newCount := 0
	newKeys := make([]string, 0)
	addedKeys := make([]string, 0)
	removedKeys := make([]string, 0)

	AppConfig.Mu.RLock()
	newCount = len(AppConfig.Config)
	for key := range AppConfig.Config {
		newKeys = append(newKeys, key)
		if !contains(oldKeys, key) {
			addedKeys = append(addedKeys, key)
		}
	}
	for _, key := range oldKeys {
		if !contains(newKeys, key) {
			removedKeys = append(removedKeys, key)
		}
	}
	AppConfig.Mu.RUnlock()

	log.Info().
		Int("old_count", oldCount).
		Int("new_count", newCount).
		Strs("added_keys", addedKeys).
		Strs("removed_keys", removedKeys).
		Msg("Configuration reloaded successfully")

	// Trigger Kustomization updates if auto-update is enabled
	if cw.autoUpdate && cw.kustomizationUpdater != nil {
		go func() {
			if err := cw.kustomizationUpdater.TriggerUpdateAll(); err != nil {
				log.Error().Err(err).Msg("Failed to trigger Kustomization updates")
			}
		}()
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// addSubdirsToWatcher recursively adds all subdirectories to the fsnotify watcher
func (cw *ConfigWatcher) addSubdirsToWatcher() error {
	return filepath.WalkDir(cw.configDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			log.Debug().Str("path", path).Msg("Adding directory to watcher")
			if err := cw.watcher.Add(path); err != nil {
				return fmt.Errorf("failed to add directory %s to watcher: %w", path, err)
			}
		}
		return nil
	})
}

func (cw *ConfigWatcher) Watch() error {
	// Recursively add all subdirectories to the watcher
	// This is necessary because Kubernetes mounts ConfigMaps/Secrets as subdirectories
	if err := cw.addSubdirsToWatcher(); err != nil {
		return fmt.Errorf("failed to add directories to watcher: %w", err)
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

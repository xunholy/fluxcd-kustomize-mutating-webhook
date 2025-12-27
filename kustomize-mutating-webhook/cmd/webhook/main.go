package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/rs/zerolog/log"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/config"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/webhook"
	"github.com/xunholy/fluxcd-mutating-webhook/pkg/utils"
)

func main() {
	cfg := config.LoadConfig()
	if err := config.ValidateConfig(cfg); err != nil {
		log.Fatal().Err(err).Msg("Invalid configuration")
	}
	config.InitLogger(cfg.LogLevel)

	log.Info().Str("config_dir", cfg.ConfigDir).Msg("Reading configuration directory")
	if err := utils.ReadConfigDirectory(cfg.ConfigDir); err != nil {
		log.Warn().Err(err).Str("config_dir", cfg.ConfigDir).Msg("Error while reading config directory")
	} else {
		log.Info().Msg("Initial configuration loaded")
	}

	// Create Kustomization updater if auto-update is enabled
	var kustomizationUpdater *utils.KustomizationUpdater
	var err error
	if cfg.AutoUpdateKustomizations {
		kustomizationUpdater, err = utils.NewKustomizationUpdater(cfg.AutoUpdateExcludeNamespaces)
		if err != nil {
			log.Warn().
				Err(err).
				Msg("Failed to create Kustomization updater - auto-update will be disabled. " +
					"This is expected if running outside Kubernetes or without proper RBAC permissions.")
			log.Info().
				Bool("auto_update", false).
				Msg("Kustomization auto-update disabled (failed to initialize)")
		} else {
			log.Info().
				Bool("auto_update", true).
				Strs("exclude_namespaces", cfg.AutoUpdateExcludeNamespaces).
				Msg("Kustomization auto-update enabled")
		}
	} else {
		log.Info().Bool("auto_update", false).Msg("Kustomization auto-update disabled")
	}

	configWatcher, err := utils.NewConfigWatcher(cfg.ConfigDir, cfg.AutoUpdateKustomizations, kustomizationUpdater)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create config watcher")
	}

	server, err := webhook.NewServer(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	// Start certificate watcher in a goroutine
	go func() {
		if err := server.CertWatcher.Watch(); err != nil {
			log.Fatal().Err(err).Msg("Certificate watcher exited with error")
		}
	}()

	// Start config watcher in a goroutine
	log.Info().Str("config_dir", cfg.ConfigDir).Msg("Starting config watcher for hot-reload")
	go func() {
		if err := configWatcher.Watch(); err != nil {
			log.Error().Err(err).Msg("Config watcher exited with error")
		}
	}()

	go func() {
		log.Info().Msgf("Starting the webhook server on %s", cfg.ServerAddress)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	waitForShutdown(server, configWatcher)
}

func waitForShutdown(server *webhook.Server, configWatcher *utils.ConfigWatcher) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")

	server.CertWatcher.Stop()
	configWatcher.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
}

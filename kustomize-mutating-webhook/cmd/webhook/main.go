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
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

func main() {
	cfg := config.LoadConfig()
	if err := config.ValidateConfig(cfg); err != nil {
		log.Fatal().Err(err).Msg("Invalid configuration")
	}
	config.InitLogger(cfg.LogLevel)

	namespace := config.DetectNamespace(cfg.WatchNamespace)
	log.Info().Str("namespace", namespace).Msg("Detected watch namespace")

	// Create Kustomization updater if auto-update is enabled
	var kustomizationUpdater *utils.KustomizationUpdater
	if cfg.AutoUpdateKustomizations {
		var err error
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

	// Create Kubernetes clientset for config watching
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to get in-cluster config for config informer")
	}
	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create Kubernetes clientset")
	}

	autoUpdate := kustomizationUpdater != nil && cfg.AutoUpdateKustomizations
	configInformer := utils.NewConfigInformer(
		clientset,
		namespace,
		cfg.WatchConfigMaps,
		cfg.WatchSecrets,
		autoUpdate,
		kustomizationUpdater,
	)

	server, err := webhook.NewServer(cfg)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to create server")
	}

	go func() {
		if err := server.CertWatcher.Watch(); err != nil {
			log.Fatal().Err(err).Msg("Certificate watcher exited with error")
		}
	}()

	log.Info().
		Strs("configmaps", cfg.WatchConfigMaps).
		Strs("secrets", cfg.WatchSecrets).
		Str("namespace", namespace).
		Msg("Starting config informer")
	go func() {
		if err := configInformer.Start(); err != nil {
			log.Error().Err(err).Msg("Config informer exited with error")
		}
	}()

	go func() {
		log.Info().Msgf("Starting the webhook server on %s", cfg.ServerAddress)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	waitForShutdown(server, configInformer)
}

func waitForShutdown(server *webhook.Server, configInformer *utils.ConfigInformer) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")

	server.CertWatcher.Stop()
	configInformer.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
}

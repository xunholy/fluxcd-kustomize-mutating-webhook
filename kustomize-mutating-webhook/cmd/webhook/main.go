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

	if err := utils.ReadConfigDirectory(cfg.ConfigDir); err != nil {
		log.Warn().Err(err).Msg("Error while reading config directory")
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

	go func() {
		log.Info().Msgf("Starting the webhook server on %s", cfg.ServerAddress)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	waitForShutdown(server)
}

func waitForShutdown(server *webhook.Server) {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")
	
	server.CertWatcher.Stop()

	ctx, cancel := context.WithTimeout(context.Background(), server.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
}

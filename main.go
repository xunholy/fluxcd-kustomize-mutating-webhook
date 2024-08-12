package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	jsoniter "github.com/json-iterator/go"
	"github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"
	"golang.org/x/time/rate"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

const (
	defaultServerAddress = ":8443"
	defaultCertFile      = "/etc/webhook/certs/tls.crt"
	defaultKeyFile       = "/etc/webhook/certs/tls.key"
	defaultConfigDir     = "/etc/config"
	defaultLogLevel      = "info"
	defaultRateLimit     = 100
)

var (
	appConfig         map[string]string
	errConfigNotFound = errors.New("configuration not found")
)

type CertWatcher struct {
	certFile string
	keyFile  string
	cert     *tls.Certificate
	mu       sync.RWMutex
	watcher  *fsnotify.Watcher
	done     chan struct{}
}

func NewCertWatcher(certFile, keyFile string) (*CertWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &CertWatcher{
		certFile: certFile,
		keyFile:  keyFile,
		watcher:  watcher,
		done:     make(chan struct{}),
	}
	if err := cw.loadCertificate(); err != nil {
		return nil, fmt.Errorf("failed to load initial certificate: %w", err)
	}
	return cw, nil
}

func (cw *CertWatcher) loadCertificate() error {
	cert, err := tls.LoadX509KeyPair(cw.certFile, cw.keyFile)
	if err != nil {
		return fmt.Errorf("failed to load key pair: %w", err)
	}
	cw.mu.Lock()
	cw.cert = &cert
	cw.mu.Unlock()
	return nil
}

func (cw *CertWatcher) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return cw.cert, nil
}

func (cw *CertWatcher) Watch() error {
	if err := cw.watcher.Add(filepath.Dir(cw.certFile)); err != nil {
		return fmt.Errorf("failed to add directory to watcher: %w", err)
	}

	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return errors.New("watcher channel closed")
			}
			// Each time a certificate is renewed, there's a series of file system events (CREATE, CHMOD, CREATE, RENAME, CREATE and REMOVE)
			// Trigger certificate reload on the last event: REMOVE
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				log.Info().Msg("Certificate files modified. Reloading...")
				if err := cw.loadCertificate(); err != nil {
					log.Error().Err(err).Msg("Failed to reload certificate")
				} else {
					log.Info().Msg("Certificate reloaded successfully")
				}
			}
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return errors.New("watcher error channel closed")
			}
			log.Error().Err(err).Msg("Error watching certificate files")
		case <-cw.done:
			return nil
		}
	}
}

func (cw *CertWatcher) Stop() {
	close(cw.done)
	cw.watcher.Close()
}

func init() {
	// Set up logging to console
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// Set up colored output
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: zerolog.TimeFieldFormat, NoColor: false}
	log.Logger = log.Output(consoleWriter)

	// Set log level
	logLevel := os.Getenv("LOG_LEVEL")
	if logLevel == "" {
		logLevel = defaultLogLevel
	}
	level, err := zerolog.ParseLevel(logLevel)
	if err != nil {
		level = zerolog.InfoLevel
	}

	// Set the global log level
	zerolog.SetGlobalLevel(level)
	log.Info().Msgf("Log level set to '%s'", level.String())
}

func readConfigMap(directory string) (map[string]string, error) {
	config := make(map[string]string)
	files, err := os.ReadDir(directory)
	if err != nil {
		return nil, fmt.Errorf("error reading directory: %w", err)
	}

	for _, file := range files {
		if file.IsDir() || strings.HasPrefix(file.Name(), ".") {
			continue
		}

		fullPath := filepath.Join(directory, file.Name())
		value, err := os.ReadFile(fullPath)
		if err != nil {
			return nil, fmt.Errorf("error reading file %s: %w", fullPath, err)
		}
		config[file.Name()] = string(value)
	}

	if len(config) == 0 {
		return nil, errConfigNotFound
	}

	return config, nil
}

func handleMutate(w http.ResponseWriter, r *http.Request) {
	var admissionReviewReq v1.AdmissionReview

	if err := jsoniter.NewDecoder(r.Body).Decode(&admissionReviewReq); err != nil {
		log.Error().Err(err).Msg("Failed to decode AdmissionReview request")
		http.Error(w, "Could not decode request", http.StatusBadRequest)
		return
	}

	// Create a default response that allows the admission request
	admissionResponse := v1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &v1.AdmissionResponse{
			UID:     admissionReviewReq.Request.UID,
			Allowed: true,
		},
	}

	// Only mutate Kustomization resources
	// This allows other resources to pass through without modification
	if admissionReviewReq.Request.Kind.Kind != "Kustomization" {
		log.Info().Msgf("Skipping mutation for non-Kustomization resource: %s", admissionReviewReq.Request.Kind.Kind)
		respondWithAdmissionReview(w, admissionResponse)
		return
	}

	var obj unstructured.Unstructured
	if err := json.Unmarshal(admissionReviewReq.Request.Object.Raw, &obj); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal Object")
		http.Error(w, "Failed to unmarshal Object", http.StatusBadRequest)
		return
	}

	// Allow deletions to proceed without modification
	if admissionReviewReq.Request.Operation == v1.Delete || !obj.GetDeletionTimestamp().IsZero() {
		respondWithAdmissionReview(w, admissionResponse)
		return
	}

	log.Info().
		Str("UID", string(admissionReviewReq.Request.UID)).
		Str("Kind", admissionReviewReq.Request.Kind.Kind).
		Str("Resource", admissionReviewReq.Request.Resource.Resource).
		Str("Name", admissionReviewReq.Request.Name).
		Str("Namespace", admissionReviewReq.Request.Namespace).
		Msg("Request details")

	// Create patch for Kustomization resources
	var patch []map[string]interface{}

	// Ensure /spec/postBuild exists
	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "postBuild"); !found {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild",
			"value": map[string]interface{}{},
		})
	}

	// Ensure /spec/postBuild/substitute exists
	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "postBuild", "substitute"); !found {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild/substitute",
			"value": map[string]interface{}{},
		})
	}

	// Add key-value pairs from appConfig to /spec/postBuild/substitute
	for key, value := range appConfig {
		escapedKey := escapeJsonPointer(key)
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild/substitute/" + escapedKey,
			"value": value,
		})
	}

	// Apply the patch if any modifications were made
	if len(patch) > 0 {
		patchBytes, _ := json.Marshal(patch)
		admissionResponse.Response.Patch = patchBytes
		pt := v1.PatchTypeJSONPatch
		admissionResponse.Response.PatchType = &pt

		log.Debug().
			Str("Patch", string(patchBytes)).
			Msg("Applying mutation to resource")
	}

	respondWithAdmissionReview(w, admissionResponse)
}

// Encodes and sends the AdmissionReview response
func respondWithAdmissionReview(w http.ResponseWriter, admissionResponse v1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(admissionResponse); err != nil {
		log.Error().Err(err).Msg("Failed to encode AdmissionReview response")
		http.Error(w, "Could not encode response", http.StatusInternalServerError)
	}
}

// escapeJsonPointer escapes special characters in JSON pointer
func escapeJsonPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	if len(appConfig) == 0 {
		http.Error(w, "Configuration not loaded", http.StatusServiceUnavailable)
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

func rateLimitMiddleware(r rate.Limit, b int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(r, b)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	serverAddress := getEnv("SERVER_ADDRESS", defaultServerAddress)
	certFile := getEnv("CERT_FILE", defaultCertFile)
	keyFile := getEnv("KEY_FILE", defaultKeyFile)
	configDir := getEnv("CONFIG_DIR", defaultConfigDir)
	rateLimit := getEnvAsInt("RATE_LIMIT", defaultRateLimit)

	var err error
	appConfig, err = readConfigMap(configDir)
	if err != nil {
		if errors.Is(err, errConfigNotFound) {
			log.Warn().Msg("No configuration found, starting with empty config")
		} else {
			log.Fatal().Err(err).Msg("Failed to read configuration")
		}
	}

	log.Debug().Msg("Loaded appConfig:")
	for key, value := range appConfig {
		log.Debug().Msgf("Config - Key: %s, Value: %s", key, value)
	}

	// Initialize certificate watcher
	certWatcher, err := NewCertWatcher(certFile, keyFile)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to initialize certificate watcher")
	}

	go func() {
		if err := certWatcher.Watch(); err != nil {
			log.Error().Err(err).Msg("Certificate watcher error")
		}
	}()

	// Initialize router
	r := chi.NewRouter()

	// Middleware
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(rateLimitMiddleware(rate.Limit(rateLimit), rateLimit))

	// Routes
	r.Post("/mutate", handleMutate)
	r.Get("/health", handleHealth)
	r.Get("/ready", handleReady)

	// Initialize server
	server := &http.Server{
		Addr:    serverAddress,
		Handler: r,
		TLSConfig: &tls.Config{
			GetCertificate: certWatcher.GetCertificate,
		},
	}

	// Start server
	go func() {
		log.Info().Msgf("Starting the webhook server on %s", server.Addr)
		if err := server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Failed to start server")
		}
	}()

	// Graceful shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info().Msg("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	certWatcher.Stop()

	if err := server.Shutdown(ctx); err != nil {
		log.Fatal().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exiting")
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

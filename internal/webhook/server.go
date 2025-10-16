package webhook

import (
	"crypto/tls"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/config"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/handlers"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/metrics"
	"github.com/xunholy/fluxcd-mutating-webhook/pkg/utils"
	"golang.org/x/time/rate"
)

type Server struct {
	*http.Server
	CertWatcher     *CertWatcher
	ShutdownTimeout time.Duration
}

func NewServer(cfg config.Config) (*Server, error) {
	certWatcher, err := NewCertWatcher(cfg.CertFile, cfg.KeyFile)
	if err != nil {
		return nil, err
	}

	r := setupRouter(cfg.RateLimit)

	server := &http.Server{
		Addr:    cfg.ServerAddress,
		Handler: r,
		TLSConfig: &tls.Config{
			GetCertificate: certWatcher.GetCertificate,
		},
	}

	return &Server{
		Server:          server,
		certWatcher:     certWatcher,
		ShutdownTimeout: 30 * time.Second,
	}, nil
}

func setupRouter(rateLimit int) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(rateLimitMiddleware(rate.Limit(rateLimit), rateLimit))
	r.Use(conditionalLoggerMiddleware())

	r.Post("/mutate", handlers.HandleMutate)
	r.Get("/health", handleHealth)
	r.Get("/ready", handleReady)
	r.Handle("/metrics", promhttp.Handler())

	return r
}

func rateLimitMiddleware(r rate.Limit, b int) func(http.Handler) http.Handler {
	limiter := rate.NewLimiter(r, b)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if !limiter.Allow() {
				metrics.RateLimitedRequests.Inc()
				http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	utils.AppConfig.Mu.RLock()
	configLoaded := len(utils.AppConfig.Config) > 0
	utils.AppConfig.Mu.RUnlock()

	ready := struct {
		Status       string `json:"status"`
		ConfigLoaded bool   `json:"configLoaded"`
		Timestamp    string `json:"timestamp"`
	}{
		Status:       "Ready",
		ConfigLoaded: configLoaded,
		Timestamp:    time.Now().Format(time.RFC3339),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(ready)
}

func conditionalLoggerMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if zerolog.GlobalLevel() == zerolog.DebugLevel {
				start := time.Now()
				next.ServeHTTP(w, r)
				duration := time.Since(start)

				log.Debug().
					Str("method", r.Method).
					Str("path", r.URL.Path).
					Str("remote_ip", r.RemoteAddr).
					Dur("duration", duration).
					Msg("Handled request")
			} else {
				next.ServeHTTP(w, r)
			}
		})
	}
}

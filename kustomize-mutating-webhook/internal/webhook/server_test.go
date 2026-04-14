package webhook

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/config"
	"github.com/xunholy/fluxcd-mutating-webhook/pkg/utils"
	"github.com/xunholy/fluxcd-mutating-webhook/test"
	"golang.org/x/time/rate"
)

func TestNewServer(t *testing.T) {
	tempDir := t.TempDir()

	certPath, keyPath, err := test.GenerateTestCertificate(tempDir)
	require.NoError(t, err)

	cfg := config.Config{
		ServerAddress: ":8443",
		CertFile:      certPath,
		KeyFile:       keyPath,
		LogLevel:      "info",
		RateLimit:     100,
	}

	server, err := NewServer(cfg)
	require.NoError(t, err)
	assert.NotNil(t, server)
	assert.Equal(t, cfg.ServerAddress, server.Addr)
	assert.NotNil(t, server.TLSConfig)
	assert.NotNil(t, server.Handler)
}

func TestSetupRouter(t *testing.T) {
	router := setupRouter(100)
	assert.NotNil(t, router)

	// Test routes
	testCases := []struct {
		method string
		path   string
	}{
		{"POST", "/mutate"},
		{"GET", "/health"},
		{"GET", "/ready"},
		{"GET", "/metrics"},
	}

	for _, tc := range testCases {
		req, err := http.NewRequest(tc.method, tc.path, nil)
		require.NoError(t, err)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		assert.NotEqual(t, http.StatusNotFound, w.Code, "Route %s %s not found", tc.method, tc.path)
	}
}

func TestRateLimitMiddleware(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := rateLimitMiddleware(rate.Limit(1), 1)
	wrappedHandler := middleware(handler)

	// First request should succeed
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusOK, rr.Code)

	// Second request should be rate limited
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	assert.Equal(t, http.StatusTooManyRequests, rr.Code)
}

func TestHandleHealth(t *testing.T) {
	req, err := http.NewRequest("GET", "/health", nil)
	require.NoError(t, err)

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleHealth)
	handler.ServeHTTP(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "OK", rr.Body.String())
	assert.Equal(t, "text/plain", rr.Header().Get("Content-Type"))
}

func TestHandleReady(t *testing.T) {
	t.Cleanup(func() {
		utils.AppConfig.Mu.Lock()
		utils.AppConfig.Config = nil
		utils.AppConfig.Mu.Unlock()
	})

	utils.AppConfig.Mu.Lock()
	utils.AppConfig.Config = map[string]string{"test": "value"}
	utils.AppConfig.Mu.Unlock()

	tests := []struct {
		name           string
		configLoaded   bool
		expectedStatus int
		expectedBody   map[string]interface{}
	}{
		{
			name:           "Config loaded",
			configLoaded:   true,
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"status":       "Ready",
				"configLoaded": true,
			},
		},
		{
			name:           "Config not loaded",
			configLoaded:   false,
			expectedStatus: http.StatusServiceUnavailable,
			expectedBody: map[string]interface{}{
				"status":       "NotReady",
				"configLoaded": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.configLoaded {
				utils.AppConfig.Mu.Lock()
				utils.AppConfig.Config = map[string]string{}
				utils.AppConfig.Mu.Unlock()
			}

			req, err := http.NewRequest("GET", "/ready", nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(handleReady)

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			body, err := io.ReadAll(rr.Body)
			require.NoError(t, err)

			var result map[string]interface{}
			err = json.Unmarshal(body, &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedBody["status"], result["status"])
			assert.Equal(t, tt.expectedBody["configLoaded"], result["configLoaded"])
			assert.Contains(t, result, "timestamp")
			assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))
		})
	}
}

func TestNewCertWatcher(t *testing.T) {
	t.Run("valid cert and key loads successfully", func(t *testing.T) {
		tempDir := t.TempDir()
		certPath, keyPath, err := test.GenerateTestCertificate(tempDir)
		require.NoError(t, err)

		cw, err := NewCertWatcher(certPath, keyPath)
		require.NoError(t, err)
		assert.NotNil(t, cw)
		cw.Stop()
	})

	t.Run("invalid path returns error", func(t *testing.T) {
		cw, err := NewCertWatcher("/nonexistent/cert.crt", "/nonexistent/key.key")
		assert.Error(t, err)
		assert.Nil(t, cw)
	})
}

func TestCertWatcher_GetCertificate(t *testing.T) {
	tempDir := t.TempDir()
	certPath, keyPath, err := test.GenerateTestCertificate(tempDir)
	require.NoError(t, err)

	cw, err := NewCertWatcher(certPath, keyPath)
	require.NoError(t, err)
	defer cw.Stop()

	cert, err := cw.GetCertificate(nil)
	require.NoError(t, err)
	assert.NotNil(t, cert)
}

func TestCertWatcher_Watch_FileChange(t *testing.T) {
	tempDir := t.TempDir()
	certPath, keyPath, err := test.GenerateTestCertificate(tempDir)
	require.NoError(t, err)

	cw, err := NewCertWatcher(certPath, keyPath)
	require.NoError(t, err)
	defer cw.Stop()

	origCert, err := cw.GetCertificate(nil)
	require.NoError(t, err)
	origBytes := make([]byte, len(origCert.Certificate[0]))
	copy(origBytes, origCert.Certificate[0])

	watchDone := make(chan error, 1)
	go func() {
		watchDone <- cw.Watch()
	}()

	// Give the watcher time to register the directory
	time.Sleep(50 * time.Millisecond)

	// Overwrite cert files with a freshly generated cert
	_, _, err = test.GenerateTestCertificate(tempDir)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		cert, err := cw.GetCertificate(nil)
		if err != nil {
			return false
		}
		return !bytes.Equal(origBytes, cert.Certificate[0])
	}, 2*time.Second, 50*time.Millisecond, "certificate should have been reloaded after file change")
}

func TestCertWatcher_Stop(t *testing.T) {
	tempDir := t.TempDir()
	certPath, keyPath, err := test.GenerateTestCertificate(tempDir)
	require.NoError(t, err)

	cw, err := NewCertWatcher(certPath, keyPath)
	require.NoError(t, err)

	watchDone := make(chan error, 1)
	go func() {
		watchDone <- cw.Watch()
	}()

	// Give the watcher time to start
	time.Sleep(50 * time.Millisecond)

	cw.Stop()

	select {
	case err := <-watchDone:
		assert.NoError(t, err)
	case <-time.After(1 * time.Second):
		t.Fatal("Watch() did not return after Stop()")
	}

	// Double Stop() must not panic
	assert.NotPanics(t, func() {
		cw.Stop()
	})
}

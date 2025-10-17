package webhook

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/config"
	"github.com/xunholy/fluxcd-mutating-webhook/pkg/utils"
	"github.com/xunholy/fluxcd-mutating-webhook/test"
	"golang.org/x/time/rate"
)

func TestNewServer(t *testing.T) {
	tempDir, err := ioutil.TempDir("", "webhook-test")
	require.NoError(t, err)
	defer os.RemoveAll(tempDir)

	certPath, keyPath, err := test.GenerateTestCertificate(tempDir)
	require.NoError(t, err)

	cfg := config.Config{
		ServerAddress: ":8443",
		CertFile:      certPath,
		KeyFile:       keyPath,
		ConfigDir:     tempDir,
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

func TestHandleReady(t *testing.T) {
	utils.AppConfig.Config = map[string]string{"test": "value"}

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
			expectedStatus: http.StatusOK,
			expectedBody: map[string]interface{}{
				"status":       "Ready",
				"configLoaded": false,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if !tt.configLoaded {
				utils.AppConfig.Config = map[string]string{}
			}

			req, err := http.NewRequest("GET", "/ready", nil)
			require.NoError(t, err)

			rr := httptest.NewRecorder()
			handler := http.HandlerFunc(handleReady)

			handler.ServeHTTP(rr, req)

			assert.Equal(t, tt.expectedStatus, rr.Code)

			body, err := ioutil.ReadAll(rr.Body)
			require.NoError(t, err)

			var result map[string]interface{}
			err = json.Unmarshal(body, &result)
			require.NoError(t, err)

			assert.Equal(t, tt.expectedBody["status"], result["status"])
			assert.Equal(t, tt.expectedBody["configLoaded"], result["configLoaded"])
			assert.Contains(t, result, "timestamp")
		})
	}
}

package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
)

func TestMetricsInitialization(t *testing.T) {
	// Create a new registry for this test to avoid conflicts with global registry
	registry := prometheus.NewRegistry()

	// Register metrics with the new registry
	registry.MustRegister(TotalRequests)
	registry.MustRegister(RequestDuration)
	registry.MustRegister(MutationCount)
	registry.MustRegister(ErrorCount)
	registry.MustRegister(ConfigReloads)
	registry.MustRegister(RateLimitedRequests)

	// Check if metrics are registered
	assert.True(t, registry.Unregister(TotalRequests))
	assert.True(t, registry.Unregister(RequestDuration))
	assert.True(t, registry.Unregister(MutationCount))
	assert.True(t, registry.Unregister(ErrorCount))
	assert.True(t, registry.Unregister(ConfigReloads))
	assert.True(t, registry.Unregister(RateLimitedRequests))

	// Assertions to check if metrics are not nil
	assert.NotNil(t, TotalRequests, "TotalRequests metric should be initialized")
	assert.NotNil(t, RequestDuration, "RequestDuration metric should be initialized")
	assert.NotNil(t, MutationCount, "MutationCount metric should be initialized")
	assert.NotNil(t, ErrorCount, "ErrorCount metric should be initialized")
	assert.NotNil(t, ConfigReloads, "ConfigReloads metric should be initialized")
	assert.NotNil(t, RateLimitedRequests, "RateLimitedRequests metric should be initialized")
}

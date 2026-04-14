package utils

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/fake"
)

func resetAppConfig() {
	AppConfig.Mu.Lock()
	AppConfig.Config = make(map[string]string)
	AppConfig.Mu.Unlock()
}

func TestConfigInformer_InitialLoad(t *testing.T) {
	resetAppConfig()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "value1", "key2": "value2"},
	}
	clientset := fake.NewSimpleClientset(cm)

	ci := NewConfigInformer(clientset, "test-ns", []string{"test-config"}, nil, false, nil)

	go ci.Start()
	defer ci.Stop()

	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return len(AppConfig.Config) == 2
	}, 5*time.Second, 50*time.Millisecond)

	AppConfig.Mu.RLock()
	assert.Equal(t, "value1", AppConfig.Config["key1"])
	assert.Equal(t, "value2", AppConfig.Config["key2"])
	AppConfig.Mu.RUnlock()
}

func TestConfigInformer_ConfigMapUpdate(t *testing.T) {
	resetAppConfig()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "initial"},
	}
	clientset := fake.NewSimpleClientset(cm)

	ci := NewConfigInformer(clientset, "test-ns", []string{"test-config"}, nil, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["key1"] == "initial"
	}, 5*time.Second, 50*time.Millisecond)

	// Update the ConfigMap
	cm.Data["key1"] = "updated"
	_, err := clientset.CoreV1().ConfigMaps("test-ns").Update(context.Background(), cm, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Wait for reload
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["key1"] == "updated"
	}, 5*time.Second, 50*time.Millisecond)
}

func TestConfigInformer_SecretUpdate(t *testing.T) {
	resetAppConfig()

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"password": []byte("initial-pass")},
	}
	clientset := fake.NewSimpleClientset(s)

	ci := NewConfigInformer(clientset, "test-ns", nil, []string{"test-secret"}, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["password"] == "initial-pass"
	}, 5*time.Second, 50*time.Millisecond)

	// Update the Secret
	s.Data["password"] = []byte("new-pass")
	_, err := clientset.CoreV1().Secrets("test-ns").Update(context.Background(), s, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Wait for reload
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["password"] == "new-pass"
	}, 5*time.Second, 50*time.Millisecond)
}

func TestConfigInformer_UnwatchedIgnored(t *testing.T) {
	resetAppConfig()

	watched := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "watched", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "value1"},
	}
	unwatched := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "unwatched", Namespace: "test-ns"},
		Data:       map[string]string{"other-key": "other-value"},
	}
	clientset := fake.NewSimpleClientset(watched, unwatched)

	ci := NewConfigInformer(clientset, "test-ns", []string{"watched"}, nil, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load from watched ConfigMap
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["key1"] == "value1"
	}, 5*time.Second, 50*time.Millisecond)

	// The unwatched ConfigMap's key should NOT be in AppConfig
	AppConfig.Mu.RLock()
	_, exists := AppConfig.Config["other-key"]
	AppConfig.Mu.RUnlock()
	assert.False(t, exists, "Unwatched ConfigMap should not be loaded")
}

func TestConfigInformer_MultipleResources(t *testing.T) {
	resetAppConfig()

	cm1 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "config1", Namespace: "test-ns"},
		Data:       map[string]string{"cm-key1": "cm-val1"},
	}
	cm2 := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "config2", Namespace: "test-ns"},
		Data:       map[string]string{"cm-key2": "cm-val2"},
	}
	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "secret1", Namespace: "test-ns"},
		Data:       map[string][]byte{"sec-key1": []byte("sec-val1")},
	}
	clientset := fake.NewSimpleClientset(cm1, cm2, s)

	ci := NewConfigInformer(clientset, "test-ns", []string{"config1", "config2"}, []string{"secret1"}, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for all keys to be loaded
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return len(AppConfig.Config) == 3
	}, 5*time.Second, 50*time.Millisecond)

	AppConfig.Mu.RLock()
	assert.Equal(t, "cm-val1", AppConfig.Config["cm-key1"])
	assert.Equal(t, "cm-val2", AppConfig.Config["cm-key2"])
	assert.Equal(t, "sec-val1", AppConfig.Config["sec-key1"])
	AppConfig.Mu.RUnlock()
}

func TestConfigInformer_Stop(t *testing.T) {
	resetAppConfig()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "value1"},
	}
	clientset := fake.NewSimpleClientset(cm)

	ci := NewConfigInformer(clientset, "test-ns", []string{"test-config"}, nil, false, nil)

	startDone := make(chan struct{})
	go func() {
		ci.Start() //nolint:errcheck
		close(startDone)
	}()

	// Wait for Start() to have begun by checking that AppConfig has been populated
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return len(AppConfig.Config) > 0
	}, 5*time.Second, 50*time.Millisecond)

	ci.Stop()

	select {
	case <-startDone:
		// Success - Start() returned
	case <-time.After(2 * time.Second):
		t.Fatal("Start() did not return after Stop() was called")
	}
}

func TestConfigInformer_ConfigMapDelete(t *testing.T) {
	resetAppConfig()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "value1"},
	}
	clientset := fake.NewSimpleClientset(cm)

	ci := NewConfigInformer(clientset, "test-ns", []string{"test-config"}, nil, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["key1"] == "value1"
	}, 5*time.Second, 50*time.Millisecond)

	// Delete the ConfigMap
	err := clientset.CoreV1().ConfigMaps("test-ns").Delete(context.Background(), "test-config", metav1.DeleteOptions{})
	require.NoError(t, err)

	// Allow time for the delete event to be processed
	time.Sleep(300 * time.Millisecond)

	// Old config should be preserved (empty-guard prevents clearing)
	AppConfig.Mu.RLock()
	assert.Equal(t, "value1", AppConfig.Config["key1"], "old config should be preserved after ConfigMap deletion")
	AppConfig.Mu.RUnlock()
}

func TestConfigInformer_SecretDelete(t *testing.T) {
	resetAppConfig()

	s := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "test-secret", Namespace: "test-ns"},
		Data:       map[string][]byte{"password": []byte("secret-value")},
	}
	clientset := fake.NewSimpleClientset(s)

	ci := NewConfigInformer(clientset, "test-ns", nil, []string{"test-secret"}, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["password"] == "secret-value"
	}, 5*time.Second, 50*time.Millisecond)

	// Delete the Secret
	err := clientset.CoreV1().Secrets("test-ns").Delete(context.Background(), "test-secret", metav1.DeleteOptions{})
	require.NoError(t, err)

	// Allow time for the delete event to be processed
	time.Sleep(300 * time.Millisecond)

	// Old config should be preserved (empty-guard prevents clearing)
	AppConfig.Mu.RLock()
	assert.Equal(t, "secret-value", AppConfig.Config["password"], "old config should be preserved after Secret deletion")
	AppConfig.Mu.RUnlock()
}

func TestConfigInformer_PreservesOldConfig(t *testing.T) {
	resetAppConfig()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "value1"},
	}
	clientset := fake.NewSimpleClientset(cm)

	ci := NewConfigInformer(clientset, "test-ns", []string{"test-config"}, nil, false, nil)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load
	require.Eventually(t, func() bool {
		AppConfig.Mu.RLock()
		defer AppConfig.Mu.RUnlock()
		return AppConfig.Config["key1"] == "value1"
	}, 5*time.Second, 50*time.Millisecond)

	// Update ConfigMap to have empty data - should trigger the empty-guard
	cm.Data = nil
	_, err := clientset.CoreV1().ConfigMaps("test-ns").Update(context.Background(), cm, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Allow time for the update event to be processed
	time.Sleep(300 * time.Millisecond)

	// Old config should be preserved
	AppConfig.Mu.RLock()
	assert.Equal(t, "value1", AppConfig.Config["key1"], "old config should be preserved when all sources return empty data")
	AppConfig.Mu.RUnlock()
}

func TestConfigInformer_AutoUpdateTrigger(t *testing.T) {
	resetAppConfig()

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: "test-config", Namespace: "test-ns"},
		Data:       map[string]string{"key1": "initial"},
	}
	clientset := fake.NewSimpleClientset(cm)

	// Create a fake Kustomization for the updater to patch
	ks := &unstructured.Unstructured{}
	ks.SetGroupVersionKind(schema.GroupVersionKind{Group: "kustomize.toolkit.fluxcd.io", Version: "v1", Kind: "Kustomization"})
	ks.SetName("test-ks")
	ks.SetNamespace("default")

	dynScheme := runtime.NewScheme()
	dynClient := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(dynScheme,
		map[schema.GroupVersionResource]string{kustomizationGVR: "KustomizationList"}, ks)

	updater := NewKustomizationUpdaterWithClient(dynClient, nil)
	ci := NewConfigInformer(clientset, "test-ns", []string{"test-config"}, nil, true, updater)

	go ci.Start()
	defer ci.Stop()

	// Wait for initial load and the auto-update triggered by initial config load
	require.Eventually(t, func() bool {
		for _, action := range dynClient.Actions() {
			if action.GetVerb() == "patch" {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond, "auto-update should patch kustomizations after initial config load")

	// Clear recorded actions and trigger another config change
	dynClient.ClearActions()

	cm.Data["key1"] = "updated"
	_, err := clientset.CoreV1().ConfigMaps("test-ns").Update(context.Background(), cm, metav1.UpdateOptions{})
	require.NoError(t, err)

	// Verify auto-update fires again after config change
	require.Eventually(t, func() bool {
		for _, action := range dynClient.Actions() {
			if action.GetVerb() == "patch" {
				return true
			}
		}
		return false
	}, 5*time.Second, 50*time.Millisecond, "auto-update should patch kustomizations after config change")
}

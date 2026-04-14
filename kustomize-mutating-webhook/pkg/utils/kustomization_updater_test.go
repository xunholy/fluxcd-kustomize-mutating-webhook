package utils

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8stesting "k8s.io/client-go/testing"
)

var kustomizationGVK = schema.GroupVersionKind{
	Group:   "kustomize.toolkit.fluxcd.io",
	Version: "v1",
	Kind:    "Kustomization",
}

func makeKustomization(name, namespace string) *unstructured.Unstructured {
	ks := &unstructured.Unstructured{}
	ks.SetGroupVersionKind(kustomizationGVK)
	ks.SetName(name)
	ks.SetNamespace(namespace)
	return ks
}

func newFakeUpdater(excludeNamespaces []string, objs ...runtime.Object) (*KustomizationUpdater, *dynamicfake.FakeDynamicClient) {
	scheme := runtime.NewScheme()
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(scheme,
		map[schema.GroupVersionResource]string{kustomizationGVR: "KustomizationList"}, objs...)
	return NewKustomizationUpdaterWithClient(client, excludeNamespaces), client
}

func TestTriggerUpdateAll_PatchesKustomizations(t *testing.T) {
	ks1 := makeKustomization("ks1", "default")
	ks2 := makeKustomization("ks2", "other-ns")

	updater, client := newFakeUpdater(nil, ks1, ks2)
	err := updater.TriggerUpdateAll()
	require.NoError(t, err)

	patchCount := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "patch" {
			patchCount++
		}
	}
	assert.Equal(t, 2, patchCount, "should have patched both kustomizations")
}

func TestTriggerUpdateAll_ExcludesNamespaces(t *testing.T) {
	ks1 := makeKustomization("ks1", "flux-system") // excluded
	ks2 := makeKustomization("ks2", "default")      // not excluded

	updater, client := newFakeUpdater([]string{"flux-system"}, ks1, ks2)
	err := updater.TriggerUpdateAll()
	require.NoError(t, err)

	patchCount := 0
	for _, action := range client.Actions() {
		if action.GetVerb() == "patch" {
			assert.Equal(t, "default", action.GetNamespace(), "should only patch non-excluded namespace")
			patchCount++
		}
	}
	assert.Equal(t, 1, patchCount, "should have patched only the non-excluded kustomization")
}

func TestTriggerUpdateAll_PartialFailure(t *testing.T) {
	ks1 := makeKustomization("ks1", "default")
	ks2 := makeKustomization("ks2", "other-ns")

	updater, client := newFakeUpdater(nil, ks1, ks2)

	// Fail patch for ks1
	client.PrependReactor("patch", "kustomizations", func(action k8stesting.Action) (bool, runtime.Object, error) {
		pa := action.(k8stesting.PatchAction)
		if pa.GetName() == "ks1" {
			return true, nil, fmt.Errorf("simulated patch failure")
		}
		return false, nil, nil
	})

	err := updater.TriggerUpdateAll()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to update 1/2")
}

func TestTriggerUpdateAll_EmptyList(t *testing.T) {
	updater, _ := newFakeUpdater(nil)
	err := updater.TriggerUpdateAll()
	assert.NoError(t, err)
}

func TestIsExcluded(t *testing.T) {
	tests := []struct {
		name      string
		excluded  []string
		namespace string
		want      bool
	}{
		{"empty exclude list", nil, "default", false},
		{"namespace in list", []string{"flux-system"}, "flux-system", true},
		{"namespace not in list", []string{"flux-system"}, "default", false},
		{"multiple exclusions match", []string{"flux-system", "kube-system"}, "kube-system", true},
		{"multiple exclusions no match", []string{"flux-system", "kube-system"}, "default", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ku := &KustomizationUpdater{excludeNamespaces: tt.excluded}
			assert.Equal(t, tt.want, ku.isExcluded(tt.namespace))
		})
	}
}

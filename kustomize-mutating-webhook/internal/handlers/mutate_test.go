package handlers

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xunholy/fluxcd-mutating-webhook/pkg/utils"
	admissionv1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestHandleMutate(t *testing.T) {
	// Set up test config
	utils.AppConfig.Config = map[string]string{
		"TEST_KEY": "test_value",
	}

	tests := []struct {
		name            string
		inputObject     map[string]interface{}
		kind            metav1.GroupVersionKind
		expectedPatch   []map[string]interface{}
		expectedAllowed bool
	}{
		{
			name: "Add postBuild and substitute",
			inputObject: map[string]interface{}{
				"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
				"kind":       "Kustomization",
				"metadata": map[string]interface{}{
					"name":      "test-kustomization",
					"namespace": "default",
				},
				"spec": map[string]interface{}{},
			},
			kind: metav1.GroupVersionKind{
				Group:   "kustomize.toolkit.fluxcd.io",
				Version: "v1",
				Kind:    "Kustomization",
			},
			expectedPatch: []map[string]interface{}{
				{
					"op":    "add",
					"path":  "/spec/postBuild",
					"value": map[string]interface{}{},
				},
				{
					"op":    "add",
					"path":  "/spec/postBuild/substitute",
					"value": map[string]interface{}{},
				},
				{
					"op":    "add",
					"path":  "/spec/postBuild/substitute/TEST_KEY",
					"value": "test_value",
				},
			},
			expectedAllowed: true,
		},
		{
			name: "No mutation for non-Kustomization resource",
			inputObject: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "ConfigMap",
				"metadata": map[string]interface{}{
					"name":      "test-configmap",
					"namespace": "default",
				},
				"data": map[string]interface{}{},
			},
			kind: metav1.GroupVersionKind{
				Group:   "",
				Version: "v1",
				Kind:    "ConfigMap",
			},
			expectedPatch:   nil,
			expectedAllowed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create admission review request
			objBytes, err := json.Marshal(tt.inputObject)
			require.NoError(t, err)

			ar := admissionv1.AdmissionReview{
				Request: &admissionv1.AdmissionRequest{
					Object:    runtime.RawExtension{Raw: objBytes},
					Kind:      tt.kind,
					Operation: admissionv1.Create,
				},
			}

			arBytes, err := json.Marshal(ar)
			require.NoError(t, err)

			// Create request
			req, err := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
			require.NoError(t, err)
			req.Header.Set("Content-Type", "application/json")

			// Create response recorder
			rr := httptest.NewRecorder()

			// Call the handler
			HandleMutate(rr, req)

			// Check the status code
			assert.Equal(t, http.StatusOK, rr.Code)

			// Parse the response
			var respAR admissionv1.AdmissionReview
			err = json.Unmarshal(rr.Body.Bytes(), &respAR)
			require.NoError(t, err)

			// Check the response
			assert.Equal(t, tt.expectedAllowed, respAR.Response.Allowed)

			if tt.expectedPatch != nil {
				var patch []map[string]interface{}
				err = json.Unmarshal(respAR.Response.Patch, &patch)
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPatch, patch)
			} else {
				assert.Nil(t, respAR.Response.Patch)
			}
		})
	}
}

func TestCreatePatch(t *testing.T) {
	utils.AppConfig.Config = map[string]string{
		"TEST_KEY": "test_value",
	}

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{},
		},
	}

	patch := createPatch(obj)

	expectedPatch := []map[string]interface{}{
		{
			"op":    "add",
			"path":  "/spec/postBuild",
			"value": map[string]interface{}{},
		},
		{
			"op":    "add",
			"path":  "/spec/postBuild/substitute",
			"value": map[string]interface{}{},
		},
		{
			"op":    "add",
			"path":  "/spec/postBuild/substitute/TEST_KEY",
			"value": "test_value",
		},
	}

	assert.Equal(t, expectedPatch, patch)
}

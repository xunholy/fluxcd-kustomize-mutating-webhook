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
	// Set up test config with proper mutex locking
	utils.AppConfig.Mu.Lock()
	utils.AppConfig.Config = map[string]string{
		"TEST_KEY": "test_value",
	}
	utils.AppConfig.Mu.Unlock()

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
				// First two entries are structural (postBuild, substitute) and order-dependent.
				// Remaining entries are config keys from map iteration — order is non-deterministic.
				require.GreaterOrEqual(t, len(patch), 2)
				assert.Equal(t, tt.expectedPatch[:2], patch[:2])
				assert.ElementsMatch(t, tt.expectedPatch[2:], patch[2:])
			} else {
				assert.Nil(t, respAR.Response.Patch)
			}
		})
	}
}

// TestHandleMutate_NilRequest verifies that a decoded AdmissionReview with a nil
// Request field returns a denied AdmissionReview JSON response instead of panicking.
func TestHandleMutate_NilRequest(t *testing.T) {
	ar := admissionv1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		// Request intentionally omitted (nil)
	}

	arBytes, err := json.Marshal(ar)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleMutate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var respAR admissionv1.AdmissionReview
	err = json.Unmarshal(rr.Body.Bytes(), &respAR)
	require.NoError(t, err)

	assert.False(t, respAR.Response.Allowed)
	require.NotNil(t, respAR.Response.Result)
	assert.Equal(t, "Request is nil", respAR.Response.Result.Message)
}

// TestHandleMutate_DeleteOperation verifies that DELETE requests are allowed
// without attempting to unmarshal the (absent) Object body.
func TestHandleMutate_DeleteOperation(t *testing.T) {
	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Operation: admissionv1.Delete,
			Kind: metav1.GroupVersionKind{
				Group:   "kustomize.toolkit.fluxcd.io",
				Version: "v1",
				Kind:    "Kustomization",
			},
			// No Object field — DELETE requests have nil Object.Raw
		},
	}

	arBytes, err := json.Marshal(ar)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleMutate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var respAR admissionv1.AdmissionReview
	err = json.Unmarshal(rr.Body.Bytes(), &respAR)
	require.NoError(t, err)

	assert.True(t, respAR.Response.Allowed)
	assert.Nil(t, respAR.Response.Patch)
}

// TestHandleMutate_DecodeError verifies that an invalid request body returns a
// denied AdmissionReview JSON response (not plain text).
func TestHandleMutate_DecodeError(t *testing.T) {
	req, err := http.NewRequest("POST", "/mutate", bytes.NewBufferString("not valid json"))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleMutate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var respAR admissionv1.AdmissionReview
	err = json.Unmarshal(rr.Body.Bytes(), &respAR)
	require.NoError(t, err)

	assert.False(t, respAR.Response.Allowed)
}

// TestHandleMutate_UnmarshalError verifies that a Kustomization whose Object body
// is invalid JSON returns a denied AdmissionReview JSON response.
func TestHandleMutate_UnmarshalError(t *testing.T) {
	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Operation: admissionv1.Create,
			Kind: metav1.GroupVersionKind{
				Group:   "kustomize.toolkit.fluxcd.io",
				Version: "v1",
				Kind:    "Kustomization",
			},
			// A JSON array is valid JSON but unstructured.Unstructured requires an object.
			Object: runtime.RawExtension{Raw: []byte(`["not","an","object"]`)},
		},
	}

	arBytes, err := json.Marshal(ar)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleMutate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)
	assert.Equal(t, "application/json", rr.Header().Get("Content-Type"))

	var respAR admissionv1.AdmissionReview
	err = json.Unmarshal(rr.Body.Bytes(), &respAR)
	require.NoError(t, err)

	assert.False(t, respAR.Response.Allowed)
	require.NotNil(t, respAR.Response.Result)
	assert.Equal(t, "Failed to unmarshal Object", respAR.Response.Result.Message)
}

// TestCreatePatch_ExistingPostBuild verifies that when spec.postBuild already exists
// (but spec.postBuild.substitute does not), createPatch omits the /spec/postBuild add
// op but still adds the /spec/postBuild/substitute op and all config key ops.
func TestCreatePatch_ExistingPostBuild(t *testing.T) {
	utils.AppConfig.Mu.Lock()
	utils.AppConfig.Config = map[string]string{
		"TEST_KEY": "test_value",
	}
	utils.AppConfig.Mu.Unlock()

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"postBuild": map[string]interface{}{},
		},
	}}

	patch := createPatch(obj)

	for _, op := range patch {
		assert.NotEqual(t, "/spec/postBuild", op["path"], "patch must not add /spec/postBuild when it already exists")
	}

	var foundSubstitute bool
	for _, op := range patch {
		if op["path"] == "/spec/postBuild/substitute" {
			foundSubstitute = true
			break
		}
	}
	assert.True(t, foundSubstitute, "patch must include /spec/postBuild/substitute add op")
}

// TestCreatePatch_ExistingPostBuildAndSubstitute verifies that when both spec.postBuild
// and spec.postBuild.substitute already exist, createPatch omits both structural ops and
// only emits the individual config key ops.
func TestCreatePatch_ExistingPostBuildAndSubstitute(t *testing.T) {
	utils.AppConfig.Mu.Lock()
	utils.AppConfig.Config = map[string]string{
		"TEST_KEY": "test_value",
	}
	utils.AppConfig.Mu.Unlock()

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"spec": map[string]interface{}{
			"postBuild": map[string]interface{}{
				"substitute": map[string]interface{}{
					"EXISTING_KEY": "existing_value",
				},
			},
		},
	}}

	patch := createPatch(obj)

	for _, op := range patch {
		assert.NotEqual(t, "/spec/postBuild", op["path"], "patch must not add /spec/postBuild when it already exists")
		assert.NotEqual(t, "/spec/postBuild/substitute", op["path"], "patch must not add /spec/postBuild/substitute when it already exists")
	}

	assert.ElementsMatch(t, []map[string]interface{}{
		{"op": "add", "path": "/spec/postBuild/substitute/TEST_KEY", "value": "test_value"},
	}, patch)
}

// TestHandleMutate_DeletionTimestamp verifies that a Kustomization with a non-zero
// DeletionTimestamp is allowed through with no patch applied.
func TestHandleMutate_DeletionTimestamp(t *testing.T) {
	inputObject := map[string]interface{}{
		"apiVersion": "kustomize.toolkit.fluxcd.io/v1",
		"kind":       "Kustomization",
		"metadata": map[string]interface{}{
			"name":              "test-kustomization",
			"namespace":         "default",
			"deletionTimestamp": "2026-01-01T00:00:00Z",
		},
		"spec": map[string]interface{}{},
	}

	objBytes, err := json.Marshal(inputObject)
	require.NoError(t, err)

	ar := admissionv1.AdmissionReview{
		Request: &admissionv1.AdmissionRequest{
			Object: runtime.RawExtension{Raw: objBytes},
			Kind: metav1.GroupVersionKind{
				Group:   "kustomize.toolkit.fluxcd.io",
				Version: "v1",
				Kind:    "Kustomization",
			},
			Operation: admissionv1.Update,
		},
	}

	arBytes, err := json.Marshal(ar)
	require.NoError(t, err)

	req, err := http.NewRequest("POST", "/mutate", bytes.NewBuffer(arBytes))
	require.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	rr := httptest.NewRecorder()
	HandleMutate(rr, req)

	assert.Equal(t, http.StatusOK, rr.Code)

	var respAR admissionv1.AdmissionReview
	err = json.Unmarshal(rr.Body.Bytes(), &respAR)
	require.NoError(t, err)

	assert.True(t, respAR.Response.Allowed)
	assert.Nil(t, respAR.Response.Patch)
}

// TestCreatePatch_MultipleConfigKeys verifies that createPatch emits an op for every
// key in AppConfig.Config regardless of map iteration order.
func TestCreatePatch_MultipleConfigKeys(t *testing.T) {
	utils.AppConfig.Mu.Lock()
	utils.AppConfig.Config = map[string]string{
		"KEY_ONE":   "value_one",
		"KEY_TWO":   "value_two",
		"KEY_THREE": "value_three",
	}
	utils.AppConfig.Mu.Unlock()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{},
		},
	}

	patch := createPatch(obj)

	require.GreaterOrEqual(t, len(patch), 2)
	assert.Equal(t, "/spec/postBuild", patch[0]["path"])
	assert.Equal(t, "/spec/postBuild/substitute", patch[1]["path"])

	assert.ElementsMatch(t, []map[string]interface{}{
		{"op": "add", "path": "/spec/postBuild/substitute/KEY_ONE", "value": "value_one"},
		{"op": "add", "path": "/spec/postBuild/substitute/KEY_TWO", "value": "value_two"},
		{"op": "add", "path": "/spec/postBuild/substitute/KEY_THREE", "value": "value_three"},
	}, patch[2:])
}

func TestCreatePatch(t *testing.T) {
	// Set up test config with proper mutex locking
	utils.AppConfig.Mu.Lock()
	utils.AppConfig.Config = map[string]string{
		"TEST_KEY": "test_value",
	}
	utils.AppConfig.Mu.Unlock()

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

	require.GreaterOrEqual(t, len(patch), 2)
	assert.Equal(t, expectedPatch[:2], patch[:2])
	assert.ElementsMatch(t, expectedPatch[2:], patch[2:])
}

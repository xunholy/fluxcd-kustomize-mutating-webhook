package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	jsoniter "github.com/json-iterator/go"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/rs/zerolog/log"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/metrics"
	"github.com/xunholy/fluxcd-mutating-webhook/pkg/utils"
	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func HandleMutate(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	var admissionReviewReq v1.AdmissionReview

	if err := jsoniter.NewDecoder(r.Body).Decode(&admissionReviewReq); err != nil {
		log.Error().Err(err).Msg("Failed to decode AdmissionReview request")
		metrics.ErrorCount.With(prometheus.Labels{"error_type": "decode_error"}).Inc()
		http.Error(w, "Could not decode request", http.StatusBadRequest)
		return
	}

	resourceKind := admissionReviewReq.Request.Kind.Kind
	operation := string(admissionReviewReq.Request.Operation)
	metrics.TotalRequests.With(prometheus.Labels{"resource_kind": resourceKind, "operation": operation}).Inc()

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

	if resourceKind != "Kustomization" {
		log.Info().Msgf("Skipping mutation for non-Kustomization resource: %s", resourceKind)
		respondWithAdmissionReview(w, admissionResponse)
		metrics.RequestDuration.With(prometheus.Labels{"resource_kind": resourceKind, "operation": operation}).Observe(time.Since(startTime).Seconds())
		return
	}

	var obj unstructured.Unstructured
	if err := json.Unmarshal(admissionReviewReq.Request.Object.Raw, &obj); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal Object")
		metrics.ErrorCount.With(prometheus.Labels{"error_type": "unmarshal_error"}).Inc()
		http.Error(w, "Failed to unmarshal Object", http.StatusBadRequest)
		return
	}

	if admissionReviewReq.Request.Operation == v1.Delete || !obj.GetDeletionTimestamp().IsZero() {
		respondWithAdmissionReview(w, admissionResponse)
		metrics.RequestDuration.With(prometheus.Labels{"resource_kind": resourceKind, "operation": operation}).Observe(time.Since(startTime).Seconds())
		return
	}

	log.Info().
		Str("UID", string(admissionReviewReq.Request.UID)).
		Str("Kind", resourceKind).
		Str("Resource", admissionReviewReq.Request.Resource.Resource).
		Str("Name", admissionReviewReq.Request.Name).
		Str("Namespace", admissionReviewReq.Request.Namespace).
		Msg("Request details")

	patch := createPatch(&obj)

	if len(patch) > 0 {
		patchBytes, _ := json.Marshal(patch)
		admissionResponse.Response.Patch = patchBytes
		pt := v1.PatchTypeJSONPatch
		admissionResponse.Response.PatchType = &pt

		log.Debug().
			Str("Patch", string(patchBytes)).
			Msg("Applying mutation to resource")

		metrics.MutationCount.With(prometheus.Labels{"resource_kind": resourceKind}).Inc()
	}

	respondWithAdmissionReview(w, admissionResponse)
	metrics.RequestDuration.With(prometheus.Labels{"resource_kind": resourceKind, "operation": operation}).Observe(time.Since(startTime).Seconds())
}

func createPatch(obj *unstructured.Unstructured) []map[string]interface{} {
	var patch []map[string]interface{}

	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "postBuild"); !found {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild",
			"value": map[string]interface{}{},
		})
	}

	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "postBuild", "substitute"); !found {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild/substitute",
			"value": map[string]interface{}{},
		})
	}

	for key := range utils.AppConfig.Config {
		if configValue, ok := utils.GetAppConfig(key); ok {
			escapedKey := utils.EscapeJsonPointer(key)
			patch = append(patch, map[string]interface{}{
				"op":    "add",
				"path":  "/spec/postBuild/substitute/" + escapedKey,
				"value": configValue,
			})
		}
	}

	return patch
}

func respondWithAdmissionReview(w http.ResponseWriter, admissionResponse v1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(admissionResponse); err != nil {
		log.Error().Err(err).Msg("Failed to encode AdmissionReview response")
		http.Error(w, "Could not encode response", http.StatusInternalServerError)
	}
}

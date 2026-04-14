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
	"k8s.io/apimachinery/pkg/types"
)

// deniedAdmissionReview builds a denied AdmissionReview response with a status message.
func deniedAdmissionReview(uid types.UID, message string) v1.AdmissionReview {
	return v1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &v1.AdmissionResponse{
			UID:     uid,
			Allowed: false,
			Result: &metav1.Status{
				Message: message,
			},
		},
	}
}

func HandleMutate(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()
	var admissionReviewReq v1.AdmissionReview

	if err := jsoniter.NewDecoder(r.Body).Decode(&admissionReviewReq); err != nil {
		log.Error().Err(err).Msg("Failed to decode AdmissionReview request")
		metrics.ErrorCount.With(prometheus.Labels{"error_type": "decode_error"}).Inc()
		respondWithAdmissionReview(w, deniedAdmissionReview("", "Could not decode request"))
		return
	}

	if admissionReviewReq.Request == nil {
		log.Error().Msg("AdmissionReview request field is nil")
		metrics.ErrorCount.With(prometheus.Labels{"error_type": "nil_request"}).Inc()
		respondWithAdmissionReview(w, deniedAdmissionReview("", "Request is nil"))
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

	if admissionReviewReq.Request.Operation == v1.Delete {
		respondWithAdmissionReview(w, admissionResponse)
		metrics.RequestDuration.With(prometheus.Labels{"resource_kind": resourceKind, "operation": operation}).Observe(time.Since(startTime).Seconds())
		return
	}

	var obj unstructured.Unstructured
	if err := json.Unmarshal(admissionReviewReq.Request.Object.Raw, &obj); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal Object")
		metrics.ErrorCount.With(prometheus.Labels{"error_type": "unmarshal_error"}).Inc()
		respondWithAdmissionReview(w, deniedAdmissionReview(admissionReviewReq.Request.UID, "Failed to unmarshal Object"))
		return
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		respondWithAdmissionReview(w, admissionResponse)
		metrics.RequestDuration.With(prometheus.Labels{"resource_kind": resourceKind, "operation": operation}).Observe(time.Since(startTime).Seconds())
		return
	}

	log.Info().
		Str("uid", string(admissionReviewReq.Request.UID)).
		Str("kind", resourceKind).
		Str("resource", admissionReviewReq.Request.Resource.Resource).
		Str("name", admissionReviewReq.Request.Name).
		Str("namespace", admissionReviewReq.Request.Namespace).
		Msg("Request details")

	patch := createPatch(&obj)

	if len(patch) > 0 {
		patchBytes, err := json.Marshal(patch)
		if err != nil {
			log.Error().Err(err).Msg("Failed to marshal patch")
			metrics.ErrorCount.With(prometheus.Labels{"error_type": "marshal_error"}).Inc()
			respondWithAdmissionReview(w, deniedAdmissionReview(admissionReviewReq.Request.UID, "Failed to marshal patch"))
			return
		}
		admissionResponse.Response.Patch = patchBytes
		pt := v1.PatchTypeJSONPatch
		admissionResponse.Response.PatchType = &pt

		log.Debug().
			Str("patch", string(patchBytes)).
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

	// Acquire read lock before iterating to ensure we see a consistent snapshot of the config
	// This prevents a race condition where the config might be reloaded mid-iteration
	utils.AppConfig.Mu.RLock()
	configSnapshot := make(map[string]string, len(utils.AppConfig.Config))
	for key, value := range utils.AppConfig.Config {
		configSnapshot[key] = value
	}
	utils.AppConfig.Mu.RUnlock()

	// Iterate over the snapshot, not the live config map
	for key, value := range configSnapshot {
		escapedKey := utils.EscapeJsonPointer(key)
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild/substitute/" + escapedKey,
			"value": value,
		})
	}

	return patch
}

func respondWithAdmissionReview(w http.ResponseWriter, admissionResponse v1.AdmissionReview) {
	respBytes, err := json.Marshal(admissionResponse)
	if err != nil {
		log.Error().Err(err).Msg("Failed to encode AdmissionReview response")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if _, err := w.Write(respBytes); err != nil {
		log.Error().Err(err).Msg("Failed to write AdmissionReview response")
	}
}

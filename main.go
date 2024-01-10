package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/rs/zerolog"
	log "github.com/rs/zerolog/log"

	v1 "k8s.io/api/admission/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

var appConfig map[string]string

func init() {
	// Set up logging to console
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	// Set up colored output
	consoleWriter := zerolog.ConsoleWriter{Out: os.Stderr, TimeFormat: zerolog.TimeFieldFormat, NoColor: false}
	log.Logger = log.Output(consoleWriter)

	// Initialize log level to Info as default
	var level zerolog.Level = zerolog.InfoLevel

	// Determine log level from environment variable
	if logLevel, ok := os.LookupEnv("LOG_LEVEL"); ok {
		var err error
		level, err = zerolog.ParseLevel(logLevel)
		if err != nil {
			log.Warn().Msgf("Invalid log level '%s'. Falling back to '%s'", logLevel, level)
		}
	}

	// Set the global log level
	zerolog.SetGlobalLevel(level)

	log.Info().Msgf("Log level set to '%s'", level.String())
}

func readConfigMap(directory string) map[string]string {
	config := make(map[string]string)

	files, err := os.ReadDir(directory)
	if err != nil {
		log.Error().Err(err).Msg("Error reading directory")
	}

	for _, file := range files {
		fileName := file.Name()

		if file.IsDir() || strings.HasPrefix(fileName, ".") {
			// Skip Directories
			continue
		}

		fullPath := filepath.Join(directory, fileName)
		value, err := os.ReadFile(fullPath)
		if err != nil {
			log.Error().Err(err).Msgf("Error reading file %s", fullPath)
			continue
		}
		config[fileName] = string(value)
	}
	return config
}

func handleMutate(w http.ResponseWriter, r *http.Request) {
	var admissionReviewReq v1.AdmissionReview

	// Decode the incoming AdmissionReview
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&admissionReviewReq); err != nil {
		log.Error().Err(err).Msg("Failed to decode AdmissionReview request")
		http.Error(w, "Could not decode request", http.StatusBadRequest)
		return
	}

	// Unmarshal Object into unstructured.Unstructured
	var obj unstructured.Unstructured
	if err := json.Unmarshal(admissionReviewReq.Request.Object.Raw, &obj); err != nil {
		log.Error().Err(err).Msg("Failed to unmarshal Object")
		http.Error(w, "Failed to unmarshal Object", http.StatusBadRequest)
		return
	}

	// Check if the resource is being deleted
	if admissionReviewReq.Request.Operation == v1.Delete || !obj.GetDeletionTimestamp().IsZero() {
		// Allow the deletion to proceed without modification
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

		// Send the response to avoid locking deletion
		respondWithAdmissionReview(w, admissionResponse)
		return
	}

	// Log details of the request
	log.Info().
		Str("UID", string(admissionReviewReq.Request.UID)).
		Str("Kind", admissionReviewReq.Request.Kind.Kind).
		Str("Resource", admissionReviewReq.Request.Resource.Resource).
		Str("Name", admissionReviewReq.Request.Name).
		Str("Namespace", admissionReviewReq.Request.Namespace).
		Msg("Request details")

	// Process the AdmissionReview request and create a patch
	var patch []map[string]interface{}

	// Ensure /spec/postBuild exists
	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "postBuild"); !found {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild",
			"value": map[string]interface{}{},
		})
	}

	// Ensure /spec/postBuild/substitute exists
	if _, found, _ := unstructured.NestedMap(obj.Object, "spec", "postBuild", "substitute"); !found {
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild/substitute",
			"value": map[string]interface{}{},
		})
	}

	// Add key-value pairs from appConfig to /spec/postBuild/substitute
	for key, value := range appConfig {
		escapedKey := escapeJsonPointer(key)
		patch = append(patch, map[string]interface{}{
			"op":    "add",
			"path":  "/spec/postBuild/substitute/" + escapedKey,
			"value": value,
		})
	}
	patchBytes, _ := json.Marshal(patch)

	// Log the mutation details
	log.Debug().
		Str("Patch", string(patchBytes)).
		Msg("Applying mutation to resource")

	// Create the AdmissionReview response
	admissionResponse := v1.AdmissionReview{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
		},
		Response: &v1.AdmissionResponse{
			UID:     admissionReviewReq.Request.UID,
			Allowed: true,
			Patch:   patchBytes,
			PatchType: func() *v1.PatchType {
				pt := v1.PatchTypeJSONPatch
				return &pt
			}(),
		},
	}

	respondWithAdmissionReview(w, admissionResponse)
}

// Encodes and sends the AdmissionReview response
func respondWithAdmissionReview(w http.ResponseWriter, admissionResponse v1.AdmissionReview) {
	w.Header().Set("Content-Type", "application/json")
	encoder := json.NewEncoder(w)
	if err := encoder.Encode(admissionResponse); err != nil {
		log.Error().Err(err).Msg("Failed to encode AdmissionReview response")
		http.Error(w, "Could not encode response", http.StatusInternalServerError)
	}
}

// escapeJsonPointer escapes special characters in JSON pointer
func escapeJsonPointer(value string) string {
	value = strings.ReplaceAll(value, "~", "~0")
	value = strings.ReplaceAll(value, "/", "~1")
	return value
}

func main() {
	// Log the starting of the server
	log.Info().Msg("Starting the webhook server on port 8443")

	// Load the ConfigMap with cluster config
	configMapPath := "/etc/config"
	appConfig = readConfigMap(configMapPath)

	log.Debug().Msg("Loaded appConfig:")
	for key, value := range appConfig {
		log.Debug().Msgf("Config - Key: %s, Value: %s", key, value)
	}

	// Set up HTTP handler and server
	http.HandleFunc("/mutate", handleMutate)
	if err := http.ListenAndServeTLS(":8443", "/etc/webhook/certs/tls.crt", "/etc/webhook/certs/tls.key", nil); err != nil {
		log.Fatal().Err(err).Msg("Failed to start server")
		os.Exit(1)
	}
}

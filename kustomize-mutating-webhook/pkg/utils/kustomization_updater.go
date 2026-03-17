package utils

import (
	"context"
	"fmt"
	"time"

	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var kustomizationGVR = schema.GroupVersionResource{
	Group:    "kustomize.toolkit.fluxcd.io",
	Version:  "v1",
	Resource: "kustomizations",
}

type KustomizationUpdater struct {
	client            dynamic.Interface
	excludeNamespaces []string
}

// NewKustomizationUpdater creates a new Kustomization updater
func NewKustomizationUpdater(excludeNamespaces []string) (*KustomizationUpdater, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	client, err := dynamic.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create dynamic client: %w", err)
	}

	return &KustomizationUpdater{
		client:            client,
		excludeNamespaces: excludeNamespaces,
	}, nil
}

// TriggerUpdateAll annotates all Kustomizations to trigger webhook mutation
func (ku *KustomizationUpdater) TriggerUpdateAll() error {
	ctx := context.Background()

	// List all Kustomizations across all namespaces
	list, err := ku.client.Resource(kustomizationGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list kustomizations: %w", err)
	}

	annotationKey := "webhook.kustomize-mutating-webhook/config-reload"
	annotationValue := time.Now().Format(time.RFC3339)

	updated := 0
	skipped := 0

	for _, item := range list.Items {
		namespace := item.GetNamespace()
		name := item.GetName()

		// Skip excluded namespaces
		if ku.isExcluded(namespace) {
			log.Debug().
				Str("namespace", namespace).
				Str("name", name).
				Msg("Skipping Kustomization in excluded namespace")
			skipped++
			continue
		}

		// Get current annotations
		annotations := item.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}

		// Add/update annotation
		annotations[annotationKey] = annotationValue
		item.SetAnnotations(annotations)

		// Update the Kustomization
		_, err := ku.client.Resource(kustomizationGVR).Namespace(namespace).Update(ctx, &item, metav1.UpdateOptions{})
		if err != nil {
			log.Error().
				Err(err).
				Str("namespace", namespace).
				Str("name", name).
				Msg("Failed to update Kustomization")
			continue
		}

		log.Debug().
			Str("namespace", namespace).
			Str("name", name).
			Msg("Triggered update on Kustomization")
		updated++
	}

	log.Info().
		Int("updated", updated).
		Int("skipped", skipped).
		Int("total", len(list.Items)).
		Msg("Triggered Kustomization updates after config reload")

	return nil
}

// isExcluded checks if a namespace is in the exclude list
func (ku *KustomizationUpdater) isExcluded(namespace string) bool {
	for _, excluded := range ku.excludeNamespaces {
		if excluded == namespace {
			return true
		}
	}
	return false
}

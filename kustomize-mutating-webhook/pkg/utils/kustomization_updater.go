package utils

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/rs/zerolog/log"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
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

// NewKustomizationUpdaterWithClient creates a new Kustomization updater with a provided client.
func NewKustomizationUpdaterWithClient(client dynamic.Interface, excludeNamespaces []string) *KustomizationUpdater {
	return &KustomizationUpdater{client: client, excludeNamespaces: excludeNamespaces}
}

// TriggerUpdateAll annotates all Kustomizations to trigger webhook mutation
func (ku *KustomizationUpdater) TriggerUpdateAll() error {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	annotationKey := "webhook.kustomize-mutating-webhook/config-reload"
	annotationValue := time.Now().Format(time.RFC3339Nano)

	updated := 0
	skipped := 0
	failCount := 0

	var continueToken string
	for {
		list, err := ku.client.Resource(kustomizationGVR).List(ctx, metav1.ListOptions{Limit: 100, Continue: continueToken})
		if err != nil {
			return fmt.Errorf("failed to list kustomizations: %w", err)
		}

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

			patch := map[string]interface{}{
				"metadata": map[string]interface{}{
					"annotations": map[string]string{annotationKey: annotationValue},
				},
			}
			patchData, err := json.Marshal(patch)
			if err != nil {
				log.Error().Err(err).Msg("Failed to marshal patch data")
				failCount++
				continue
			}
			_, err = ku.client.Resource(kustomizationGVR).Namespace(namespace).Patch(ctx, name, types.MergePatchType, patchData, metav1.PatchOptions{})
			if err != nil {
				log.Error().
					Err(err).
					Str("namespace", namespace).
					Str("name", name).
					Msg("Failed to patch Kustomization")
				failCount++
				continue
			}

			log.Debug().
				Str("namespace", namespace).
				Str("name", name).
				Msg("Triggered update on Kustomization")
			updated++
		}

		continueToken = list.GetContinue()
		if continueToken == "" {
			break
		}
	}

	attempted := updated + failCount
	log.Info().
		Int("updated", updated).
		Int("skipped", skipped).
		Int("failed", failCount).
		Int("attempted", attempted).
		Msg("Triggered Kustomization updates after config reload")

	if failCount > 0 {
		return fmt.Errorf("failed to update %d/%d kustomizations", failCount, attempted)
	}

	return nil
}

// isExcluded checks if a namespace is in the exclude list
func (ku *KustomizationUpdater) isExcluded(namespace string) bool {
	return slices.Contains(ku.excludeNamespaces, namespace)
}

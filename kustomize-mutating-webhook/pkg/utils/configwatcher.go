package utils

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/xunholy/fluxcd-mutating-webhook/internal/metrics"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

const (
	resyncPeriod = 5 * time.Minute
)

type ConfigInformer struct {
	clientset            kubernetes.Interface
	namespace            string
	configMapNames       map[string]bool
	secretNames          map[string]bool
	factory              informers.SharedInformerFactory
	kustomizationUpdater *KustomizationUpdater
	autoUpdate           bool
	updateCh             chan struct{}
	done                 chan struct{}
	stopOnce             sync.Once
}

func NewConfigInformer(
	clientset kubernetes.Interface,
	namespace string,
	configMapNames []string,
	secretNames []string,
	autoUpdate bool,
	kustomizationUpdater *KustomizationUpdater,
) *ConfigInformer {
	cmNames := make(map[string]bool, len(configMapNames))
	for _, name := range configMapNames {
		if name != "" {
			cmNames[name] = true
		}
	}
	sNames := make(map[string]bool, len(secretNames))
	for _, name := range secretNames {
		if name != "" {
			sNames[name] = true
		}
	}

	factory := informers.NewSharedInformerFactoryWithOptions(
		clientset,
		resyncPeriod,
		informers.WithNamespace(namespace),
	)

	ci := &ConfigInformer{
		clientset:            clientset,
		namespace:            namespace,
		configMapNames:       cmNames,
		secretNames:          sNames,
		factory:              factory,
		kustomizationUpdater: kustomizationUpdater,
		autoUpdate:           autoUpdate,
		updateCh:             make(chan struct{}, 1),
		done:                 make(chan struct{}),
	}

	// Register event handlers for ConfigMaps
	if len(cmNames) > 0 {
		factory.Core().V1().ConfigMaps().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				cm := obj.(*corev1.ConfigMap)
				if cmNames[cm.Name] {
					log.Info().Str("configmap", cm.Name).Msg("ConfigMap added, reloading config")
					ci.reloadConfig()
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				cm := newObj.(*corev1.ConfigMap)
				if cmNames[cm.Name] {
					log.Info().Str("configmap", cm.Name).Msg("ConfigMap updated, reloading config")
					ci.reloadConfig()
				}
			},
			DeleteFunc: func(obj interface{}) {
				if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
					obj = d.Obj
				}
				cm, ok := obj.(*corev1.ConfigMap)
				if !ok {
					return
				}
				if cmNames[cm.Name] {
					log.Info().Str("configmap", cm.Name).Msg("ConfigMap deleted, reloading config")
					ci.reloadConfig()
				}
			},
		})
	}

	// Register event handlers for Secrets
	if len(sNames) > 0 {
		factory.Core().V1().Secrets().Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				s := obj.(*corev1.Secret)
				if sNames[s.Name] {
					log.Info().Str("secret", s.Name).Msg("Secret added, reloading config")
					ci.reloadConfig()
				}
			},
			UpdateFunc: func(oldObj, newObj interface{}) {
				s := newObj.(*corev1.Secret)
				if sNames[s.Name] {
					log.Info().Str("secret", s.Name).Msg("Secret updated, reloading config")
					ci.reloadConfig()
				}
			},
			DeleteFunc: func(obj interface{}) {
				if d, ok := obj.(cache.DeletedFinalStateUnknown); ok {
					obj = d.Obj
				}
				s, ok := obj.(*corev1.Secret)
				if !ok {
					return
				}
				if sNames[s.Name] {
					log.Info().Str("secret", s.Name).Msg("Secret deleted, reloading config")
					ci.reloadConfig()
				}
			},
		})
	}

	return ci
}

func (ci *ConfigInformer) reloadConfig() {
	// Capture old state for diff logging
	oldKeys := make([]string, 0)
	AppConfig.Mu.RLock()
	oldCount := len(AppConfig.Config)
	for key := range AppConfig.Config {
		oldKeys = append(oldKeys, key)
	}
	AppConfig.Mu.RUnlock()

	config := make(map[string]string)
	ctx := context.Background()

	// Read ConfigMaps - sort names for deterministic key ordering
	cmNames := make([]string, 0, len(ci.configMapNames))
	for name := range ci.configMapNames {
		cmNames = append(cmNames, name)
	}
	sort.Strings(cmNames)

	for _, name := range cmNames {
		cm, err := ci.clientset.CoreV1().ConfigMaps(ci.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			log.Warn().Err(err).Str("configmap", name).Msg("Failed to get ConfigMap")
			continue
		}
		for k, v := range cm.Data {
			config[k] = v
		}
	}

	// Read Secrets - sort names for deterministic key ordering
	sNames := make([]string, 0, len(ci.secretNames))
	for name := range ci.secretNames {
		sNames = append(sNames, name)
	}
	sort.Strings(sNames)

	for _, name := range sNames {
		s, err := ci.clientset.CoreV1().Secrets(ci.namespace).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			log.Warn().Err(err).Str("secret", name).Msg("Failed to get Secret")
			continue
		}
		for k, v := range s.Data {
			config[k] = string(v)
		}
	}

	if len(config) == 0 && oldCount > 0 {
		log.Warn().
			Int("old_count", oldCount).
			Msg("All watched ConfigMaps/Secrets returned empty data, preserving existing config")
		return
	}

	// Update AppConfig
	AppConfig.Mu.Lock()
	AppConfig.Config = config
	AppConfig.Mu.Unlock()

	metrics.ConfigReloads.Inc()

	// Log changes
	newKeys := make([]string, 0, len(config))
	addedKeys := make([]string, 0)
	removedKeys := make([]string, 0)
	for key := range config {
		newKeys = append(newKeys, key)
		if !contains(oldKeys, key) {
			addedKeys = append(addedKeys, key)
		}
	}
	for _, key := range oldKeys {
		if !contains(newKeys, key) {
			removedKeys = append(removedKeys, key)
		}
	}

	log.Info().
		Int("old_count", oldCount).
		Int("new_count", len(config)).
		Strs("added_keys", addedKeys).
		Strs("removed_keys", removedKeys).
		Msg("Configuration reloaded successfully")

	// Trigger Kustomization updates
	if ci.autoUpdate && ci.kustomizationUpdater != nil {
		select {
		case ci.updateCh <- struct{}{}:
			go func() {
				defer func() { <-ci.updateCh }()
				if err := ci.kustomizationUpdater.TriggerUpdateAll(); err != nil {
					log.Error().Err(err).Msg("Failed to trigger Kustomization updates")
				}
			}()
		default:
			log.Debug().Msg("Kustomization update already in progress, skipping")
		}
	}
}

func contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

func (ci *ConfigInformer) Start() error {
	log.Info().
		Str("namespace", ci.namespace).
		Int("configmaps", len(ci.configMapNames)).
		Int("secrets", len(ci.secretNames)).
		Msg("Starting config informer")

	ci.factory.Start(ci.done)

	// Wait for informer caches to sync
	synced := ci.factory.WaitForCacheSync(ci.done)
	for typ, ok := range synced {
		if !ok {
			return fmt.Errorf("failed to sync informer cache for %v", typ)
		}
	}

	log.Info().Msg("Informer caches synced, performing initial config load")
	ci.reloadConfig()

	// Block until stopped
	<-ci.done
	return nil
}

func (ci *ConfigInformer) Stop() {
	ci.stopOnce.Do(func() {
		close(ci.done)
	})
}

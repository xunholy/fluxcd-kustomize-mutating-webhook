package webhook

import (
	"crypto/tls"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog/log"
)

type CertWatcher struct {
	certFile string
	keyFile  string
	cert     *tls.Certificate
	mu       sync.RWMutex
	watcher  *fsnotify.Watcher
	done     chan struct{}
}

func NewCertWatcher(certFile, keyFile string) (*CertWatcher, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	cw := &CertWatcher{
		certFile: certFile,
		keyFile:  keyFile,
		watcher:  watcher,
		done:     make(chan struct{}),
	}
	if err := cw.loadCertificate(); err != nil {
		return nil, fmt.Errorf("failed to load initial certificate: %w", err)
	}
	return cw, nil
}

func (cw *CertWatcher) loadCertificate() error {
	cert, err := tls.LoadX509KeyPair(cw.certFile, cw.keyFile)
	if err != nil {
		return fmt.Errorf("failed to load key pair: %w", err)
	}
	cw.mu.Lock()
	cw.cert = &cert
	cw.mu.Unlock()
	return nil
}

func (cw *CertWatcher) GetCertificate(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
	cw.mu.RLock()
	defer cw.mu.RUnlock()
	return cw.cert, nil
}

func (cw *CertWatcher) Watch() error {
	if err := cw.watcher.Add(filepath.Dir(cw.certFile)); err != nil {
		return fmt.Errorf("failed to add directory to watcher: %w", err)
	}

	for {
		select {
		case event, ok := <-cw.watcher.Events:
			if !ok {
				return errors.New("watcher channel closed")
			}
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				log.Info().Msg("Certificate files modified. Reloading...")
				time.Sleep(100 * time.Millisecond)
				if err := cw.loadCertificate(); err != nil {
					log.Error().Err(err).Msg("Failed to reload certificate")
				} else {
					log.Info().Msg("Certificate reloaded successfully")
				}
			}
		case err, ok := <-cw.watcher.Errors:
			if !ok {
				return errors.New("watcher error channel closed")
			}
			log.Error().Err(err).Msg("Error watching certificate files")
		case <-cw.done:
			return nil
		}
	}
}

func (cw *CertWatcher) Stop() {
	close(cw.done)
	cw.watcher.Close()
}

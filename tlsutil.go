package main

import (
	"crypto/tls"
	"log/slog"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// KeypairReloader structs holds cert path and certs
type KeypairReloader struct {
	certMu   sync.RWMutex
	cert     *tls.Certificate
	certPath string
	keyPath  string
}

// NewKeypairReloader will load certs on first run and trigger a goroutine for fsnotify watcher
func NewKeypairReloader(certPath, keyPath string) (*KeypairReloader, error) {
	result := &KeypairReloader{
		certPath: certPath,
		keyPath:  keyPath,
	}
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	result.cert = &cert

	// creates a new file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}

	defer func() {
		if err != nil {
			watcher.Close()
		}
	}()

	if err := watcher.Add("/etc/webhook/certs"); err != nil {
		return nil, err
	}

	go func() {
		for {
			select {
			// watch for events
			case event := <-watcher.Events:
				// fsnotify.create events will tell us if there are new certs
				if event.Op&fsnotify.Create == fsnotify.Create {
					slog.Info("reloading certs")
					if err := result.reload(); err != nil {
						slog.Error("could not load new certs", "error", err)
					}
				}

				// watch for errors
			case err := <-watcher.Errors:
				slog.Error("watcher error", "error", err)
			}
		}
	}()

	return result, nil
}

// reload loads updated cert and key whenever they are updated
func (kpr *KeypairReloader) reload() error {
	newCert, err := tls.LoadX509KeyPair(kpr.certPath, kpr.keyPath)
	if err != nil {
		return err
	}
	kpr.certMu.Lock()
	defer kpr.certMu.Unlock()
	kpr.cert = &newCert
	return nil
}

// GetCertificateFunc will return function which will be used as tls.Config.GetCertificate
func (kpr *KeypairReloader) GetCertificateFunc() func(*tls.ClientHelloInfo) (*tls.Certificate, error) {
	return func(clientHello *tls.ClientHelloInfo) (*tls.Certificate, error) {
		kpr.certMu.RLock()
		defer kpr.certMu.RUnlock()
		return kpr.cert, nil
	}
}

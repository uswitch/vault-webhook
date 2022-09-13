package main

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"os"
	"time"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	log "github.com/sirupsen/logrus"
	webhook "github.com/uswitch/vault-webhook/pkg/client/clientset/versioned"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/sample-controller/pkg/signals"
)

var (
	vaultAddr        string
	vaultCaPath      string
	gatewayAddr      string
	loginPath        string
	secretPathFormat string
	sidecarImage     string
)

func main() {

	kingpin.Flag("vault-address", "URL of vault").Required().StringVar(&vaultAddr)
	kingpin.Flag("vault-ca-path", "Path to the CA cert for vault").StringVar(&vaultCaPath)
	kingpin.Flag("login-path", "Kubernetes auth login path for vault").Required().StringVar(&loginPath)
	kingpin.Flag("sidecar-image", "Vault-creds sidecar image to use").Required().StringVar(&sidecarImage)
	kingpin.Flag("gateway-address", "URL of Push Gateway").StringVar(&gatewayAddr)
	kingpin.Flag("secret-path-format", "The format for the path used for reading database credentials, where the first %s is the database name and the second %s is the role").Default("%s/creds/%s").StringVar(&secretPathFormat)
	kingpin.Parse()
	log.SetOutput(os.Stderr)

	ctx := context.Background()

	// load certs
	kpr, err := NewKeypairReloader("/etc/webhook/certs/cert.pem", "/etc/webhook/certs/key.pem")
	if err != nil {
		log.Errorf("Failed to load key pair: %v", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		log.Fatalf("error creating kube client config: %s", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatalf("error creating kube client: %s", err)
	}

	webhookClient, err := webhook.NewForConfig(config)
	if err != nil {
		log.Fatalf("error creating webhook client: %s", err)
	}

	watcher := NewListWatch(webhookClient)

	srv := http.Server{Addr: fmt.Sprintf(":443")}

	// this will check if there are new certs before every tls handshake
	t := &tls.Config{GetCertificate: kpr.GetCertificateFunc()}
	srv.TLSConfig = t

	whsvr := webHookServer{
		server:   &srv,
		client:   client,
		bindings: watcher,
		ctx:      ctx,
	}

	stopCh := signals.SetupSignalHandler()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	enabledScrapers := []scrape.Scraper{scrapeImpl.MyScraperOne{}, scrape.HostScrape{}, scrape.CpuScrape{}, scrape.GpuScrape{}, scrape.DiskScrape{}, scrape.MemoryScrape{}}
	handlerFunc := newHandler(collector.NewMetrics(), enabledScrapers)
	promhandler := promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, handlerFunc)

	whsvr.server.Handler = promhandler

	healthMux := http.NewServeMux()
	healthMux.Handle("/metrics", promhttp.Handler())
	healthMux.HandleFunc("/healthz", whsvr.checkHealth)

	healthServer := &http.Server{
		Addr:    fmt.Sprintf(":8080"),
		Handler: healthMux,
	}

	watcher.Run(ctx)

	log.Info("Waiting for informer caches to sync")
	if ok := watcher.controller.HasSynced(); !ok {
		log.Fatal("failed to wait for caches to sync")
	}

	log.Info("starting server")

	// start webhook server in new rountine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to listen and serve webhook server: %v", err)
		}
	}()

	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Failed to listen and serve health server: %v", err)
		}
	}()

	// listening OS shutdown singal
	<-stopCh

	log.Infof("Got OS shutdown signal, shutting down webhook server gracefully...")
	shutDownCTX, shutDownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutDownCancel()
	whsvr.server.Shutdown(shutDownCTX)
	healthServer.Shutdown(shutDownCTX)
}

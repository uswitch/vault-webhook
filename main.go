package main

import (
	"context"
	"crypto/tls"
	"net/http"
	"os"
	"time"

	"flag"
	"log/slog"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	webhook "github.com/uswitch/vault-webhook/pkg/client/clientset/versioned"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
)

var (
	vaultAddr        string
	vaultCaPath      string
	gatewayAddr      string
	loginPath        string
	secretPathFormat string
	sidecarImage     string
	serverAddress    string
)

func main() {

	flag.StringVar(&vaultAddr, "vault-address", "", "URL of vault (required)")
	flag.StringVar(&vaultCaPath, "vault-ca-path", "", "Path to the CA cert for vault")
	flag.StringVar(&loginPath, "login-path", "", "Kubernetes auth login path for vault (required)")
	flag.StringVar(&sidecarImage, "sidecar-image", "", "Vault-creds sidecar image to use (required)")
	flag.StringVar(&gatewayAddr, "gateway-address", "", "URL of Push Gateway")
	flag.StringVar(&secretPathFormat, "secret-path-format", "%s/creds/%s", "The format for the path used for reading database credentials, where the first %s is the database name and the second %s is the role")
	flag.StringVar(&serverAddress, "server-address", ":8443", "The address the webhook server will listen on.")
	flag.Parse()

	for _, required := range []struct{ name, val string }{
		{"vault-address", vaultAddr},
		{"login-path", loginPath},
		{"sidecar-image", sidecarImage},
	} {
		if required.val == "" {
			slog.Error("flag is required", "flag", required.name)
			os.Exit(1)
		}
	}

	ctx := context.Background()

	// load certs
	kpr, err := NewKeypairReloader("/etc/webhook/certs/cert.pem", "/etc/webhook/certs/key.pem")
	if err != nil {
		slog.Error("failed to load key pair", "error", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		slog.Error("error creating kube client config", "error", err)
		os.Exit(1)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		slog.Error("error creating kube client", "error", err)
		os.Exit(1)
	}

	webhookClient, err := webhook.NewForConfig(config)
	if err != nil {
		slog.Error("error creating webhook client", "error", err)
		os.Exit(1)
	}

	watcher := NewListWatch(webhookClient)

	srv := http.Server{Addr: serverAddress}

	// this will check if there are new certs before every tls handshake
	t := &tls.Config{GetCertificate: kpr.GetCertificateFunc()}
	srv.TLSConfig = t

	whsvr := webHookServer{
		server:   &srv,
		client:   client,
		bindings: watcher,
		ctx:      ctx,
	}

	cont := ctrl.SetupSignalHandler()
	ctx, cancel := context.WithCancel(cont)
	defer cancel()

	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	promhandler := promhttp.InstrumentMetricHandler(prometheus.DefaultRegisterer, mux)

	whsvr.server.Handler = promhandler

	healthMux := http.NewServeMux()
	healthMux.Handle("/metrics", promhttp.Handler())
	healthMux.HandleFunc("/healthz", whsvr.checkHealth)

	healthServer := &http.Server{
		Addr:    ":8080",
		Handler: healthMux,
	}

	watcher.Run(ctx)

	slog.Info("waiting for informer caches to sync")
	if ok := watcher.controller.HasSynced(); !ok {
		slog.Error("failed to wait for caches to sync")
		os.Exit(1)
	}

	slog.Info("starting server")

	// start webhook server in new rountine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to listen and serve webhook server", "error", err)
			os.Exit(1)
		}
	}()

	go func() {
		if err := healthServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("failed to listen and serve health server", "error", err)
			os.Exit(1)
		}
	}()

	// listening OS shutdown singal
	<-cont.Done()

	slog.Info("got OS shutdown signal, shutting down webhook server gracefully")
	shutDownCTX, shutDownCancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer shutDownCancel()
	whsvr.server.Shutdown(shutDownCTX)
	healthServer.Shutdown(shutDownCTX)
}

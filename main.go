package main

import (
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/golang/glog"
	kingpin "gopkg.in/alecthomas/kingpin.v2"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var cluster string

func main() {

	kingpin.Flag("cluster", "Name of cluster").Required().StringVar(&cluster)
	kingpin.Parse()

	pair, err := tls.LoadX509KeyPair("/etc/webhook/certs/cert.pem", "/etc/webhook/certs/key.pem")
	if err != nil {
		glog.Errorf("Filed to load key pair: %v", err)
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		glog.Fatalf("error creating kube client config: %s", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		glog.Fatalf("error creating kube client: %s", err)
	}

	whsvr := webHookServer{
		server: &http.Server{
			Addr:      fmt.Sprintf(":443"),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
		client: client,
	}

	flag.Set("logtostderr", "true")

	// define http server and server handler
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	whsvr.server.Handler = mux

	// start webhook server in new rountine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			glog.Errorf("Filed to listen and serve webhook server: %v", err)
		}
	}()

	// listening OS shutdown singal
	signalChan := make(chan os.Signal, 1)
	signal.Notify(signalChan, syscall.SIGINT, syscall.SIGTERM)
	<-signalChan

	glog.Infof("Got OS shutdown signal, shutting down webook server gracefully...")
	whsvr.server.Shutdown(context.Background())
}

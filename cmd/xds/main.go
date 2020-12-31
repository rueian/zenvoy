package main

import (
	"context"
	"fmt"
	"github.com/rueian/zenvoy/pkg/alloc"
	"github.com/rueian/zenvoy/pkg/config"
	"github.com/rueian/zenvoy/pkg/kube"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
	"k8s.io/client-go/kubernetes"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
)

var l = &logger.Std{}

func main() {
	conf, err := config.GetXDS()
	if err != nil {
		l.Fatalf("config error %+v", err)
	}

	server := xds.NewServer(l, conf.XDSNodeID, xds.Debug(l.Debug))

	manager, err := kube.NewManager()
	if err != nil {
		l.Fatalf("manager error %+v", err)
	}

	clientset, err := kubernetes.NewForConfig(manager.GetConfig())
	if err != nil {
		l.Fatalf("clientset error %+v", err)
	}

	idAlloc := alloc.NewID(conf.ProxyPortMin, conf.ProxyPortMax)
	if err = kube.SetupEndpointController(manager, conf.ProxyAddr, server.Snapshot, idAlloc); err != nil {
		l.Fatalf("controller error %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := manager.Start(ctx); err != nil {
			l.Fatalf("controller start error %+v", err)
		}
	}()

	scaler := kube.NewScaler(clientset)
	go func() {
		http.ListenAndServe(fmt.Sprintf(":%d", conf.TriggerPort), http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			if deploy := request.URL.Query().Get("deployment"); deploy != "" {
				if err := scaler.Scale(context.Background(), "default", deploy); err != nil {
					l.Errorf("fail to scale deployment %s %+v", deploy, err)
				}
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}()

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", conf.XDSPort))
	if err != nil {
		l.Fatalf("listen error %+v", err)
	}
	defer lis.Close()
	l.Infof("xds listen on %s", lis.Addr().String())

	go func() {
		if err := server.Serve(context.Background(), lis); err != nil {
			l.Fatalf("listen error %+v", err)
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	cancel()
	server.GracefulStop()
}

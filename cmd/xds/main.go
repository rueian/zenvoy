package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	"k8s.io/client-go/kubernetes"

	"github.com/rueian/zenvoy/pkg/config"
	"github.com/rueian/zenvoy/pkg/kube"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
)

var l = &logger.Std{}

func main() {
	conf, err := config.GetXDS()
	if err != nil {
		l.Fatalf("config error %+v", err)
	}

	server := xds.NewServer(l, conf.XDSNodeID, xds.Debug(l.Debug))

	manager, err := kube.NewManager(conf.KubeNamespace)
	if err != nil {
		l.Fatalf("manager error %+v", err)
	}

	clientset, err := kubernetes.NewForConfig(manager.GetConfig())
	if err != nil {
		l.Fatalf("clientset error %+v", err)
	}

	if err = kube.SetupEndpointController(manager, server.Snapshot, conf.ProxyAddr, conf.ProxyPortMin, conf.ProxyPortMax); err != nil {
		l.Fatalf("controller error %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := manager.Start(ctx); err != nil {
			l.Fatalf("controller start error %+v", err)
		}
	}()

	scaler := kube.NewScaler(l, clientset, conf.KubeNamespace)
	monitor := xds.NewMonitorServer(scaler, xds.MonitorOptions{
		ScaleToZeroAfter: conf.ScaleToZeroAfter,
		ScaleToZeroCheck: conf.ScaleToZeroCheck,
	})

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", conf.XDSPort))
	if err != nil {
		l.Fatalf("listen error %+v", err)
	}
	defer lis.Close()
	l.Infof("xds listen on %s", lis.Addr().String())

	go func() {
		if err := server.Serve(context.Background(), lis, monitor); err != nil {
			l.Fatalf("listen error %+v", err)
		}
	}()

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
	<-sigs
	cancel()
	server.GracefulStop()
}

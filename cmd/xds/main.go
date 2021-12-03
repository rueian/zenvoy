package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"syscall"

	envconfig "github.com/rueian/zenvoy/pkg/config"
	"github.com/rueian/zenvoy/pkg/kube"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
	"github.com/rueian/zenvoy/pkg/xds/controllers"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

var (
	l      = &logger.Std{}
	scheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
}

func main() {
	conf, err := envconfig.GetXDS()
	if err != nil {
		l.Fatalf("envconfig error %+v", err)
	}

	server := xds.NewServer(l, conf.XDSNodeID, conf, xds.Debug(l.Debug))

	clientConf, err := config.GetConfig()
	if err != nil {
		l.Fatalf("k8s client config error %+v", err)
	}
	mgr, err := manager.New(clientConf, manager.Options{Scheme: scheme, Namespace: conf.KubeNamespace})
	if err != nil {
		l.Fatalf("manager error %+v", err)
	}

	clientset, err := kubernetes.NewForConfig(mgr.GetConfig())
	if err != nil {
		l.Fatalf("clientset error %+v", err)
	}

	scaler := kube.NewScaler(l, clientset, conf.KubeNamespace)
	monitor := xds.NewMonitorServer(scaler, xds.MonitorOptions{
		ScaleToZeroAfter: conf.ScaleToZeroAfter,
		ScaleToZeroCheck: conf.ScaleToZeroCheck,
	})

	if err = controllers.SetupEndpointController(
		mgr,
		monitor,
		server.Snapshot,
		conf.ProxyAddr,
		conf.ProxyPortMin,
		conf.ProxyPortMax,
		xds.MakeVirtualHostFn(conf),
	); err != nil {
		l.Fatalf("controller error %+v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		if err := mgr.Start(ctx); err != nil {
			l.Fatalf("controller start error %+v", err)
		}
	}()

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

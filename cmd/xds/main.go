package main

import (
	"context"
	"fmt"
	"github.com/rueian/zenvoy/pkg/alloc"
	"github.com/rueian/zenvoy/pkg/config"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
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

	idAlloc := alloc.NewID(conf.ProxyPortMin, conf.ProxyPortMax)
	echoProxyPort, _ := idAlloc.Acquire()

	go func() {
		http.ListenAndServe(fmt.Sprintf(":%d", conf.TriggerPort), http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			err := server.SetClusterEndpoints("echo", 8080, "echo")
			if err != nil {
				l.Errorf("fail to update xds %+v", err)
			}
			w.WriteHeader(http.StatusNoContent)
		}))
	}()

	err = server.SetCluster("echo")
	err = server.SetClusterEndpoints("echo", echoProxyPort, "proxy")
	err = server.SetClusterRoute("echo", "*", "/")
	if err != nil {
		l.Fatalf("fail to update xds %+v", err)
	}

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
	server.GracefulStop()
}

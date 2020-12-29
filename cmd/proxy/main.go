// +build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/proxy"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

var (
	l          *logger.Std
	port       uint
	xdsAddr    string
	nodeID     string
	triggerURL string
)

func init() {
	l = &logger.Std{}

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	flag.UintVar(&port, "port", 20000, "proxy server port")

	flag.StringVar(&nodeID, "nodeID", "zenvoy", "Node ID")

	flag.StringVar(&xdsAddr, "xds", "xds:18000", "xds server addr")

	flag.StringVar(&triggerURL, "triggerURL", "http://xds:17999", "trigger to scale")
}

func main() {
	lc := net.ListenConfig{Control: SetSocketOptions}
	lis, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		l.Fatalf("listen error %+v", err)
	}

	conn, err := grpc.Dial(xdsAddr, grpc.WithInsecure())
	if err != nil {
		l.Fatalf("grpc dial error %+v", err)
	}

	ip := GetNonLoopbackIP()
	l.Infof("proxy ip identifier: %s", ip)

	isProxy := func(addr string) bool {
		return strings.HasPrefix(addr, ip)
	}

	sg := singleflight.Group{}

	xdsClient := proxy.NewXDSClient(l, conn, nodeID, isProxy)
	server := proxy.NewServer(l, xdsClient, isProxy, func(cluster string) {
		sg.Do(cluster, func() (interface{}, error) {
			resp, err := http.Get(triggerURL)
			if err != nil {
				l.Errorf("trigger error %+v", err)
			}
			if resp != nil {
				resp.Body.Close()
			}
			return nil, nil
		})
	})
	go func() {
		for {
			if err := xdsClient.Listen(context.Background()); err != nil {
				l.Errorf("xdsClient listen err %+v", err)
			}
			time.Sleep(time.Second)
		}
	}()
	server.Serve(lis)
}

func SetSocketOptions(network string, address string, c syscall.RawConn) error {
	return c.Control(func(s uintptr) {
		err := syscall.SetsockoptInt(int(s), syscall.SOL_IP, syscall.IP_TRANSPARENT, 1)
		if err != nil {
			l.Fatalf("fail to set IP_TRANSPARENT: %v", err)
		}
	})
}

func GetNonLoopbackIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return ""
	}
	for _, address := range addrs {
		if ipnet, ok := address.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return ""
}

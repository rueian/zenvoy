// +build linux

package main

import (
	"context"
	"fmt"
	"github.com/rueian/zenvoy/pkg/config"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/proxy"
	"github.com/rueian/zenvoy/pkg/tproxy"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"net"
	"net/http"
	"strings"
	"syscall"
	"time"
)

var l = &logger.Std{}

func main() {
	conf, err := config.GetProxy()
	if err != nil {
		l.Fatalf("config error %+v", err)
	}

	ip := conf.ProxyAddr
	if ip == "" {
		ip = GetNonLoopbackIP()
	}
	l.Infof("proxy ip identifier: %s", ip)

	out, err := tproxy.Setup(conf.TPROXYCMD, ip, conf.ProxyPort, conf.ProxyPortMin, conf.ProxyPortMax)
	if err != nil {
		l.Fatalf("%s tproxy error %+v: %s", conf.TPROXYCMD, err, out)
	}
	l.Infof("set tproxy for %s:%d-%d to :%d", ip, conf.ProxyPortMin, conf.ProxyPortMax, conf.ProxyPort)

	lc := net.ListenConfig{Control: SetSocketOptions}
	lis, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", conf.ProxyPort))
	if err != nil {
		l.Fatalf("listen error %+v", err)
	}
	defer lis.Close()
	l.Infof("proxy listen on %s", lis.Addr().String())

	conn, err := grpc.Dial(conf.XDSAddr, grpc.WithInsecure())
	if err != nil {
		l.Fatalf("grpc dial error %+v", err)
	}
	defer conn.Close()

	isProxy := func(addr string) bool {
		return strings.HasPrefix(addr, ip)
	}

	sg := singleflight.Group{}

	xdsClient := proxy.NewXDSClient(l, conn, conf.XDSNodeID, isProxy)
	server := proxy.NewServer(l, xdsClient, isProxy, func(cluster string) {
		sg.Do(cluster, func() (interface{}, error) {
			resp, err := http.Get(conf.TriggerURL + "?deployment=" + cluster)
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

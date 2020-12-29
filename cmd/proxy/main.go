// +build linux

package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/proxy"
	"net"
	"syscall"
)

var (
	l    *logger.Std
	port uint
)

func init() {
	l = &logger.Std{}

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	flag.UintVar(&port, "port", 20000, "proxy server port")
}

func main() {
	lc := net.ListenConfig{Control: SetSocketOptions}
	lis, err := lc.Listen(context.Background(), "tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		l.Fatalf("listen error %+v", err)
	}

	store := proxy.NewStore()
	server := proxy.NewServer(l, store)
	store.SetIntendedEndpoints(20000, []string{"echo:8080"})
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

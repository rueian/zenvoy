// Copyright 2020 Envoyproxy Authors
//
//   Licensed under the Apache License, Version 2.0 (the "License");
//   you may not use this file except in compliance with the License.
//   You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
//   Unless required by applicable law or agreed to in writing, software
//   distributed under the License is distributed on an "AS IS" BASIS,
//   WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
//   See the License for the specific language governing permissions and
//   limitations under the License.
package main

import (
	"context"
	"flag"
	"fmt"
	"github.com/rueian/zenvoy/pkg/alloc"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"
)

var (
	l *logger.Std

	port   uint
	nodeID string

	proxyMinPort uint
	proxyMaxPort uint
)

func init() {
	l = &logger.Std{}

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	// The port that this xDS server listens on
	flag.UintVar(&port, "port", 18000, "xDS management server port")

	// Tell Envoy to use this Node ID
	flag.StringVar(&nodeID, "nodeID", "zenvoy", "Node ID")

	flag.UintVar(&proxyMinPort, "proxyMinPort", 20000, "min proxy port for envoy cluster")
	flag.UintVar(&proxyMaxPort, "proxyMaxPort", 32767, "max proxy port for envoy cluster")
}

func main() {
	flag.Parse()

	server := xds.NewServer(l, nodeID, xds.Debug(l.Debug))

	idAlloc := alloc.NewID(uint32(proxyMinPort), uint32(proxyMaxPort))
	echoProxyPort, _ := idAlloc.Acquire()

	go func() {
		time.Sleep(10 * time.Second)
		err := server.SetClusterEndpoints("echo", 8080, "echo")
		if err != nil {
			l.Errorf("fail to update xds %+v", err)
		}
	}()

	var err error
	err = server.SetCluster("echo")
	err = server.SetClusterEndpoints("echo", echoProxyPort, "proxy")
	err = server.SetClusterRoute("echo", "*", "/")
	if err != nil {
		l.Fatalf("fail to update xds %+v", err)
	}

	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		l.Fatalf("listen error %+v", err)
	}

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

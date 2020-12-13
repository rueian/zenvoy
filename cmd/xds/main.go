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
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	testv3 "github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
	"google.golang.org/grpc"
	"net"
	"os"
	"os/signal"
	"syscall"
)

const grpcMaxConcurrentStreams = 1000000

var (
	l *logger.Std

	port   uint
	nodeID string
)

func init() {
	l = &logger.Std{}

	flag.BoolVar(&l.Debug, "debug", false, "Enable xDS server debug logging")

	// The port that this xDS server listens on
	flag.UintVar(&port, "port", 18000, "xDS management server port")

	// Tell Envoy to use this Node ID
	flag.StringVar(&nodeID, "nodeID", "zenvoy", "Node ID")
}

func main() {
	flag.Parse()

	cache := cachev3.NewSnapshotCache(false, cachev3.IDHash{}, l)
	generator := xds.PassiveSnapshotGenerator{}

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := generator.Start(ctx)
	if err != nil {
		l.Fatalf("fail to start snapshot generator: %+v", err)
	}
	go func() {
		xds.Simulation(&generator)
	}()

	started := false
	l.Infof("start receiving snapshots")
	for snapshot := range ch {
		if err := snapshot.Consistent(); err != nil {
			l.Errorf("skip inconsistent snapshot: %+v\n%+v", snapshot, err)
			continue
		}

		if err := cache.SetSnapshot(nodeID, snapshot); err != nil {
			l.Errorf("snapshot error %q for %+v", err, snapshot)
			continue
		}

		l.Debugf("will serve snapshot %+v", snapshot)

		if started {
			continue
		}

		cb := &testv3.Callbacks{Debug: l.Debug}
		server := serverv3.NewServer(ctx, cache, cb)

		var grpcOptions []grpc.ServerOption
		grpcOptions = append(grpcOptions, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
		grpcServer := grpc.NewServer(grpcOptions...)

		lis, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			l.Fatalf("listen error %+v", err)
		}

		// register services
		discoverygrpc.RegisterAggregatedDiscoveryServiceServer(grpcServer, server)
		endpointservice.RegisterEndpointDiscoveryServiceServer(grpcServer, server)
		clusterservice.RegisterClusterDiscoveryServiceServer(grpcServer, server)
		routeservice.RegisterRouteDiscoveryServiceServer(grpcServer, server)
		listenerservice.RegisterListenerDiscoveryServiceServer(grpcServer, server)
		secretservice.RegisterSecretDiscoveryServiceServer(grpcServer, server)
		runtimeservice.RegisterRuntimeDiscoveryServiceServer(grpcServer, server)

		go func() {
			l.Infof("management server listening on %d", port)
			if err = grpcServer.Serve(lis); err != nil {
				l.Fatalf("management server start error %+v", err)
			}
		}()

		go func() {
			sigs := make(chan os.Signal, 1)
			signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
			<-sigs
			grpcServer.GracefulStop()
			cancel()
		}()

		started = true
	}
}

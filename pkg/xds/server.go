package xds

import (
	"context"
	clusterservice "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservice "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservice "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservice "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	runtimeservice "github.com/envoyproxy/go-control-plane/envoy/service/runtime/v3"
	secretservice "github.com/envoyproxy/go-control-plane/envoy/service/secret/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	testv3 "github.com/envoyproxy/go-control-plane/pkg/test/v3"
	"google.golang.org/grpc"
	"net"
)

const grpcMaxConcurrentStreams = 1000000

func NewServer(logger log.Logger, nodeID string, options ...func(s *Server)) *Server {
	s := &Server{
		logger:   logger,
		Snapshot: NewSnapshot(logger, nodeID),
	}
	for _, o := range options {
		o(s)
	}
	return s
}

func Debug(debug bool) func(s *Server) {
	return func(s *Server) { s.debug = debug }
}

type Server struct {
	*Snapshot
	debug  bool
	logger log.Logger
	server *grpc.Server
}

func (s *Server) Serve(ctx context.Context, lis net.Listener, options ...grpc.ServerOption) error {
	options = append(options, grpc.MaxConcurrentStreams(grpcMaxConcurrentStreams))
	server := grpc.NewServer(options...)
	cb := &testv3.Callbacks{Debug: s.debug}
	svc := serverv3.NewServer(ctx, s.Snapshot.Cache(), cb)
	discoverygrpc.RegisterAggregatedDiscoveryServiceServer(server, svc)
	endpointservice.RegisterEndpointDiscoveryServiceServer(server, svc)
	clusterservice.RegisterClusterDiscoveryServiceServer(server, svc)
	routeservice.RegisterRouteDiscoveryServiceServer(server, svc)
	listenerservice.RegisterListenerDiscoveryServiceServer(server, svc)
	secretservice.RegisterSecretDiscoveryServiceServer(server, svc)
	runtimeservice.RegisterRuntimeDiscoveryServiceServer(server, svc)
	s.server = server
	return server.Serve(lis)
}

func (s *Server) GracefulStop() {
	s.server.GracefulStop()
}

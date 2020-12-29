package proxy

import (
	"context"
	"fmt"
	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"google.golang.org/grpc"
	"io"
	"sync"
)

type XDS interface {
	GetIntendedEndpoints(port uint32) ClusterEndpoints
	OnUpdated(func())
}

func NewStore() *store {
	return &store{endpoints: make(map[uint32]ClusterEndpoints)}
}

type ClusterEndpoints struct {
	Cluster   string
	Endpoints []string
}

type store struct {
	endpoints map[uint32]ClusterEndpoints
	mu        sync.RWMutex
	cb        []func()
}

func (x *store) SetIntendedEndpoints(port uint32, clusterEndpoints ClusterEndpoints) {
	x.mu.Lock()
	x.endpoints[port] = clusterEndpoints
	x.mu.Unlock()
	for _, f := range x.cb {
		f()
	}
}

func (x *store) GetIntendedEndpoints(port uint32) ClusterEndpoints {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.endpoints[port]
}

func (x *store) OnUpdated(f func()) {
	x.cb = append(x.cb, f)
}

func NewXDSClient(logger log.Logger, conn *grpc.ClientConn, nodeID string, isProxy func(addr *corev3.SocketAddress) bool) *Client {
	return &Client{
		store:    NewStore(),
		conn:     conn,
		nodeID:   nodeID,
		logger:   logger,
		clusters: make(map[string]uint32),
		isProxy:  isProxy,
	}
}

type Client struct {
	*store

	conn   *grpc.ClientConn
	nodeID string
	logger log.Logger

	clusters map[string]uint32

	isProxy func(addr *corev3.SocketAddress) bool
}

func (c *Client) Listen(ctx context.Context) error {
	client := discoverygrpc.NewAggregatedDiscoveryServiceClient(c.conn)
	stream, err := client.StreamAggregatedResources(ctx)
	if err != nil {
		return err
	}

	requests := make(chan *discoverygrpc.DiscoveryRequest)
	defer close(requests)

	go func() {
		for req := range requests {
			stream.Send(req)
		}
		stream.CloseSend()
	}()

	requests <- &discoverygrpc.DiscoveryRequest{
		Node:    &corev3.Node{Id: c.nodeID},
		TypeUrl: typeCDS,
	}

	for {
		in, err := stream.Recv()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}

		reqs := []*discoverygrpc.DiscoveryRequest{
			{
				VersionInfo:   in.VersionInfo,
				TypeUrl:       in.TypeUrl,
				ResponseNonce: in.Nonce,
			},
		}

		switch in.TypeUrl {
		case typeCDS:
			var edsClusters []string
			for _, res := range in.Resources {
				cluster := &clusterv3.Cluster{}
				if err := res.UnmarshalTo(cluster); err != nil {
					c.logger.Errorf("fail to unmarshal %s with %s", typeCDS, res.String())
					continue
				}
				if cluster.EdsClusterConfig != nil {
					edsClusters = append(edsClusters, cluster.Name)
				}
			}
			if len(edsClusters) > 0 {
				reqs = append(reqs, &discoverygrpc.DiscoveryRequest{
					ResourceNames: edsClusters,
					TypeUrl:       typeEDS,
				})
			}
		case typeEDS:
			for _, res := range in.Resources {
				cla := &endpointv3.ClusterLoadAssignment{}
				if err := res.UnmarshalTo(cla); err != nil {
					c.logger.Errorf("fail to unmarshal %s with %s", typeEDS, res.String())
					continue
				}
				var intended []string
				for _, endpoints := range cla.Endpoints {
					for _, e := range endpoints.LbEndpoints {
						if endpoint := e.GetEndpoint(); endpoint == nil {
							c.logger.Errorf("fail to GetEndpoint() %s", typeEDS, e.String())
							continue
						} else {
							addr := endpoint.Address.GetSocketAddress()
							if c.isProxy(addr) {
								if port, ok := c.clusters[cla.ClusterName]; ok && port != addr.GetPortValue() {
									c.store.SetIntendedEndpoints(port, ClusterEndpoints{})
								}
								c.clusters[cla.ClusterName] = addr.GetPortValue()
							}
							intended = append(intended, fmt.Sprintf("%s:%d", addr.Address, addr.GetPortValue()))
						}
					}
				}
				if port, ok := c.clusters[cla.ClusterName]; ok {
					c.store.SetIntendedEndpoints(port, ClusterEndpoints{
						Cluster:   cla.ClusterName,
						Endpoints: intended,
					})
				}
			}
		}

		for _, req := range reqs {
			requests <- req
		}
	}
}

const (
	typeCDS = "type.googleapis.com/envoy.config.cluster.v3.Cluster"
	typeEDS = "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment"
)

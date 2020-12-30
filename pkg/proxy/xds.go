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
	"sync"
)

type XDSClient interface {
	GetCluster(port uint32) Cluster
	OnUpdated(func(port uint32))
}

func NewStore() *store {
	return &store{clusters: make(map[uint32]Cluster)}
}

type Cluster struct {
	Name      string
	Endpoints []string
}

type store struct {
	clusters map[uint32]Cluster
	mu       sync.RWMutex
	cb       []func(uint32)
}

func (x *store) SetCluster(port uint32, cluster Cluster) {
	x.mu.Lock()
	if orig := x.clusters[port]; orig.Name == cluster.Name && !changed(orig.Endpoints, cluster.Endpoints) {
		x.mu.Unlock()
		return
	}
	x.clusters[port] = cluster
	x.mu.Unlock()
	for _, f := range x.cb {
		f(port)
	}
}

func (x *store) GetCluster(port uint32) Cluster {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.clusters[port]
}

func (x *store) OnUpdated(f func(port uint32)) {
	x.cb = append(x.cb, f)
}

func NewXDSClient(logger log.Logger, conn *grpc.ClientConn, nodeID string, isProxy isProxyFn) *Client {
	return &Client{
		store:    NewStore(),
		grpcConn: conn,
		nodeID:   nodeID,
		logger:   logger,
		clusters: make(map[string]uint32),
		isProxy:  isProxy,
	}
}

type isProxyFn func(string) bool

type Client struct {
	*store

	logger   log.Logger
	nodeID   string
	isProxy  isProxyFn
	clusters map[string]uint32
	grpcConn *grpc.ClientConn
}

func (c *Client) Listen(ctx context.Context) error {
	client := discoverygrpc.NewAggregatedDiscoveryServiceClient(c.grpcConn)
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

	var previous []string

	for {
		in, err := stream.Recv()
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
			var current []string
			for _, res := range in.Resources {
				cluster := &clusterv3.Cluster{}
				if err := res.UnmarshalTo(cluster); err != nil {
					c.logger.Errorf("fail to unmarshal %s with %s", typeCDS, res.String())
					continue
				}
				if cluster.EdsClusterConfig != nil {
					current = append(current, cluster.Name)
				}
			}
			if changed(previous, current) {
				reqs = append(reqs, &discoverygrpc.DiscoveryRequest{
					ResourceNames: current,
					TypeUrl:       typeEDS,
				})
			}
			previous = current
			c.logger.Infof("receive xds clusters: ver=%s %v", in.VersionInfo, current)
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
						} else if addr := endpoint.Address.GetSocketAddress(); addr != nil {
							addrStr := fmt.Sprintf("%s:%d", addr.Address, addr.GetPortValue())
							if c.isProxy(addrStr) {
								if port, ok := c.clusters[cla.ClusterName]; ok && port != addr.GetPortValue() {
									c.store.SetCluster(port, Cluster{})
								}
								c.clusters[cla.ClusterName] = addr.GetPortValue()
							}
							intended = append(intended, addrStr)
						}
					}
				}
				if port, ok := c.clusters[cla.ClusterName]; ok {
					c.store.SetCluster(port, Cluster{
						Name:      cla.ClusterName,
						Endpoints: intended,
					})
				}
				c.logger.Infof("receive xds endpoints: ver=%s %s %v", in.VersionInfo, cla.ClusterName, intended)
			}
		}

		for _, req := range reqs {
			requests <- req
		}
	}
}

func changed(prev, now []string) bool {
	if len(prev) != len(now) {
		return true
	}
	m := make(map[string]bool, len(prev))
	for _, p := range prev {
		m[p] = true
	}
	for _, n := range now {
		if _, ok := m[n]; !ok {
			return true
		}
	}
	return false
}

const (
	typeCDS = "type.googleapis.com/envoy.config.cluster.v3.Cluster"
	typeEDS = "type.googleapis.com/envoy.config.endpoint.v3.ClusterLoadAssignment"
)

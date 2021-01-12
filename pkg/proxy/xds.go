package proxy

import (
	"context"
	"fmt"
	"sync"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	discoverygrpc "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	metricsservice "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	prom "github.com/prometheus/client_model/go"
	"google.golang.org/grpc"

	"github.com/rueian/zenvoy/pkg/xds"
)

type XDSClient interface {
	GetCluster(port uint32) Cluster
	OnUpdated(func(Cluster, bool))
}

func NewStore() *store {
	return &store{
		clusters: make(map[string]Cluster),
		portMap:  make(map[uint32]string),
	}
}

type Cluster struct {
	Name      string
	Endpoints []string
}

type store struct {
	clusters map[string]Cluster
	portMap  map[uint32]string
	mu       sync.RWMutex
	cb       []func(Cluster, bool)
}

func (x *store) SetCluster(port uint32, cluster Cluster) {
	x.mu.Lock()
	if port != 0 {
		x.portMap[port] = cluster.Name
	}
	orig, ok := x.clusters[cluster.Name]
	x.clusters[cluster.Name] = cluster
	x.mu.Unlock()

	if !ok || (cluster.Name != "" && changed(orig.Endpoints, cluster.Endpoints)) {
		for _, f := range x.cb {
			f(cluster, false)
		}
	}
}

func (x *store) RemoveCluster(name string) {
	x.mu.Lock()
	cluster, ok := x.clusters[name]
	delete(x.clusters, name)
	x.mu.Unlock()

	if ok && cluster.Name != "" {
		for _, f := range x.cb {
			f(cluster, true)
		}
	}
}

func (x *store) GetCluster(port uint32) Cluster {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return x.clusters[x.portMap[port]]
}

func (x *store) OnUpdated(f func(Cluster, bool)) {
	x.cb = append(x.cb, f)
}

func NewXDSClient(logger log.Logger, conn *grpc.ClientConn, nodeID string, isProxy isProxyFn) *Client {
	return &Client{
		store:    NewStore(),
		grpcConn: conn,
		nodeID:   nodeID,
		logger:   logger,
		isProxy:  isProxy,
	}
}

type isProxyFn func(string) bool

type Client struct {
	*store

	logger   log.Logger
	nodeID   string
	isProxy  isProxyFn
	clusters []string
	grpcConn *grpc.ClientConn
}

func (c *Client) Trigger(ctx context.Context, cluster string) error {
	client := metricsservice.NewMetricsServiceClient(c.grpcConn)
	stream, err := client.StreamMetrics(ctx)
	if err != nil {
		return err
	}
	defer stream.CloseAndRecv()

	mn := fmt.Sprintf("cluster.%s.%s", cluster, xds.TriggerMetric)
	ty := prom.MetricType_COUNTER
	ts := time.Now().UnixNano() / 1e6
	va := float64(1)
	return stream.Send(&metricsservice.StreamMetricsMessage{
		Identifier: &metricsservice.StreamMetricsMessage_Identifier{Node: &corev3.Node{Id: c.nodeID}},
		EnvoyMetrics: []*prom.MetricFamily{{
			Name:   &mn,
			Type:   &ty,
			Metric: []*prom.Metric{{TimestampMs: &ts, Counter: &prom.Counter{Value: &va}}},
		}},
	})
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
			if removed, ok := missing(c.clusters, current); ok {
				reqs = append(reqs, &discoverygrpc.DiscoveryRequest{ResourceNames: current, TypeUrl: typeEDS})
				c.clusters = current
				for _, m := range removed {
					c.store.RemoveCluster(m)
				}
			}
			c.logger.Infof("receive xds clusters: ver=%s %v", in.VersionInfo, current)
		case typeEDS:
			for _, res := range in.Resources {
				cla := &endpointv3.ClusterLoadAssignment{}
				if err := res.UnmarshalTo(cla); err != nil {
					c.logger.Errorf("fail to unmarshal %s with %s", typeEDS, res.String())
					continue
				}
				var intended []string
				var proxyPort uint32
				for _, endpoints := range cla.Endpoints {
					for _, e := range endpoints.LbEndpoints {
						if endpoint := e.GetEndpoint(); endpoint == nil {
							c.logger.Errorf("fail to GetEndpoint() %s", typeEDS, e.String())
						} else if addr := endpoint.Address.GetSocketAddress(); addr != nil {
							addrStr := fmt.Sprintf("%s:%d", addr.Address, addr.GetPortValue())
							if c.isProxy(addrStr) {
								proxyPort = addr.GetPortValue()
							} else {
								intended = append(intended, addrStr)
							}
						}
					}
				}
				c.store.SetCluster(proxyPort, Cluster{Name: cla.ClusterName, Endpoints: intended})
				c.logger.Infof("receive xds endpoints: ver=%s %s %v", in.VersionInfo, cla.ClusterName, intended)
			}
		}

		for _, req := range reqs {
			requests <- req
		}
	}
}

func missing(prev, now []string) ([]string, bool) {
	m := make(map[string]bool, len(prev))
	for _, p := range prev {
		m[p] = true
	}
	for _, n := range now {
		delete(m, n)
	}
	remain := make([]string, 0, len(m))
	for n := range m {
		remain = append(remain, n)
	}
	return remain, len(remain) != 0 || len(prev) != len(now)
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

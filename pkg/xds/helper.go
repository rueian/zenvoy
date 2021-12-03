package xds

import (
	"net"
	"sort"
	"time"

	cluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	core "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	hcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/golang/protobuf/ptypes"
	"github.com/rueian/zenvoy/pkg/config"
	"github.com/rueian/zenvoy/pkg/xds/controllers"
	v1 "k8s.io/api/core/v1"
)

const (
	RouteName    = "local_route"
	ListenerName = "listener_0"
	ListenerPort = 10000
)

type Endpoint struct {
	IP   string
	Port uint32
}

func makeCluster(clusterName string, timeout time.Duration) *cluster.Cluster {
	return &cluster.Cluster{
		Name:                 clusterName,
		ConnectTimeout:       ptypes.DurationProto(timeout),
		ClusterDiscoveryType: &cluster.Cluster_Type{Type: cluster.Cluster_EDS},
		EdsClusterConfig: &cluster.Cluster_EdsClusterConfig{
			ServiceName: clusterName,
			EdsConfig:   makeConfigSource(),
		},
		LbPolicy: cluster.Cluster_ROUND_ROBIN,
	}
}

func makeEndpoints(clusterName string, eps ...Endpoint) *endpoint.ClusterLoadAssignment {
	sort.Slice(eps, func(i, j int) bool {
		return eps[i].IP < eps[j].IP
	})
	endpoints := make([]*endpoint.LbEndpoint, 0, len(eps))
	for _, ep := range eps {
		ip := net.ParseIP(ep.IP)
		if ip == nil {
			ips, err := net.LookupIP(ep.IP)
			if err != nil || len(ips) == 0 {
				continue
			}
			ip = ips[0]
		}
		endpoints = append(endpoints, &endpoint.LbEndpoint{
			HostIdentifier: &endpoint.LbEndpoint_Endpoint{
				Endpoint: &endpoint.Endpoint{
					Address: &core.Address{
						Address: &core.Address_SocketAddress{
							SocketAddress: &core.SocketAddress{
								Protocol: core.SocketAddress_TCP,
								Address:  ip.String(),
								PortSpecifier: &core.SocketAddress_PortValue{
									PortValue: ep.Port,
								},
							},
						},
					},
				},
			},
		})
	}

	return &endpoint.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*endpoint.LocalityLbEndpoints{{
			LbEndpoints: endpoints,
		}},
	}
}

func MakeVirtualHostFn(conf config.XDS) controllers.EnvoyVirtualHostFn {
	return func(endpoints *v1.Endpoints) (*route.VirtualHost, error) {
		return &route.VirtualHost{
			Name:    endpoints.Name,
			Domains: []string{endpoints.Name},
			Routes: []*route.Route{{
				Match: &route.RouteMatch{
					PathSpecifier: &route.RouteMatch_Prefix{
						Prefix: "",
					},
				},
				Action: &route.Route_Route{
					Route: &route.RouteAction{
						ClusterSpecifier: &route.RouteAction_Cluster{
							Cluster: endpoints.Name,
						},
						Timeout: ptypes.DurationProto(conf.EnvoyReadTimeout),
					},
				},
			}},
		}, nil
	}
}

func makeHTTPListener(listenerName string, route string) *listener.Listener {
	// HTTP filter configuration
	manager := &hcm.HttpConnectionManager{
		CodecType:  hcm.HttpConnectionManager_AUTO,
		StatPrefix: "http",
		RouteSpecifier: &hcm.HttpConnectionManager_Rds{
			Rds: &hcm.Rds{
				ConfigSource:    makeConfigSource(),
				RouteConfigName: route,
			},
		},
		HttpFilters: []*hcm.HttpFilter{{
			Name: wellknown.Router,
		}},
	}
	pbst, err := ptypes.MarshalAny(manager)
	if err != nil {
		panic(err)
	}

	return &listener.Listener{
		Name: listenerName,
		Address: &core.Address{
			Address: &core.Address_SocketAddress{
				SocketAddress: &core.SocketAddress{
					Protocol: core.SocketAddress_TCP,
					Address:  "0.0.0.0",
					PortSpecifier: &core.SocketAddress_PortValue{
						PortValue: ListenerPort,
					},
				},
			},
		},
		FilterChains: []*listener.FilterChain{{
			Filters: []*listener.Filter{{
				Name: wellknown.HTTPConnectionManager,
				ConfigType: &listener.Filter_TypedConfig{
					TypedConfig: pbst,
				},
			}},
		}},
	}
}

func makeConfigSource() *core.ConfigSource {
	source := &core.ConfigSource{}
	source.ResourceApiVersion = resource.DefaultAPIVersion
	source.ConfigSourceSpecifier = &core.ConfigSource_Ads{
		Ads: &core.AggregatedConfigSource{},
	}
	return source
}

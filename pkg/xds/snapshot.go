package xds

import (
	"context"
	"strconv"

	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"github.com/golang/protobuf/proto"
	"github.com/rueian/zenvoy/pkg/config"
)

func NewSnapshot(logger log.Logger, nodeID string, config config.XDS) *Snapshot {
	return &Snapshot{
		nodeID: nodeID,
		config: config,
		cache:  cache.NewSnapshotCache(true, cache.IDHash{}, logger),
		cds:    map[string]types.Resource{},
		eds:    map[string]types.Resource{},
		vhs:    map[string]*route.VirtualHost{},
	}
}

type Snapshot struct {
	ver    uint64
	cds    map[string]types.Resource
	eds    map[string]types.Resource
	vhs    map[string]*route.VirtualHost
	cache  cache.SnapshotCache
	config config.XDS
	nodeID string
}

func (s *Snapshot) Cache() cache.SnapshotCache {
	return s.cache
}

func (s *Snapshot) SetCluster(name string) error {
	if _, ok := s.cds[name]; ok {
		return nil
	}
	s.cds[name] = makeCluster(name, s.config.EnvoyReadTimeout)
	return s.setSnapshot()
}

func (s *Snapshot) RemoveCluster(name string) error {
	if _, ok := s.cds[name]; !ok {
		return nil
	}
	delete(s.cds, name)
	return s.setSnapshot()
}

func (s *Snapshot) SetClusterRoute(name string, vh *route.VirtualHost) error {
	if original, ok := s.vhs[name]; ok && proto.Equal(original, vh) {
		return nil
	}
	s.vhs[name] = vh
	return s.setSnapshot()
}

func (s *Snapshot) RemoveClusterRoute(name string) error {
	if _, ok := s.vhs[name]; !ok {
		return nil
	}
	delete(s.vhs, name)
	return s.setSnapshot()
}

func (s *Snapshot) SetClusterEndpoints(name string, eps ...Endpoint) error {
	endpoints := makeEndpoints(name, eps...)
	if original, ok := s.eds[name]; ok && proto.Equal(original, endpoints) {
		return nil
	}
	s.eds[name] = endpoints
	return s.setSnapshot()
}

func (s *Snapshot) RemoveClusterEndpoints(name string) error {
	if _, ok := s.eds[name]; !ok {
		return nil
	}
	delete(s.eds, name)
	return s.setSnapshot()
}

func (s *Snapshot) setSnapshot() (err error) {
	ver := s.ver + 1
	snapshot := cache.NewSnapshot(
		strconv.FormatUint(ver, 10),
		resources(s.eds),
		resources(s.cds),
		[]types.Resource{&route.RouteConfiguration{
			Name:         RouteName,
			VirtualHosts: virtualHosts(s.vhs),
		}},
		[]types.Resource{makeHTTPListener(ListenerName, RouteName)},
		[]types.Resource{}, // runtimes
		[]types.Resource{}, // secrets
		nil,
	)
	if err = snapshot.Consistent(); err == nil {
		err = s.cache.SetSnapshot(context.Background(), s.nodeID, snapshot)
	}
	if err == nil {
		s.ver = ver
	}
	return err
}

func resources(m map[string]types.Resource) (out []types.Resource) {
	out = make([]types.Resource, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

func virtualHosts(m map[string]*route.VirtualHost) (out []*route.VirtualHost) {
	out = make([]*route.VirtualHost, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	return out
}

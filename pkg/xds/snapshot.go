package xds

import (
	route "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	"github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"strconv"
)

func NewSnapshot(logger log.Logger, nodeID string) *Snapshot {
	return &Snapshot{
		nodeID: nodeID,
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
	nodeID string
}

func (s *Snapshot) Cache() cache.SnapshotCache {
	return s.cache
}

func (s *Snapshot) SetCluster(name string) error {
	s.cds[name] = makeCluster(name)
	return s.setSnapshot()
}

func (s *Snapshot) RemoveCluster(name string) error {
	delete(s.cds, name)
	return s.setSnapshot()
}

func (s *Snapshot) SetClusterRoute(name, domain, prefix string) error {
	s.vhs[name] = makeVirtualHostRoutes(name, domain, prefix)
	return s.setSnapshot()
}

func (s *Snapshot) RemoveClusterRoute(name string) error {
	delete(s.vhs, name)
	return s.setSnapshot()
}

func (s *Snapshot) SetClusterEndpoints(name string, port uint32, hosts ...string) error {
	s.eds[name] = makeEndpoints(name, port, hosts...)
	return s.setSnapshot()
}

func (s *Snapshot) RemoveClusterEndpoints(name string) error {
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
	)
	if err = snapshot.Consistent(); err == nil {
		err = s.cache.SetSnapshot(s.nodeID, snapshot)
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

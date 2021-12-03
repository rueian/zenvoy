package proxy

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rueian/zenvoy/pkg/config"
	"google.golang.org/grpc"

	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
)

type XDSSuite struct {
	ln      net.Listener
	conn    *grpc.ClientConn
	server  *xds.Server
	monitor *xds.MonitorServer
	client  *Client
	nodeID  string
	cancel  context.CancelFunc
	isProxy isProxyFn
	scaler  *mockScaler

	proxyHost string
}

func (s *XDSSuite) Close() {
	s.cancel()
	s.server.GracefulStop()
	s.ln.Close()
	s.conn.Close()
}

func TestNewXDSClient(t *testing.T) {
	suite := setupXDS(t)
	defer suite.Close()

	updated := make(chan struct{})

	suite.client.OnUpdated(func(cluster Cluster, deleted bool) {
		updated <- struct{}{}
	})

	var err error

	for _, test := range []struct {
		ClusterName  string
		ClusterBind  uint32
		EndpointPort uint32
		Hosts        []string
		Delete       bool
	}{
		{ClusterName: "1", ClusterBind: 1111, EndpointPort: 1111, Hosts: []string{suite.proxyHost}},
		{ClusterName: "1", ClusterBind: 1111, EndpointPort: 2222, Hosts: []string{"1.1.1.1", "2.2.2.2"}},
		{ClusterName: "2", ClusterBind: 2222, EndpointPort: 2222, Hosts: []string{suite.proxyHost}},
		{ClusterName: "2", ClusterBind: 2222, EndpointPort: 3333, Hosts: []string{"3.3.3.3", "4.4.4.4"}},
		{ClusterName: "2", ClusterBind: 2222, Delete: true},
		{ClusterName: "1", ClusterBind: 1111, EndpointPort: 2222, Hosts: []string{"2.2.2.2", "3.3.3.3"}},
		{ClusterName: "1", ClusterBind: 3333, EndpointPort: 3333, Hosts: []string{suite.proxyHost}},
	} {
		if test.Delete {
			err = suite.server.RemoveClusterEndpoints(test.ClusterName)
			err = suite.server.RemoveCluster(test.ClusterName)
			suite.monitor.RemoveCluster(test.ClusterName)
		} else {
			upstreams := endpoints(test.EndpointPort, test.Hosts)
			err = suite.server.SetCluster(test.ClusterName)
			err = suite.server.SetClusterEndpoints(test.ClusterName, upstreams...)
			suite.monitor.TrackCluster(test.ClusterName, len(upstreams))
		}
		if err != nil {
			t.Fatalf("xds error %v", err)
		}

		<-updated

		if test.Delete {
			continue
		}

		cluster := suite.client.GetCluster(test.ClusterBind)
		if cluster.Name != test.ClusterName {
			t.Fatalf("expected cluster name %s, got %s", test.ClusterName, cluster.Name)
		}
		expected := exclude(test.Hosts, suite.isProxy)
		if len(cluster.Endpoints) != len(expected) {
			t.Fatalf("expected endpoints len %d, got %d", len(expected), len(cluster.Endpoints))
		}
		for i, endpoint := range cluster.Endpoints {
			if expect := fmt.Sprintf("%s:%d", expected[i], test.EndpointPort); endpoint != expect {
				t.Fatalf("expected endpint %s, got %v", expect, endpoint)
			}
		}
	}

	if err = suite.client.Trigger(context.Background(), "mycluster"); err != nil {
		t.Fatalf("trigger err %v", err)
	}
	suite.monitor.TrackCluster("mycluster", 1)
	select {
	case <-suite.scaler.Triggered("mycluster"):
	case <-time.After(time.Second):
		t.Fatalf("trigger timeout")
	}

}

func setupXDS(t *testing.T) *XDSSuite {
	l := &logger.Std{}

	nodeID := "test"
	proxyHost := "127.0.0.1"
	isProxy := func(s string) bool {
		return strings.HasPrefix(s, proxyHost)
	}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server := xds.NewServer(l, nodeID, config.XDS{}, xds.Debug(true))
	scaler := &mockScaler{chs: make(map[string]chan struct{})}
	monitor := xds.NewMonitorServer(scaler, xds.MonitorOptions{
		ScaleToZeroAfter: time.Millisecond,
		ScaleToZeroCheck: time.Millisecond,
	})
	go server.Serve(context.Background(), ln, monitor)

	conn, err := grpc.Dial(ln.Addr().String(), grpc.WithInsecure())
	if err != nil {
		t.Fatal(err)
	}

	client := NewXDSClient(l, conn, nodeID, isProxy)
	ctx, cancel := context.WithCancel(context.Background())
	go client.Listen(ctx)

	return &XDSSuite{
		ln:        ln,
		conn:      conn,
		server:    server,
		monitor:   monitor,
		client:    client,
		nodeID:    nodeID,
		proxyHost: proxyHost,
		cancel:    cancel,
		isProxy:   isProxy,
		scaler:    scaler,
	}
}

func endpoints(port uint32, hosts []string) []xds.Endpoint {
	out := make([]xds.Endpoint, len(hosts))
	for i, host := range hosts {
		out[i] = xds.Endpoint{IP: host, Port: port}
	}
	return out
}

func exclude(in []string, fn func(string) bool) []string {
	out := make([]string, 0, len(in))
	for _, i := range in {
		if !fn(i) {
			out = append(out, i)
		}
	}
	return out
}

type mockScaler struct {
	mu  sync.Mutex
	chs map[string]chan struct{}
}

func (m *mockScaler) Triggered(cluster string) <-chan struct{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.chs[cluster]
}

func (m *mockScaler) ScaleToZero(cluster string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.chs[cluster] != nil {
		close(m.chs[cluster])
		m.chs[cluster] = nil
	}
}

func (m *mockScaler) ScaleFromZero(cluster string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.chs[cluster] = make(chan struct{})
}

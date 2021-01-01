package proxy

import (
	"context"
	"fmt"
	"github.com/rueian/zenvoy/pkg/logger"
	"github.com/rueian/zenvoy/pkg/xds"
	"google.golang.org/grpc"
	"net"
	"strings"
	"testing"
)

type XDSSuite struct {
	ln      net.Listener
	conn    *grpc.ClientConn
	server  *xds.Server
	client  *Client
	nodeID  string
	cancel  context.CancelFunc
	isProxy isProxyFn

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

	suite.client.OnUpdated(func(port uint32) {
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
		{ClusterName: "1", ClusterBind: 1111, EndpointPort: 2222, Hosts: []string{"1.1.1.1", "2.2.2.2", "3.3.3.3"}},
		{ClusterName: "1", ClusterBind: 3333, EndpointPort: 3333, Hosts: []string{suite.proxyHost}},
	} {
		if test.Delete {
			err = suite.server.RemoveClusterEndpoints(test.ClusterName)
			err = suite.server.RemoveCluster(test.ClusterName)
		} else {
			err = suite.server.SetCluster(test.ClusterName)
			err = suite.server.SetClusterEndpoints(test.ClusterName, endpoints(test.EndpointPort, test.Hosts)...)
		}
		if err != nil {
			t.Fatalf("xds error %v", err)
		}

		if test.Delete {
			continue
		}

		<-updated

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
	server := xds.NewServer(l, nodeID)
	go server.Serve(context.Background(), ln)

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
		client:    client,
		nodeID:    nodeID,
		proxyHost: proxyHost,
		cancel:    cancel,
		isProxy:   isProxy,
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

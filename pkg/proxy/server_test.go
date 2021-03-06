package proxy

import (
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/rueian/zenvoy/pkg/logger"
)

type ServerSuite struct {
	store    *store
	server   *Server
	serverLn net.Listener
	targetLn net.Listener
	triggers *int
}

func (s *ServerSuite) Close() {
	s.serverLn.Close()
	s.targetLn.Close()
}

func setup(t *testing.T) *ServerSuite {
	ln1, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ln2, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}

	triggers := 0

	store := NewStore()
	server := NewServer(&logger.Std{}, store, func(s string) {
		triggers++
	})

	go server.Serve(ln1)
	go http.Serve(ln2, http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))

	return &ServerSuite{
		store:    store,
		server:   server,
		serverLn: ln1,
		targetLn: ln2,
		triggers: &triggers,
	}
}

func TestDirectClose(t *testing.T) {
	suite := setup(t)
	defer suite.Close()

	resp, err := http.Get("http://" + suite.serverLn.Addr().String())
	if !strings.Contains(err.Error(), "EOF") &&
		!strings.Contains(err.Error(), "server closed") &&
		!strings.Contains(err.Error(), "connection reset by peer") {
		t.Error(resp, err)
	}
}

func TestDirectRedirect(t *testing.T) {
	suite := setup(t)
	defer suite.Close()

	suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
		Name:      "any",
		Endpoints: []string{suite.targetLn.Addr().String()},
	})

	resp, err := http.Get("http://" + suite.serverLn.Addr().String())
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Error(resp, err)
	}
}

func TestPendingRedirect(t *testing.T) {
	suite := setup(t)
	defer suite.Close()

	suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
		Name: "any",
	})
	go func() {
		time.Sleep(time.Second / 2)
		suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
			Name:      "any",
			Endpoints: []string{suite.targetLn.Addr().String()},
		})
	}()

	resp, err := http.Get("http://" + suite.serverLn.Addr().String())
	if err != nil || resp.StatusCode != http.StatusNoContent {
		t.Error(resp, err)
	}
}

func TestPendingClose(t *testing.T) {
	suite := setup(t)
	defer suite.Close()

	suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
		Name: "any",
	})
	go func() {
		time.Sleep(time.Second / 2)
		suite.store.RemoveCluster("any")
	}()

	resp, err := http.Get("http://" + suite.serverLn.Addr().String())
	if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "connection reset by peer") {
		t.Error(resp, err)
	}
}

func TestDynamicRedirect(t *testing.T) {
	suite := setup(t)
	defer suite.Close()

	suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
		Name: "any",
	})

	stop := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
				Name: "any",
			})
			time.Sleep(time.Second / 10)
			suite.store.SetCluster(addrPort(suite.serverLn.Addr()), Cluster{
				Name:      "any",
				Endpoints: []string{suite.targetLn.Addr().String()},
			})
			time.Sleep(time.Second / 10)
		}
		close(stop)
	}()

loop:
	for i := 0; ; i++ {
		select {
		case <-stop:
			if i == 0 {
				t.Fatal("no request succeeded")
			}
			if *suite.triggers == 0 {
				t.Fatal("no triggers")
			}
			if *suite.triggers >= i {
				t.Fatalf("triggers(%d) >= requests(%d)", *suite.triggers, i)
			}
			break loop
		default:
			resp, err := http.Get("http://" + suite.serverLn.Addr().String())
			if err != nil || resp.StatusCode != http.StatusNoContent {
				t.Error(resp, err)
			}
		}
	}
}

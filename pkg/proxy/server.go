package proxy

import (
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"io"
	"math/rand"
	"net"
	"sync"
)

func NewServer(logger log.Logger, xds XDS, isProxy isProxyFn, trigger func(string)) *Server {
	s := &Server{
		xds:         xds,
		logger:      logger,
		pendingConn: make(map[uint32]map[string]net.Conn),
		triggerFn:   trigger,
		isProxyFn:   isProxy,
	}
	s.xds.OnUpdated(s.onXDSUpdated)
	return s
}

type Server struct {
	logger      log.Logger
	pendingConn map[uint32]map[string]net.Conn
	mu          sync.Mutex
	xds         XDS
	isProxyFn   isProxyFn
	triggerFn   func(string)
}

func (s *Server) Serve(ln net.Listener) error {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	p := port(conn.LocalAddr())
	ces := s.xds.GetIntendedEndpoints(p)
	if len(ces.Endpoints) == 0 {
		conn.Close()
		return
	}

	others := exclude(ces.Endpoints, s.isProxyFn)
	if len(others) == 0 {
		s.pending(p, conn)
		go s.trigger(ces.Cluster)
		return
	}

	endpoint := others[rand.Intn(len(others))]
	s.redirect(endpoint, conn)
}

func (s *Server) pending(port uint32, conn net.Conn) {
	s.mu.Lock()
	m, ok := s.pendingConn[port]
	if !ok {
		m = make(map[string]net.Conn)
		s.pendingConn[port] = m
	}
	m[conn.RemoteAddr().String()] = conn
	s.mu.Unlock()
}

func (s *Server) redirect(endpoint string, conn net.Conn) {
	defer conn.Close()

	conn2, err := net.Dial("tcp", endpoint)
	if err != nil {
		s.logger.Errorf("fail to dial %s: %v", endpoint, err)
		return
	}
	defer conn2.Close()

	go io.Copy(conn, conn2)
	io.Copy(conn2, conn)
}

func (s *Server) trigger(cluster string) {
	if s.triggerFn != nil {
		s.triggerFn(cluster)
	}
}

func (s *Server) onXDSUpdated() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for port, n := range s.pendingConn {
		if len(n) == 0 {
			continue
		}
		ces := s.xds.GetIntendedEndpoints(port)
		if len(ces.Endpoints) == 0 {
			for k, conn := range n {
				delete(n, k)
				go conn.Close()
			}
			continue
		}
		others := exclude(ces.Endpoints, s.isProxyFn)
		if len(others) != 0 {
			for k, conn := range n {
				delete(n, k)
				go s.redirect(others[rand.Intn(len(others))], conn)
			}
		}
	}
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

func port(addr net.Addr) uint32 {
	if addr, ok := addr.(*net.TCPAddr); ok {
		return uint32(addr.Port)
	}
	panic(addr.String() + "is not a tcp address")
}

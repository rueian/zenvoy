package proxy

import (
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"io"
	"math/rand"
	"net"
	"sync"
)

func NewServer(logger log.Logger, xds XDSClient, isProxy isProxyFn, trigger func(string)) *Server {
	s := &Server{
		xdsClient: xds,
		logger:    logger,
		pending:   make(map[uint32][]net.Conn),
		triggerFn: trigger,
		isProxyFn: isProxy,
	}
	s.xdsClient.OnUpdated(s.onXDSUpdated)
	return s
}

type Server struct {
	mu        sync.Mutex
	logger    log.Logger
	pending   map[uint32][]net.Conn
	xdsClient XDSClient
	isProxyFn isProxyFn
	triggerFn func(string)
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
	port := addrPort(conn.LocalAddr())
	cluster := s.xdsClient.GetCluster(port)
	if len(cluster.Endpoints) == 0 {
		conn.Close()
		return
	}

	others := exclude(cluster.Endpoints, s.isProxyFn)
	if len(others) == 0 {
		s.holding(port, conn)
		go s.trigger(cluster.Name)
		return
	}

	s.redirect(others[rand.Intn(len(others))], conn)
}

func (s *Server) holding(port uint32, conn net.Conn) {
	s.mu.Lock()
	pending, ok := s.pending[port]
	if !ok {
		pending = make([]net.Conn, 0, 10)
	}
	s.pending[port] = append(pending, conn)
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

func (s *Server) onXDSUpdated(port uint32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.pending[port]; ok && len(n) > 0 {
		ces := s.xdsClient.GetCluster(port)
		if len(ces.Endpoints) == 0 {
			for _, conn := range n {
				go conn.Close()
			}
			delete(s.pending, port)
			return
		}
		others := exclude(ces.Endpoints, s.isProxyFn)
		if len(others) != 0 {
			for _, conn := range n {
				go s.redirect(others[rand.Intn(len(others))], conn)
			}
			delete(s.pending, port)
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

func addrPort(addr net.Addr) uint32 {
	if addr, ok := addr.(*net.TCPAddr); ok {
		return uint32(addr.Port)
	}
	panic(addr.String() + "is not a tcp address")
}

package proxy

import (
	"github.com/envoyproxy/go-control-plane/pkg/log"
	"io"
	"math/rand"
	"net"
	"strconv"
	"sync"
)

func NewServer(logger log.Logger, xds XDS) *Server {
	s := &Server{
		logger:      logger,
		pendingConn: make(map[uint32]map[string]net.Conn),
		xds:         xds,
	}
	s.xds.OnUpdated(s.onXDSUpdated)
	return s
}

type Server struct {
	logger      log.Logger
	pendingConn map[uint32]map[string]net.Conn
	mu          sync.Mutex

	xds XDS
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
	var port uint64

	_, ps, err := net.SplitHostPort(conn.LocalAddr().String())
	if err == nil {
		port, err = strconv.ParseUint(ps, 10, 32)
	}
	if err != nil {
		s.logger.Errorf("fail to parse port from %v", conn.LocalAddr().String())
		conn.Close()
		return
	}

	endpoints := s.xds.GetIntendedEndpoints(uint32(port))
	if len(endpoints) == 0 {
		conn.Close()
		return
	}

	others := exclude(endpoints, conn.LocalAddr().String())
	if len(others) == 0 {
		s.pending(uint32(port), conn)
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

func (s *Server) onXDSUpdated() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for port, n := range s.pendingConn {
		if len(n) == 0 {
			continue
		}
		endpoints := s.xds.GetIntendedEndpoints(port)
		if len(endpoints) == 0 {
			for k, conn := range n {
				delete(n, k)
				go conn.Close()
			}
			continue
		}
		others := exclude(endpoints, first(n).LocalAddr().String())
		if len(others) != 0 {
			for k, conn := range n {
				delete(n, k)
				go s.redirect(others[rand.Intn(len(others))], conn)
			}
		}
	}
}

func exclude(in []string, x string) []string {
	out := make([]string, 0, len(in))
	for _, i := range in {
		if i != x {
			out = append(out, i)
		}
	}
	return out
}

func first(m map[string]net.Conn) net.Conn {
	for _, v := range m {
		return v
	}
	return nil
}

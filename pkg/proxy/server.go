package proxy

import (
	"io"
	"math/rand"
	"net"
	"sync"

	"github.com/envoyproxy/go-control-plane/pkg/log"
)

func NewServer(logger log.Logger, xds XDSClient, trigger func(string)) *Server {
	s := &Server{
		xdsClient: xds,
		logger:    logger,
		pending:   make(map[string][]net.Conn),
		triggerFn: trigger,
	}
	s.xdsClient.OnUpdated(s.onXDSUpdated)
	return s
}

type Server struct {
	mu        sync.Mutex
	logger    log.Logger
	pending   map[string][]net.Conn
	xdsClient XDSClient
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
	if cluster.Name == "" {
		conn.Close()
		return
	}

	if endpoints := cluster.Endpoints; len(endpoints) == 0 {
		s.holding(cluster.Name, conn)
		go s.trigger(cluster.Name)
	} else {
		s.redirect(endpoints[rand.Intn(len(endpoints))], conn)
	}
}

func (s *Server) holding(cluster string, conn net.Conn) {
	s.mu.Lock()
	pending, ok := s.pending[cluster]
	if !ok {
		pending = make([]net.Conn, 0, 10)
	}
	s.pending[cluster] = append(pending, conn)
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

	// TODO: bpf_sk_redirect_map + sock_map to bypass userspace forwarding
	go io.Copy(conn, conn2)
	io.Copy(conn2, conn)
}

func (s *Server) trigger(cluster string) {
	if s.triggerFn != nil {
		s.triggerFn(cluster)
	}
}

func (s *Server) onXDSUpdated(cluster Cluster, deleted bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if n, ok := s.pending[cluster.Name]; ok && len(n) > 0 {
		if deleted {
			for _, conn := range n {
				go conn.Close()
			}
			delete(s.pending, cluster.Name)
		} else if endpoints := cluster.Endpoints; len(endpoints) != 0 {
			for _, conn := range n {
				go s.redirect(endpoints[rand.Intn(len(endpoints))], conn)
			}
			delete(s.pending, cluster.Name)
		}
	}
}

func addrPort(addr net.Addr) uint32 {
	if addr, ok := addr.(*net.TCPAddr); ok {
		return uint32(addr.Port)
	}
	panic(addr.String() + "is not a tcp address")
}

package xds

import (
	"strings"
	"sync"
	"time"

	metricsservice "github.com/envoyproxy/go-control-plane/envoy/service/metrics/v3"
	prom "github.com/prometheus/client_model/go"
)

type Scaler interface {
	ScaleToZero(cluster string)
	ScaleFromZero(cluster string)
}

type stat struct {
	liveEndpoints  int
	requestTotal   float64
	lastUpdateTime int64
}

const TriggerMetric = "upstream_rq_total"

type MonitorOptions struct {
	ScaleToZeroAfter time.Duration
	ScaleToZeroCheck time.Duration
}

func NewMonitorServer(scaler Scaler, options MonitorOptions) *MonitorServer {
	s := &MonitorServer{
		options:  options,
		clusters: make(map[string]clusterStats),
		scaler:   scaler,
	}
	go s.idleClusterReaper()
	return s
}

type clusterStats struct {
	endpoints int
	stats     map[string]stat
}

type MonitorServer struct {
	mu       sync.RWMutex
	options  MonitorOptions
	clusters map[string]clusterStats
	scaler   Scaler
}

func (s *MonitorServer) TrackCluster(name string, endpoints int) {
	s.mu.Lock()
	if prev, ok := s.clusters[name]; ok {
		prev.endpoints = endpoints
		s.clusters[name] = prev
	} else {
		s.clusters[name] = clusterStats{endpoints: endpoints, stats: make(map[string]stat)}
	}
	s.mu.Unlock()
}

func (s *MonitorServer) RemoveCluster(cluster string) {
	s.mu.Lock()
	delete(s.clusters, cluster)
	s.mu.Unlock()
}

func (s *MonitorServer) StreamMetrics(server metricsservice.MetricsService_StreamMetricsServer) error {
	defer server.SendAndClose(&metricsservice.StreamMetricsResponse{})

	var id string

	for {
		msg, err := server.Recv()
		if err != nil {
			return err
		}

		// the Identifier field will only present in the first message,
		// we must manually check it before using it.
		if msg.Identifier != nil {
			id = msg.Identifier.GetNode().GetId()
		}

		s.mu.Lock()
		for _, m := range msg.EnvoyMetrics {
			if *m.Type == prom.MetricType_COUNTER && len(m.Metric) > 0 {
				s.processCounter(id, m)
			}
		}
		s.mu.Unlock()
	}
}

func (s *MonitorServer) processCounter(id string, m *prom.MetricFamily) {
	if mn := *m.Name; strings.HasSuffix(mn, TriggerMetric) {
		if parts := strings.Split(mn, "."); len(parts) == 3 {
			name := parts[1]
			cluster := s.clusters[name]
			if cluster.stats == nil {
				cluster.stats = make(map[string]stat)
			}
			prev := cluster.stats[id]

			requestTotal := *m.Metric[0].Counter.Value // Value means the amount of received requestTotal.
			lastUpdateTime := *m.Metric[0].TimestampMs

			// If two request counts are the same means no new requestTotal received,
			// hence we don't need to update the cluster state.
			if requestTotal != prev.requestTotal {
				prev.requestTotal = requestTotal
				prev.lastUpdateTime = lastUpdateTime
				cluster.stats[id] = prev
				if requestTotal > 0 {
					go s.scaler.ScaleFromZero(name)
				}
			}
			s.clusters[name] = cluster // always assign it for ensuring we will handle it in the reaper
		}
	}
}

func (s *MonitorServer) idleClusterReaper() {
	for {
		time.Sleep(s.options.ScaleToZeroCheck)
		now := time.Now().UnixNano() / 1e6

		s.mu.RLock()
		for name, cluster := range s.clusters {
			var lastUpdateTime int64
			for _, stat := range cluster.stats {
				if stat.lastUpdateTime > lastUpdateTime {
					lastUpdateTime = stat.lastUpdateTime
				}
			}

			canScaleDown := now-lastUpdateTime > s.options.ScaleToZeroAfter.Milliseconds()

			if cluster.endpoints > 0 && canScaleDown {
				go s.scaler.ScaleToZero(name)
			}
		}
		s.mu.RUnlock()
	}
}

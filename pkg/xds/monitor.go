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
		clusters: make(map[string]stat),
		scaler:   scaler,
	}
	go s.idleClusterReaper()
	return s
}

type MonitorServer struct {
	mu       sync.RWMutex
	options  MonitorOptions
	clusters map[string]stat
	scaler   Scaler
}

func (s *MonitorServer) TrackCluster(cluster string, endpoints int) {
	s.mu.Lock()
	if prev, ok := s.clusters[cluster]; ok {
		prev.liveEndpoints = endpoints
		s.clusters[cluster] = prev
	} else {
		s.clusters[cluster] = stat{liveEndpoints: endpoints, lastUpdateTime: time.Now().UnixNano() / 1e6}
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
	for {
		msg, err := server.Recv()
		if err != nil {
			return err
		}
		s.mu.Lock()
		for _, m := range msg.EnvoyMetrics {
			if *m.Type == prom.MetricType_COUNTER && len(m.Metric) > 0 {
				s.processCounter(m)
			}
		}
		s.mu.Unlock()
	}
}

func (s *MonitorServer) processCounter(m *prom.MetricFamily) {
	if mn := *m.Name; strings.HasSuffix(mn, TriggerMetric) {
		if parts := strings.Split(mn, "."); len(parts) == 3 {
			name := parts[1]
			prev := s.clusters[name]
			requestTotal := *m.Metric[0].Counter.Value // Value means the amount of received requestTotal.
			lastUpdateTime := *m.Metric[0].TimestampMs

			// If two request counts are the same means no new requestTotal received,
			// hence we don't need to update the cluster state.
			if requestTotal != prev.requestTotal {
				prev.requestTotal = requestTotal
				prev.lastUpdateTime = lastUpdateTime
				if requestTotal > 0 {
					go s.scaler.ScaleFromZero(name)
				}
			}
			s.clusters[name] = prev
		}
	}
}

func (s *MonitorServer) idleClusterReaper() {
	for {
		time.Sleep(s.options.ScaleToZeroCheck)
		now := time.Now().UnixNano() / 1e6

		s.mu.RLock()
		for cluster, stat := range s.clusters {
			canScaleDown := now-stat.lastUpdateTime > s.options.ScaleToZeroAfter.Milliseconds()

			if stat.liveEndpoints > 0 && canScaleDown {
				go s.scaler.ScaleToZero(cluster)
			}
		}
		s.mu.RUnlock()
	}
}

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
	isActive       bool
	requestCount   float64
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
	mu       sync.Mutex
	options  MonitorOptions
	clusters map[string]stat
	scaler   Scaler
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
			currRequestCount := *m.Metric[0].Counter.Value // Value means the amount of received requests.

			// If two request counts are the same means no new requests received,
			// hence we don't need to update the cluster state.
			if currRequestCount == s.clusters[name].requestCount {
				return
			}

			go s.scaler.ScaleFromZero(name)
			s.clusters[name] = stat{
				requestCount:   *m.Metric[0].Counter.Value,
				lastUpdateTime: *m.Metric[0].TimestampMs,
				isActive:       true,
			}
		}
	}
}

func (s *MonitorServer) idleClusterReaper() {
	for {
		time.Sleep(s.options.ScaleToZeroCheck)
		now := time.Now().UnixNano() / 1e6

		s.mu.Lock()
		for cluster, stat := range s.clusters {
			isNowIdle := now-stat.lastUpdateTime > s.options.ScaleToZeroAfter.Milliseconds()

			if stat.isActive && isNowIdle {
				stat.isActive = false
				s.clusters[cluster] = stat
				go s.scaler.ScaleToZero(cluster)
			}
		}
		s.mu.Unlock()
	}
}

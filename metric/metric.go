package metric

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var counters = map[string]*prometheus.CounterVec{}

type MetricEvent struct {
	Name   string
	Labels map[string]string
	Value  float64
}

type Server struct {
	conf *Config
}

type Config struct {
	Port int `default:"4014"`
}

func New(conf *Config) *Server {
	if conf == nil {
		conf = &Config{}
		envconfig.MustProcess("metric", conf)
	}
	return &Server{conf: conf}
}

func (s *Server) Start() error {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.conf.Port),
		Handler:           mux,
		ReadHeaderTimeout: time.Second * 5,
	}
	return server.ListenAndServe()
}

// RecordEvent auto register counter
func RecordEvent(e MetricEvent) {
	labelKeys := []string{}
	labelValues := []string{}
	for k, v := range e.Labels {
		labelKeys = append(labelKeys, k)
		labelValues = append(labelValues, v)
	}

	// 缓存 & 注册
	counter, ok := counters[e.Name]
	if !ok {
		counter = prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: e.Name, Help: "auto generated"},
			labelKeys,
		)
		prometheus.MustRegister(counter)
		counters[e.Name] = counter
	}

	counter.WithLabelValues(labelValues...).Add(e.Value)
}

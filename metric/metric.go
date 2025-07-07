package metric

import (
	"fmt"
	"github.com/prometheus/client_golang/prometheus"
	"net/http"
	"sort"
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
	// 1. 对 labelKeys 排序
	labelKeys := make([]string, 0, len(e.Labels))
	for k := range e.Labels {
		labelKeys = append(labelKeys, k)
	}
	sort.Strings(labelKeys)

	// 2. 根据排序后的 keys 构造 labelValues
	labelValues := make([]string, 0, len(labelKeys))
	for _, k := range labelKeys {
		labelValues = append(labelValues, e.Labels[k])
	}

	// 3. 注册并缓存 CounterVec
	counter, ok := counters[e.Name]
	if !ok {
		counter = prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: e.Name, Help: "auto generated"},
			labelKeys,
		)
		prometheus.MustRegister(counter)
		counters[e.Name] = counter
	}

	// 4. 打点
	counter.WithLabelValues(labelValues...).Add(e.Value)
}

package metric

import (
	"fmt"
	"net/http"
	"sort"
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type MetricEvent struct {
	Name   string
	Labels map[string]string
	Value  float64
}

type counterEntry struct {
	once      sync.Once
	counter   *prometheus.CounterVec
	labelKeys []string
}

var counters sync.Map

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

// RecordEvent auto register counter, now concurrency-safe.
func RecordEvent(e MetricEvent) {
	if e.Name == "" {
		return
	}

	if e.Labels == nil {
		e.Labels = map[string]string{}
	}

	entryIface, _ := counters.LoadOrStore(e.Name, &counterEntry{})
	entry := entryIface.(*counterEntry)

	entry.once.Do(func() {
		labelKeys := make([]string, 0, len(e.Labels))
		for k := range e.Labels {
			labelKeys = append(labelKeys, k)
		}
		sort.Strings(labelKeys)

		entry.labelKeys = labelKeys
		entry.counter = prometheus.NewCounterVec(
			prometheus.CounterOpts{Name: e.Name, Help: "auto generated"},
			labelKeys,
		)
		prometheus.MustRegister(entry.counter)
	})

	labelValues := make([]string, len(entry.labelKeys))
	for idx, key := range entry.labelKeys {
		labelValues[idx] = e.Labels[key]
	}

	entry.counter.WithLabelValues(labelValues...).Add(e.Value)
}

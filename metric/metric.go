package metric

import (
	"fmt"
	"net/http"

	"github.com/kelseyhightower/envconfig"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

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
		Addr:    fmt.Sprintf(":%d", s.conf.Port),
		Handler: mux,
	}
	return server.ListenAndServe()
}

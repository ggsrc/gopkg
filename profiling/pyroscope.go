package profiling

import (
	"time"

	"dario.cat/mergo"
	"github.com/grafana/pyroscope-go"
	"github.com/kelseyhightower/envconfig"

	"github.com/ggsrc/gopkg/env"
)

type Server struct {
	conf     pyroscope.Config
	Profiler *pyroscope.Profiler
}

// Config 只把环境变量难以注入的字段暴露出来
type Config struct {
	Tags         map[string]string
	UploadRate   time.Duration
	Logger       pyroscope.Logger
	ProfileTypes []pyroscope.ProfileType
	HTTPHeaders  map[string]string
}

func InitProfiler(conf *Config) *Server {
	envConfig := pyroscope.Config{}
	envconfig.MustProcess("profiling", &envConfig)
	if envConfig.ApplicationName == "" {
		envConfig.ApplicationName = env.ServiceName()
	}
	if len(conf.ProfileTypes) == 0 {
		conf.ProfileTypes = pyroscope.DefaultProfileTypes
	}
	//nolint:errcheck
	mergo.Merge(&envConfig, conf)
	return &Server{
		conf:     envConfig,
		Profiler: nil,
	}
}

func (s *Server) Start() (err error) {
	s.Profiler, err = pyroscope.Start(s.conf)
	if err != nil {
		return err
	}
	return nil
}

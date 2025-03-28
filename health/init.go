package health

import (
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog/log"
)

func InitHealthCheck(hc ...HealthCheckable) *Server {
	c := Config{}
	envconfig.MustProcess("healthcheck", &c)
	log.Warn().Msgf("healthcheck Config: %+v", c)
	checker := New(&c, nil, hc...)
	return checker
}

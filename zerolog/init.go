package zerolog

import (
	"github.com/ggsrc/gopkg/env"
	"github.com/rs/zerolog"
	"github.com/uptrace/uptrace-go/uptrace"
)

func InitLogger(debug bool) {
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	// zerolog.DefaultContextLogger = &log.Logger
	uptrace.ConfigureOpentelemetry(
		uptrace.WithDeploymentEnvironment(env.Env()),
		uptrace.WithServiceVersion(env.ServiceVersion()+"-"+env.BuildTime()),
	)
	InitDefaultLogger()
}

package zerolog

import (
	"github.com/rs/zerolog"
	"github.com/uptrace/uptrace-go/uptrace"

	"github.com/ggsrc/gopkg/env"
)

func InitLogger(debug bool) {
	if debug {
		zerolog.SetGlobalLevel(zerolog.DebugLevel)
	} else {
		zerolog.SetGlobalLevel(zerolog.InfoLevel)
	}
	// zerolog.DefaultContextLogger = &log.Logger
	if env.IsUnitTest() {
		return
	}
	uptrace.ConfigureOpentelemetry(
		uptrace.WithDeploymentEnvironment(env.Env()),
		uptrace.WithServiceVersion(env.ServiceVersion()+"-"+env.BuildTime()),
	)
	InitDefaultLogger()
}

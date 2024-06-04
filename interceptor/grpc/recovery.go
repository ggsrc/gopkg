package grpcinterceptor

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/getsentry/sentry-go"
	"github.com/rs/zerolog/log"
	"google.golang.org/grpc"

	"github.com/ggsrc/gopkg/env"
)

const RecoverLogKey = "khturNQNRuAJ"

func SentryUnaryServerInterceptor(ravenDSN string) grpc.UnaryServerInterceptor {
	sentry.Init(sentry.ClientOptions{Dsn: ravenDSN})
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				eID := sentry.CaptureException(fmt.Errorf("%v", r))

				log.Info().Msgf("Recovered from panic: %v", r)
				log.Info().Msg("Stack trace of the panic:")
				log.Info().Msg(string(debug.Stack()))

				buf := make([]byte, 64<<10)
				buf = buf[:runtime.Stack(buf, false)]
				e := fmt.Errorf("%v %s", r, buf)
				log.Err(e).Msgf("Panic captured by sentry: %s, %v", RecoverLogKey, eID)

				err = fmt.Errorf("server Internal Error")
			}
		}()

		resp, err = handler(ctx, req)
		return resp, err
	}
}

func SentryUnaryClientInterceptor(ravenDSN string) grpc.UnaryClientInterceptor {
	sentry.Init(sentry.ClientOptions{Dsn: ravenDSN, Environment: env.Env()})

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		defer func() {
			if r := recover(); r != nil {
				eID := sentry.CaptureException(fmt.Errorf("%v", r))

				log.Info().Msgf("Recovered from panic: %v", r)
				log.Info().Msg("Stack trace of the panic:")
				log.Info().Msg(string(debug.Stack()))

				buf := make([]byte, 64<<10)
				buf = buf[:runtime.Stack(buf, false)]
				e := fmt.Errorf("%v %s", r, buf)
				log.Err(e).Msgf("Panic captured by sentry: %s, %v", RecoverLogKey, eID)

				err = fmt.Errorf("server Internal Error")
			}
		}()

		err = invoker(ctx, method, req, reply, cc, opts...)
		return err
	}
}

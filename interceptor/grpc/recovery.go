package grpcinterceptor

import (
	"context"
	"fmt"
	"runtime"
	"runtime/debug"

	"github.com/getsentry/sentry-go"
	"google.golang.org/grpc"

	"github.com/ggsrc/gopkg/env"
	"github.com/ggsrc/gopkg/zerolog/log"
)

const RecoverLogKey = "khturNQNRuAJ"

func SentryUnaryServerInterceptor(ravenDSN string) grpc.UnaryServerInterceptor {
	err := sentry.Init(sentry.ClientOptions{Dsn: ravenDSN})
	if err != nil {
		log.Err(err).Msg("sentry init failed, ignore it and continue...")
	}
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (resp interface{}, err error) {
		defer func() {
			if r := recover(); r != nil {
				log.Ctx(ctx).Info().Msgf("Recovered from panic: %v", r)
				log.Ctx(ctx).Error().Stack().Msg("Stack trace of the panic:")
				err = fmt.Errorf("server Internal Error")
			}
		}()

		resp, err = handler(ctx, req)
		if err != nil {
			log.Err(err).Err(err).Msg("server error")
		}
		return resp, err
	}
}

func SentryUnaryClientInterceptor(ravenDSN string) grpc.UnaryClientInterceptor {
	err := sentry.Init(sentry.ClientOptions{Dsn: ravenDSN, Environment: env.Env()})
	if err != nil {
		log.Err(err).Msg("sentry init failed")
	}

	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) (err error) {
		defer func() {
			if r := recover(); r != nil {
				eID := sentry.CaptureException(fmt.Errorf("%v", r))

				log.Ctx(ctx).Info().Msgf("Recovered from panic: %v", r)
				log.Ctx(ctx).Info().Msg("Stack trace of the panic:")
				log.Ctx(ctx).Info().Msg(string(debug.Stack()))

				buf := make([]byte, 64<<10)
				buf = buf[:runtime.Stack(buf, false)]
				e := fmt.Errorf("%v %s", r, buf)
				log.Ctx(ctx).Err(e).Msgf("Panic captured by sentry: %s, %v", RecoverLogKey, eID)

				err = fmt.Errorf("server Internal Error")
			}
		}()

		err = invoker(ctx, method, req, reply, cc, opts...)
		return err
	}
}

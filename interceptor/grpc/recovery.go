package grpcinterceptor

import (
	"context"
	"encoding/json"
	"fmt"
	"runtime/debug"
	"strings"

	"github.com/getsentry/sentry-go"
	"github.com/jinzhu/copier"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"

	"github.com/ggsrc/gopkg/env"
	"github.com/ggsrc/gopkg/interceptor/grpc/errors"
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
				log.Ctx(ctx).Error().
					Str("panic.stack", string(debug.Stack())).
					Err(fmt.Errorf("[panic] %v", r)).
					Msgf("%s grpc server panic", strings.Trim(info.FullMethod, "/"))
				err = fmt.Errorf("server Internal Error")
			}
		}()

		resp, err = handler(ctx, req)
		if err != nil {
			var subErrs []*errors.ErrorInfo
			grpcStatus, ok := status.FromError(err)
			if ok {
				for _, errorDetail := range grpcStatus.Details() {
					if errorDetail == nil {
						continue
					}
					var subErr errors.ErrorInfo
					copyErr := copier.Copy(&subErr, errorDetail)
					if copyErr != nil {
						continue
					}
					subErrs = append(subErrs, &subErr)
				}
				subErrStr, _ := json.Marshal(subErrs)
				log.Ctx(ctx).Error().Str("sub_errors", string(subErrStr)).Err(err).Msg("grpc server error")
			} else {
				log.Ctx(ctx).Error().Err(err).Msg("grpc server error")
			}
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
				log.Ctx(ctx).Error().
					Str("panic.stack", string(debug.Stack())).
					Err(fmt.Errorf("[panic] %v", r)).
					Msgf("%s grpc client panic", strings.Trim(method, "/"))
				err = fmt.Errorf("server Internal Error")
			}
		}()

		err = invoker(ctx, method, req, reply, cc, opts...)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("grpc client error")
		}
		return err
	}
}

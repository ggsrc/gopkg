package grpcinterceptor

import (
	"context"
	"strconv"
	"time"

	"github.com/bytedance/gopkg/util/xxhash3"
	"github.com/bytedance/sonic"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/ggsrc/gopkg/utils"
	"github.com/ggsrc/gopkg/zerolog/log"
)

var g singleflight.Group

func rpcCacheKey(method string, req interface{}) (string, error) {
	// Add a string to the hash, and print the current hash value.
	reqBytes, err := sonic.Marshal(req)
	if err != nil {
		return "", err
	}
	return method + ":" + strconv.FormatUint(xxhash3.Hash(reqBytes), 16), nil
}

func ContextCacheUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if !utils.ContextCacheExists(ctx) && !utils.SingleflightEnable(ctx) {
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		cacheKey, err := rpcCacheKey(method, req)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Generate rpc cacheKey error")
			return invoker(ctx, method, req, reply, cc, opts...)
		}
		// 开启了 context cache
		if utils.ContextCacheExists(ctx) {
			log.Ctx(ctx).Info().Msg("context cache enabled")
			grpcReply, err := utils.LoadFromCtxCache(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
				if utils.SingleflightEnable(ctx) {
					log.Ctx(ctx).Info().Msg("singleflight enabled")
					_, err, _ = g.Do(cacheKey, func() (interface{}, error) {
						go func() {
							time.Sleep(100 * time.Millisecond)
							g.Forget(cacheKey)
						}()
						return nil, invoker(ctx, method, req, reply, cc, opts...)
					})
					if err != nil {
						log.Ctx(ctx).Error().Interface("reply", reply).Msg("reply")
						return nil, err
					}
				} else {
					err = invoker(ctx, method, req, reply, cc, opts...)
					if err != nil {
						log.Ctx(ctx).Error().Interface("reply", reply).Msg("reply")
						return nil, err
					}
				}
				return reply, nil
			})
			log.Ctx(ctx).Info().Err(err).
				Interface("reply", reply).
				Interface("grpcReply", grpcReply).
				Msg("reply")
			if reply != grpcReply {
				if replyMsg, ok := reply.(proto.Message); ok {
					if grpcReplyMsg, ok := grpcReply.(proto.Message); ok {
						proto.Reset(replyMsg)
						proto.Merge(replyMsg, grpcReplyMsg)
					}
				} else {
					log.Ctx(ctx).Error().Msg("reply is not proto.Message")
				}
			}
			return err
		}
		// 只有 singleflight
		if utils.SingleflightEnable(ctx) {
			_, err, _ := g.Do(cacheKey, func() (interface{}, error) {
				go func() {
					time.Sleep(100 * time.Millisecond)
					g.Forget(cacheKey)
				}()
				return nil, invoker(ctx, method, req, reply, cc, opts...)
			})
			return err
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

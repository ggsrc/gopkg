package grpcinterceptor

import (
	"context"
	"strconv"
	"time"

	"github.com/bytedance/gopkg/util/xxhash3"
	"github.com/bytedance/sonic"
	"github.com/dgraph-io/ristretto"
	"go.opentelemetry.io/otel/attribute"
	"golang.org/x/sync/singleflight"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/proto"

	"github.com/ggsrc/gopkg/utils"
	"github.com/ggsrc/gopkg/zerolog/log"
)

func rpcCacheKey(method string, req interface{}) (string, error) {
	// Add a string to the hash, and print the current hash value.
	reqBytes, err := sonic.Marshal(req)
	if err != nil {
		return "", err
	}
	return method + ":" + strconv.FormatUint(xxhash3.Hash(reqBytes), 16), nil
}

func ContextCacheUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	var g singleflight.Group
	cache, _ := ristretto.NewCache(&ristretto.Config{
		NumCounters: 1e7,     // number of keys to track frequency of (10M).
		MaxCost:     1 << 26, // maximum cost of cache (64MB).
		BufferItems: 64,      // number of keys per Get buffer.
	})

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
			ctx2, span := utils.StartTrace(ctx, "rpcCtxCache")
			span.SetAttributes(attribute.String("cacheKey", cacheKey))
			defer span.End()
			grpcReply, err := utils.LoadFromCtxCache(ctx2, cacheKey, func(ctx context.Context) (interface{}, error) {
				if utils.SingleflightEnable(ctx2) {
					ctx3, span2 := utils.StartTrace(ctx2, "rpcSfCache")
					span2.SetAttributes(attribute.String("cacheKey", cacheKey))
					defer span2.End()
					reply2, err, _ := g.Do(cacheKey, func() (interface{}, error) {
						ctx4, span3 := utils.StartTrace(ctx3, "rpcInvoke")
						span3.SetAttributes(attribute.String("cacheKey", cacheKey))
						defer span3.End()

						cacheReply, ok := cache.Get(cacheKey)
						if ok {
							return cacheReply, nil
						}
						err2 := invoker(ctx4, method, req, reply, cc, opts...)
						if err2 != nil {
							return nil, err2
						}
						cache.SetWithTTL(cacheKey, reply, 1, time.Microsecond*10)
						return reply, nil
					})
					if err != nil {
						return nil, err
					}
					return reply2, nil
				} else {
					err = invoker(ctx2, method, req, reply, cc, opts...)
					if err != nil {
						return nil, err
					}
				}
				return reply, nil
			})
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
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

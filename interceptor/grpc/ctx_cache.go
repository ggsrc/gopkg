package grpcinterceptor

import (
	"context"
	"strconv"
	"time"

	"github.com/bytedance/gopkg/util/xxhash3"
	"github.com/bytedance/sonic"
	"github.com/kelseyhightower/envconfig"
	"github.com/maypok86/otter"
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

type CacheConfig struct {
	Capacity int `default:"10000"`
	TTL      int `default:"50"`
}

var (
	cache otter.Cache[string, any]
)

func init() {
	conf := &CacheConfig{}
	envconfig.MustProcess("grpc_cache", conf)

	var err error
	cache, err = otter.MustBuilder[string, any](conf.Capacity).
		CollectStats().
		Cost(func(key string, value any) uint32 {
			return 1
		}).
		WithTTL(time.Millisecond * time.Duration(conf.TTL)).
		Build()
	if err != nil {
		panic(err)
	}
}

func ContextCacheUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	var g singleflight.Group

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
			grpcReply, err := utils.LoadFromCtxCache(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
				if utils.SingleflightEnable(ctx) {
					reply2, err, _ := g.Do(cacheKey, func() (interface{}, error) {
						go func() {
							time.Sleep(time.Millisecond * 100)
							g.Forget(cacheKey)
						}()
						if utils.MemCacheEnable(ctx) {
							cacheReply, ok := cache.Get(cacheKey)
							if ok {
								return cacheReply, nil
							}
						}
						err2 := invoker(ctx, method, req, reply, cc, opts...)
						if err2 != nil {
							return nil, err2
						}
						if utils.MemCacheEnable(ctx) {
							cache.SetIfAbsent(cacheKey, reply)
						}
						return reply, nil
					})
					if err != nil {
						return nil, err
					}
					return reply2, nil
				} else {
					err = invoker(ctx, method, req, reply, cc, opts...)
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

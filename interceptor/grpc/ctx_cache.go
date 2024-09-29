package grpcinterceptor

import (
	"context"
	"strconv"

	"github.com/bytedance/gopkg/util/xxhash3"
	"github.com/bytedance/sonic"
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
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		if utils.ContextCacheExists(ctx) {
			cacheKey, err := rpcCacheKey(method, req)
			if err != nil {
				log.Ctx(ctx).Error().Err(err).Msg("Generate rpc cacheKey error")
			} else {
				grpcReply, err := utils.LoadFromCtxCache(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
					err = invoker(ctx, method, req, reply, cc, opts...)
					if err != nil {
						return "", err
					}
					return reply, nil
				})
				if replyMsg, ok := reply.(proto.Message); ok {
					if grpcReplyMsg, ok := grpcReply.(proto.Message); ok {
						proto.Reset(replyMsg)
						proto.Merge(replyMsg, grpcReplyMsg)
					}
				} else {
					log.Ctx(ctx).Error().Msg("reply is not proto.Message")
				}
				return err
			}
		}
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

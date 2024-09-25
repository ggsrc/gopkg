package grpcinterceptor

import (
	"context"
	"hash/maphash"
	"strconv"

	"github.com/bytedance/sonic"
	"google.golang.org/grpc"

	"github.com/ggsrc/gopkg/utils"
	"github.com/ggsrc/gopkg/zerolog/log"
)

func RpcCacheKey(method string, req interface{}) (string, error) {
	var h maphash.Hash

	// Add a string to the hash, and print the current hash value.
	reqBytes, err := sonic.Marshal(req)
	if err != nil {
		return "", err
	}
	_, err = h.Write(reqBytes)
	if err != nil {
		return "", err
	}
	return method + strconv.FormatUint(h.Sum64(), 16), nil
}

func ContextCacheUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		cacheKey, err := RpcCacheKey(method, req)
		if err != nil {
			log.Ctx(ctx).Error().Err(err).Msg("Generate rpc cacheKey error")
		}
		grpcReply, err := utils.LoadFromCtxCache(ctx, cacheKey, func(ctx context.Context) (interface{}, error) {
			err = invoker(ctx, method, req, reply, cc, opts...)
			if err != nil {
				return "", err
			}
			return reply, nil
		})
		if err != nil {
			return err
		}
		reply = grpcReply
		return nil
	}
}

package grpcinterceptor

import (
	"context"

	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"

	"github.com/ggsrc/gopkg/interceptor/grpc/metautils"
	pkgmetadata "github.com/ggsrc/gopkg/interceptor/metadata"
)

func ContextUnaryServerInterceptor() grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		// set request source
		md := metautils.ExtractIncoming(ctx)
		requestSource := md.Get(pkgmetadata.CTX_KEY_REQUEST_SOURCE)
		//nolint:golint,staticcheck
		ctx = context.WithValue(ctx, pkgmetadata.CTX_KEY_REQUEST_SOURCE, requestSource)

		// set access token
		accessToken := md.Get(pkgmetadata.CTX_KEY_ACCESS_TOKEN)
		//nolint:golint,staticcheck
		ctx = context.WithValue(ctx, pkgmetadata.CTX_KEY_ACCESS_TOKEN, accessToken)

		// set galxeId
		galxeId := md.Get(pkgmetadata.CTX_KEY_GALXE_ID)
		//nolint:golint,staticcheck
		ctx = context.WithValue(ctx, pkgmetadata.CTX_KEY_GALXE_ID, galxeId)

		// set origin
		origin := md.Get(pkgmetadata.CTX_KEY_ORIGIN)
		//nolint:golint,staticcheck
		ctx = context.WithValue(ctx, pkgmetadata.CTX_KEY_ORIGIN, origin)

		// set account
		accountId := md.Get(pkgmetadata.CTX_KEY_ACCOUNT_ID)
		//nolint:golint,staticcheck
		ctx = context.WithValue(ctx, pkgmetadata.CTX_KEY_ACCOUNT_ID, accountId)

		// set account type
		accountType := md.Get(pkgmetadata.CTX_KEY_ACCOUNT_TYPE)
		//nolint:golint,staticcheck
		ctx = context.WithValue(ctx, pkgmetadata.CTX_KEY_ACCOUNT_TYPE, accountType)

		ret, err := handler(ctx, req)
		return ret, err
	}
}

func ContextUnaryClientInterceptor() grpc.UnaryClientInterceptor {
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		md, ok := metadata.FromIncomingContext(ctx)
		if ok {
			md = md.Copy()
		} else {
			md = metadata.MD{}
		}
		outgoingmd, ok := metadata.FromOutgoingContext(ctx)
		if ok {
			// explicitly declared outgoing md take precedence over transitive incoming md
			md = metadata.Join(outgoingmd, md)
		}
		ctx = metadata.NewOutgoingContext(ctx, md)
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

func NewContextWithGRPCMeta(ctx context.Context) context.Context {
	newCtx := context.Background()
	md, ok := metadata.FromIncomingContext(ctx)
	if ok {
		newCtx = metadata.NewIncomingContext(newCtx, md)
	}
	md, ok = metadata.FromOutgoingContext(ctx)
	if ok {
		newCtx = metadata.NewOutgoingContext(newCtx, md)
	}
	return newCtx
}

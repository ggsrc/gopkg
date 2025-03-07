package rpcutil

import "context"

type CustomResource interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

func WrapCustomResource(startFunc, closeFunc func(ctx context.Context) error) CustomResource {
	return &wrappedResource{
		start: startFunc,
		close: closeFunc,
	}
}

type wrappedResource struct {
	start func(ctx context.Context) error
	close func(ctx context.Context) error
}

func (r *wrappedResource) Start(ctx context.Context) error {
	return r.start(ctx)
}

func (r *wrappedResource) Close(ctx context.Context) error {
	return r.close(ctx)
}

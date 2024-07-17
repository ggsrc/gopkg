package rpcutil

import "context"

type CustomResource interface {
	Start(ctx context.Context) error
	Close(ctx context.Context) error
}

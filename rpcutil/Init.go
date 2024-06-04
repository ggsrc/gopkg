package rpcutil

import (
	"context"
	gg_grpc "github.com/ggsrc/gopkg/grpc"
	"google.golang.org/grpc"
)

// Defaults for RpcInitHelperOptions.
const (
	DefaultDebug         = true
	DefaultInitDBTimeout = 5000
)

// RpcInitHelperOption configures init.
type RpcInitHelperOption func(o *RpcInitHelperOptions)

// RpcInitHelperOptions is configuration settings for rpc init helper.
type RpcInitHelperOptions struct {
	// Debug is the flag to enable debug mode.
	Debug   bool
	AppName string

	InitWpgx        bool
	InitCache       bool
	InitHealthCheck bool

	InitGrpcServer bool
	GrpcServerConf *gg_grpc.ServerConfig
	GrpcServerOpt  []grpc.ServerOption
}

func WithAppName(appName string) RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.AppName = appName
	}
}

func WithDebug() RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.Debug = true
	}
}

func WithWPGXInit() RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.InitWpgx = true
	}
}

func WithCacheInit() RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.InitCache = true
	}
}

func WithGrpcServerInit(conf *gg_grpc.ServerConfig, opt ...grpc.ServerOption) RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.InitGrpcServer = true
		o.GrpcServerConf = conf
		o.GrpcServerOpt = opt
	}
}

func WithHealthCheckInit() RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.InitHealthCheck = true
	}
}

func InitResource(ctx context.Context, options ...RpcInitHelperOption) (*Resource, error) {
	o := RpcInitHelperOptions{
		Debug: DefaultDebug,
	}
	for _, opt := range options {
		opt(&o)
	}

	var err error
	InitRes.Do(func() {
		var res *Resource
		res, err = NewResource(ctx, o)
		resource = res
	})
	return resource, err
}

func MustInitResource(ctx context.Context, options ...RpcInitHelperOption) *Resource {
	res, err := InitResource(ctx, options...)
	if err != nil {
		panic(err)
	}
	return res
}

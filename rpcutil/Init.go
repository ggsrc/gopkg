package rpcutil

import (
	"context"
	"time"

	"google.golang.org/grpc"

	gg_grpc "github.com/ggsrc/gopkg/grpc"
	"github.com/ggsrc/gopkg/health"
)

// Defaults for RpcInitHelperOptions.
const (
	DefaultDebug         = true
	DefaultInitDBTimeout = time.Second * 5
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
	Checkable       []health.HealthCheckable
	InitMetric      bool

	InitGrpcServer bool
	GrpcServerConf *gg_grpc.ServerConfig
	GrpcServerOpt  []grpc.ServerOption
}

func WithAppName(appName string) RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.AppName = appName
	}
}

func WithDebug(debug bool) RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.Debug = debug
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

func WithHealthCheckInit(checkable ...health.HealthCheckable) RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.InitHealthCheck = true
		o.Checkable = checkable
	}
}

func WithMetricInit() RpcInitHelperOption {
	return func(o *RpcInitHelperOptions) {
		o.InitMetric = true
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
	initRes.Do(func() {
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

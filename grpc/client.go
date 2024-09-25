package grpc

import (
	"context"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	grpcinterceptor "github.com/ggsrc/gopkg/interceptor/grpc"
)

type ClientConfig struct {
	RavenDSN string `default:"https://77f63f901858d8662af2db33c999b6b8@sentry.corp.galxe.com/19"`
	Verbose  bool   `default:"false"`
}

type Client struct {
	serverName string
	clientName string
	conf       *ClientConfig
}

func NewClient(serverName, clientName string, conf *ClientConfig) *Client {
	if conf == nil {
		conf = &ClientConfig{}
		envconfig.MustProcess("grpc", conf)
	}
	return &Client{
		serverName: serverName,
		clientName: clientName,
		conf:       conf,
	}
}

func (c *Client) Dial(ctx context.Context, addr string, opts ...grpc.DialOption) (conn *grpc.ClientConn, err error) {
	logger := zerolog.DefaultContextLogger
	if logger == nil {
		logger = zerolog.Ctx(ctx)
	}

	loggableEvents := []logging.LoggableEvent{}
	if c.conf.Verbose {
		loggableEvents = append(loggableEvents, logging.StartCall)
		loggableEvents = append(loggableEvents, logging.FinishCall)
	}

	defaultOpts := []grpc.DialOption{
		grpc.WithUnaryInterceptor(chainUnaryClient(
			grpcinterceptor.SentryUnaryClientInterceptor(c.conf.RavenDSN),
			grpcinterceptor.ContextUnaryClientInterceptor(),
			logging.UnaryClientInterceptor(InterceptorLogger(*logger), logging.WithLogOnEvents(loggableEvents...)),
			grpcinterceptor.ContextCacheUnaryClientInterceptor(),
			grpc_prometheus.UnaryClientInterceptor,
		)),

		grpc.WithStreamInterceptor(grpc_prometheus.StreamClientInterceptor),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	opts = append(defaultOpts, opts...)

	return grpc.NewClient(addr, opts...)
}

// From https://github.com/grpc-ecosystem/go-grpc-middleware/blob/master/chain.go
func chainUnaryClient(interceptors ...grpc.UnaryClientInterceptor) grpc.UnaryClientInterceptor {
	n := len(interceptors)

	if n > 1 {
		lastI := n - 1
		return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			var (
				chainHandler grpc.UnaryInvoker
				curI         int
			)

			chainHandler = func(currentCtx context.Context, currentMethod string, currentReq, currentRepl interface{}, currentConn *grpc.ClientConn, currentOpts ...grpc.CallOption) error {
				if curI == lastI {
					return invoker(currentCtx, currentMethod, currentReq, currentRepl, currentConn, currentOpts...)
				}
				curI++
				err := interceptors[curI](currentCtx, currentMethod, currentReq, currentRepl, currentConn, chainHandler, currentOpts...)
				curI--
				return err
			}

			return interceptors[0](ctx, method, req, reply, cc, chainHandler, opts...)
		}
	}

	if n == 1 {
		return interceptors[0]
	}

	// n == 0; Dummy interceptor maintained for backward compatibility to avoid returning nil.
	return func(ctx context.Context, method string, req, reply interface{}, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
		return invoker(ctx, method, req, reply, cc, opts...)
	}
}

package grpc

import (
	"context"
	"fmt"
	"net"

	"github.com/grpc-ecosystem/go-grpc-middleware/v2/interceptors/logging"
	grpc_prometheus "github.com/grpc-ecosystem/go-grpc-prometheus"
	"github.com/kelseyhightower/envconfig"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"

	"github.com/ggsrc/gopkg/env"
	grpcinterceptor "github.com/ggsrc/gopkg/interceptor/grpc"
)

type Server struct {
	serviceName string
	conf        *ServerConfig

	server *grpc.Server
}

type ServerConfig struct {
	Debug    bool   `default:"false"`
	Port     int    `default:"9090"`
	RavenDSN string `default:"https://77f63f901858d8662af2db33c999b6b8@sentry.corp.galxe.com/19"`
	// LogMasker takes in FullMethod and req as input and returns masked req
	Verbose bool `default:"false"`
}

func NewServer(serviceName string, conf *ServerConfig, opts ...grpc.ServerOption) *Server {
	if conf == nil {
		conf = &ServerConfig{}
		envconfig.MustProcess("grpc", conf)
	}
	s := &Server{
		serviceName: serviceName,
		conf:        conf,
	}

	logger := zerolog.DefaultContextLogger
	if logger == nil {
		logger = &log.Logger
	}

	loggableEvents := []logging.LoggableEvent{}
	if conf.Verbose {
		loggableEvents = append(loggableEvents, logging.StartCall)
		loggableEvents = append(loggableEvents, logging.FinishCall)
	}

	interceptors := []grpc.UnaryServerInterceptor{
		grpcinterceptor.SentryUnaryServerInterceptor(conf.RavenDSN),
		logging.UnaryServerInterceptor(InterceptorLogger(*logger), logging.WithLogOnEvents(loggableEvents...)),
		grpc_prometheus.UnaryServerInterceptor,
		grpcinterceptor.ContextUnaryServerInterceptor(),
	}

	if conf.Debug || env.IsStaging() {
		interceptors = append(interceptors, grpcinterceptor.LogUnaryServerInterceptor())
	}

	defaultOpts := []grpc.ServerOption{
		grpc.UnaryInterceptor(chainUnaryServer(interceptors...)),
		grpc.StreamInterceptor(grpc_prometheus.StreamServerInterceptor),
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
	}
	defaultOpts = append(defaultOpts, opts...)
	s.server = grpc.NewServer(defaultOpts...)
	grpc_prometheus.Register(s.server)
	// Prometheus histograms are a great way to measure latency distributions of your RPCs. However,
	// since it is bad practice to have metrics of high cardinality the latency monitoring metrics
	// are disabled by default.
	grpc_prometheus.EnableHandlingTimeHistogram()
	return s
}

// Server return the grpc server for registering service.
func (s *Server) Server() *grpc.Server {
	return s.server
}

// Start create a tcp listener and start goroutine for serving each incoming request.
// Start will return a non-nil error unless Stop or GracefulStop is called.
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf(":%d", s.conf.Port))
	if err != nil {
		return err
	}
	return s.Serve(lis)
}

// Serve accepts incoming connections on the listener lis, creating a new
// ServerTransport and service goroutine for each.
// Serve will return a non-nil error unless Stop or GracefulStop is called.
func (s *Server) Serve(lis net.Listener) error {
	return s.server.Serve(lis)
}

// Shutdown stops the server gracefully. It stops the server from
// accepting new connections and RPCs and blocks until all the pending RPCs are
// finished or the context deadline is reached.
func (s *Server) Shutdown(ctx context.Context) (err error) {
	ch := make(chan struct{})
	go func() {
		s.server.GracefulStop()
		close(ch)
	}()
	select {
	case <-ctx.Done():
		s.server.Stop()
		err = ctx.Err()
	case <-ch:
	}
	return
}

// From https://github.com/grpc-ecosystem/go-grpc-middleware/blob/master/chain.go
func chainUnaryServer(interceptors ...grpc.UnaryServerInterceptor) grpc.UnaryServerInterceptor {
	n := len(interceptors)

	if n > 1 {
		lastI := n - 1
		return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
			var (
				chainHandler grpc.UnaryHandler
				curI         int
			)

			chainHandler = func(currentCtx context.Context, currentReq interface{}) (interface{}, error) {
				if curI == lastI {
					return handler(currentCtx, currentReq)
				}
				curI++
				resp, err := interceptors[curI](currentCtx, currentReq, info, chainHandler)
				curI--
				return resp, err
			}

			return interceptors[0](ctx, req, info, chainHandler)
		}
	}

	if n == 1 {
		return interceptors[0]
	}

	// n == 0; Dummy interceptor maintained for backward compatibility to avoid returning nil.
	return func(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		return handler(ctx, req)
	}
}

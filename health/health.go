package health

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/kelseyhightower/envconfig"
	redisV9 "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
	"github.com/stumble/wpgx"
)

type HttpRouter interface {
	HandleFunc(pattern string, handler func(http.ResponseWriter, *http.Request))
	ServeHTTP(writer http.ResponseWriter, request *http.Request)
}

type Server struct {
	conf      *Config
	hooks     []Checkable
	stop      bool
	hooksLock sync.RWMutex

	consecutive int

	ready bool
	alive bool

	httpRouter HttpRouter
}

type HealthCheckable interface {
	OK(ctx context.Context) error
}

type Config struct {
	Port          int `default:"8080"`
	ReadyCount    int
	LiveCount     int           `default:"3"`
	ProbeInterval time.Duration `default:"5s"`
	ProbeTimeout  time.Duration `default:"5s"`
	Ready         bool          `default:"true"`
	Alive         bool          `default:"true"`
}

func New(conf *Config, httpRouter HttpRouter, hc ...HealthCheckable) *Server {
	if conf == nil {
		conf = &Config{}
		envconfig.MustProcess("healthcheck", conf)
	}
	var hooks []Checkable
	for _, h := range hc {
		hooks = append(hooks, h.OK)
	}
	s := &Server{
		conf:       conf,
		hooks:      hooks,
		ready:      conf.Ready,
		alive:      conf.Alive,
		httpRouter: httpRouter,
	}
	go s.start()
	return s
}

func (s *Server) start() {
	for {
		<-time.After(s.conf.ProbeInterval)
		ctx, cancel := context.WithTimeout(context.Background(), s.conf.ProbeTimeout)
		s.hooksLock.RLock()
		if err := GoCheck(ctx, s.hooks...); err != nil {
			s.consecutive++
			log.Error().Err(err).Msg("healthcheck failed")
		} else {
			s.consecutive = 0
		}
		s.hooksLock.RUnlock()
		cancel()
	}
}

func (s *Server) readinessHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.ready {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.stop {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.consecutive <= s.conf.ReadyCount {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func (s *Server) AddHooks(hooks ...Checkable) {
	s.hooksLock.Lock()
	defer s.hooksLock.Unlock()
	s.hooks = append(s.hooks, hooks...)
}

// Ready set service is ready to serve or not
func (s *Server) Ready(serviceReady bool) {
	s.ready = serviceReady
}

// Alive set service is alive or not
func (s *Server) Alive(serviceAlive bool) {
	s.alive = serviceAlive
}

func (s *Server) livenessHandler() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.stop {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if s.consecutive <= s.conf.LiveCount {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusBadRequest)
		}
	}
}

func (s *Server) Start() error {
	if s.httpRouter == nil {
		var mux = http.NewServeMux()
		mux.HandleFunc("/health/ready", s.readinessHandler())
		mux.HandleFunc("/health/alive", s.livenessHandler())
		s.httpRouter = mux
	}
	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", s.conf.Port),
		Handler:           s.httpRouter,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return httpServer.ListenAndServe()
}

func (s *Server) Router() HttpRouter {
	return s.httpRouter
}

// Stop makes the service not ready and not lively
func (s *Server) Stop() {
	s.stop = true
}

type Checkable func(context.Context) error

// GoCheck is a helper to run multiple check functions in parallel and fail if one fails
func GoCheck(ctx context.Context, toCheck ...Checkable) error {
	ch := make(chan error, len(toCheck))
	for _, f := range toCheck {
		go func(f func(context.Context) error) {
			ch <- f(ctx)
		}(f)
	}
	for range toCheck {
		if err := <-ch; err != nil {
			return err
		}
	}
	return nil
}

// CheckRedis is helper function to ping redis
// DEPRECATE: Use CheckRedisV8
// func CheckRedis(redis redis.UniversalClient) func(context.Context) error {
// 	return func(ctx context.Context) error {
// 		ch := make(chan error)
// 		go func() {
// 			ch <- redis.Ping().Err()
// 		}()
// 		select {
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		case err := <-ch:
// 			if err != nil {
// 				log.Error().Err(err).Msg("redis healthcheck failed")
// 			}
// 			return err
// 		}
// 	}
// }

// CheckRedis is helper function to ping redis
// DEPRECATE: Use CheckRedisV9
// func CheckRedisV8(redis redisV8.UniversalClient) func(context.Context) error {
// 	return func(ctx context.Context) error {
// 		ch := make(chan error)
// 		go func() {
// 			ch <- redis.Ping(ctx).Err()
// 		}()
// 		select {
// 		case <-ctx.Done():
// 			return ctx.Err()
// 		case err := <-ch:
// 			if err != nil {
// 				log.Error().Err(err).Msg("redis healthcheck failed")
// 			}
// 			return err
// 		}
// 	}
// }

// CheckRedisV9 is helper function to ping redis
func CheckRedisV9(redis redisV9.UniversalClient) func(context.Context) error {
	return func(ctx context.Context) error {
		ch := make(chan error)
		go func() {
			ch <- redis.Ping(ctx).Err()
		}()
		select {
		case <-ctx.Done():
			return ctx.Err()
		case err := <-ch:
			if err != nil {
				log.Error().Err(err).Msg("redis healthcheck failed")
			}
			return err
		}
	}
}

// CheckSQL is helper function to test write sql
func CheckSQL(conn *sql.DB) func(context.Context) error {
	return func(ctx context.Context) error {
		err := conn.PingContext(ctx)
		if err != nil {
			log.Error().Err(err).Msg("sql healthcheck failed")
		}
		return err
	}
}

// CheckPgSQL is a helper function to check PostgresSQL connection
func CheckPgSQL(pool *wpgx.Pool) func(context.Context) error {
	return func(ctx context.Context) error {
		err := pool.Ping(ctx)
		if err != nil {
			log.Error().Err(err).Msg("sql healthcheck failed")
		}
		return err
	}
}

module github.com/ggsrc/gopkg/rpcutil

go 1.22

replace (
	github.com/ggsrc/gopkg/database/cache => ../database/cache
	github.com/ggsrc/gopkg/database/wpgx => ../database/wpgx
	github.com/ggsrc/gopkg/env => ../env
	github.com/ggsrc/gopkg/goodns => ../goodns
	github.com/ggsrc/gopkg/grpc => ../grpc
	github.com/ggsrc/gopkg/health => ../health
	github.com/ggsrc/gopkg/interceptor => ../interceptor
	github.com/ggsrc/gopkg/metric => ../metric
	github.com/ggsrc/gopkg/profiling => ../profiling
	github.com/ggsrc/gopkg/zerolog => ../zerolog
)

require (
	github.com/ggsrc/gopkg/database/cache v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/database/wpgx v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/env v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/grpc v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/health v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/metric v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/profiling v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/zerolog v0.0.0-20240701121102-34284860bec7
	github.com/go-co-op/gocron/v2 v2.6.0
	github.com/redis/go-redis/v9 v9.5.2
	github.com/stumble/dcache v0.2.0
	github.com/stumble/wpgx v0.2.2
	github.com/uptrace/uptrace-go v1.27.1
	google.golang.org/grpc v1.64.0
)

require (
	dario.cat/mergo v1.0.0 // indirect
	github.com/agoda-com/opentelemetry-go/otelzerolog v0.0.1 // indirect
	github.com/agoda-com/opentelemetry-logs-go v0.5.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/coocood/freecache v1.2.4 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/getsentry/sentry-go v0.28.0 // indirect
	github.com/ggsrc/gopkg/goodns v0.0.0-20240604065326-0b574afd0001 // indirect
	github.com/ggsrc/gopkg/interceptor v0.0.0-20240627103648-9470085e7ddf // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grafana/pyroscope-go v1.1.1 // indirect
	github.com/grafana/pyroscope-go/godeltaprof v0.1.6 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.1.0 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.20.0 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20221227161230-091c0ba34f0a // indirect
	github.com/jackc/pgx/v5 v5.4.3 // indirect
	github.com/jackc/puddle/v2 v2.2.1 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.4.0 // indirect
	github.com/kelseyhightower/envconfig v1.4.0 // indirect
	github.com/klauspost/compress v1.17.8 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/miekg/dns v1.1.59 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.19.1 // indirect
	github.com/prometheus/client_model v0.5.0 // indirect
	github.com/prometheus/common v0.48.0 // indirect
	github.com/prometheus/procfs v0.12.0 // indirect
	github.com/redis/go-redis/extra/rediscmd/v9 v9.0.5 // indirect
	github.com/redis/go-redis/extra/redisotel/v9 v9.0.5 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.52.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/runtime v0.52.0 // indirect
	go.opentelemetry.io/otel v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.3.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.27.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.27.0 // indirect
	go.opentelemetry.io/otel/log v0.3.0 // indirect
	go.opentelemetry.io/otel/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk v1.27.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.3.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.27.0 // indirect
	go.opentelemetry.io/otel/trace v1.27.0 // indirect
	go.opentelemetry.io/proto/otlp v1.2.0 // indirect
	golang.org/x/crypto v0.24.0 // indirect
	golang.org/x/exp v0.0.0-20240416160154-fe59bbe5cc7f // indirect
	golang.org/x/mod v0.17.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sync v0.7.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	golang.org/x/tools v0.21.1-0.20240508182429-e35e4ccd0d2d // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240617180043-68d350f18fd4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240617180043-68d350f18fd4 // indirect
	google.golang.org/protobuf v1.34.2 // indirect
)

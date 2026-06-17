module github.com/ggsrc/gopkg/rpcutil

go 1.26

replace (
	github.com/ggsrc/gopkg/database/cache => ../database/cache
	github.com/ggsrc/gopkg/database/wpgx => ../database/wpgx
	github.com/ggsrc/gopkg/env => ../env
	github.com/ggsrc/gopkg/goodns => ../goodns
	github.com/ggsrc/gopkg/grpc => ../grpc
	github.com/ggsrc/gopkg/health => ../health
	github.com/ggsrc/gopkg/interceptor => ../interceptor
	github.com/ggsrc/gopkg/mctx => ../mctx
	github.com/ggsrc/gopkg/metric => ../metric
	github.com/ggsrc/gopkg/profiling => ../profiling
	github.com/ggsrc/gopkg/utils => ../utils
	github.com/ggsrc/gopkg/zerolog => ../zerolog
)

require (
	github.com/ggsrc/gopkg/database/cache v0.0.0-20250110085348-9283cf95374b
	github.com/ggsrc/gopkg/database/wpgx v0.0.0-20250110085348-9283cf95374b
	github.com/ggsrc/gopkg/env v0.0.0-20250307074235-8cbb76b9e006
	github.com/ggsrc/gopkg/grpc v0.0.0-20250110085348-9283cf95374b
	github.com/ggsrc/gopkg/health v0.0.0-20250110085348-9283cf95374b
	github.com/ggsrc/gopkg/metric v0.0.0-20250110085348-9283cf95374b
	github.com/ggsrc/gopkg/profiling v0.0.0-20250110085348-9283cf95374b
	github.com/ggsrc/gopkg/zerolog v0.0.0-20250307074235-8cbb76b9e006
	github.com/go-co-op/gocron/v2 v2.21.1
	github.com/hatchet-dev/hatchet v0.90.1
	github.com/jackc/pgx/v5 v5.9.2
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/posthog/posthog-go v1.12.1
	github.com/redis/go-redis/v9 v9.8.0
	github.com/stumble/dcache v0.3.0
	github.com/stumble/wpgx v0.3.1
	github.com/uptrace/uptrace-go v1.30.1
	google.golang.org/grpc v1.80.0
)

require (
	cel.dev/expr v0.25.1 // indirect
	dario.cat/mergo v1.0.2 // indirect
	github.com/Masterminds/semver/v3 v3.4.0 // indirect
	github.com/agoda-com/opentelemetry-go/otelzerolog v0.0.2-0.20240530231629-5ecb4b699e80 // indirect
	github.com/agoda-com/opentelemetry-logs-go v0.5.1 // indirect
	github.com/antlr4-go/antlr/v4 v4.13.1 // indirect
	github.com/apapsch/go-jsonmerge/v2 v2.0.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bytedance/gopkg v0.1.1 // indirect
	github.com/bytedance/sonic v1.12.3 // indirect
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cenkalti/backoff/v5 v5.0.3 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/coocood/freecache v1.2.4 // indirect
	github.com/creasty/defaults v1.8.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/fsnotify/fsnotify v1.9.0 // indirect
	github.com/gabriel-vasile/mimetype v1.4.13 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/getkin/kin-openapi v0.135.0 // indirect
	github.com/getsentry/sentry-go v0.45.1 // indirect
	github.com/ggsrc/gopkg/goodns v0.0.0-20240701121102-34284860bec7 // indirect
	github.com/ggsrc/gopkg/interceptor v0.0.0-20250307074235-8cbb76b9e006 // indirect
	github.com/ggsrc/gopkg/mctx v0.0.0-20250307074235-8cbb76b9e006 // indirect
	github.com/ggsrc/gopkg/utils v0.0.0-20250307074235-8cbb76b9e006 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/go-openapi/jsonpointer v0.21.0 // indirect
	github.com/go-openapi/swag v0.23.0 // indirect
	github.com/go-playground/locales v0.14.1 // indirect
	github.com/go-playground/universal-translator v0.18.1 // indirect
	github.com/go-playground/validator/v10 v10.30.2 // indirect
	github.com/go-viper/mapstructure/v2 v2.4.0 // indirect
	github.com/goccy/go-json v0.10.5 // indirect
	github.com/google/cel-go v0.28.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/grafana/pyroscope-go v1.2.0 // indirect
	github.com/grafana/pyroscope-go/godeltaprof v0.1.8 // indirect
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.3 // indirect
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0 // indirect
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.28.0 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/hashicorp/go-multierror v1.1.1 // indirect
	github.com/hashicorp/golang-lru/v2 v2.0.7 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/jonboulle/clockwork v0.5.0 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/klauspost/compress v1.18.5 // indirect
	github.com/klauspost/cpuid/v2 v2.2.7 // indirect
	github.com/labstack/echo/v4 v4.15.1 // indirect
	github.com/labstack/gommon v0.4.2 // indirect
	github.com/leodido/go-urn v1.4.0 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/maypok86/otter v1.2.3 // indirect
	github.com/miekg/dns v1.1.62 // indirect
	github.com/mohae/deepcopy v0.0.0-20170929034955-c48cc78d4826 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/oapi-codegen/runtime v1.4.0 // indirect
	github.com/oasdiff/yaml v0.0.9 // indirect
	github.com/oasdiff/yaml3 v0.0.9 // indirect
	github.com/pelletier/go-toml/v2 v2.2.4 // indirect
	github.com/perimeterx/marshmallow v1.1.5 // indirect
	github.com/pkg/errors v0.9.1 // indirect
	github.com/prometheus/client_golang v1.23.2 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.66.1 // indirect
	github.com/prometheus/procfs v0.19.2 // indirect
	github.com/redis/go-redis/extra/rediscmd/v9 v9.0.5 // indirect
	github.com/redis/go-redis/extra/redisotel/v9 v9.0.5 // indirect
	github.com/robfig/cron/v3 v3.0.1 // indirect
	github.com/rs/zerolog v1.35.1 // indirect
	github.com/sagikazarmark/locafero v0.11.0 // indirect
	github.com/satori/go.uuid v1.2.0 // indirect
	github.com/showa-93/go-mask v0.6.2 // indirect
	github.com/sourcegraph/conc v0.3.1-0.20240121214520-5f936abd7ae8 // indirect
	github.com/spf13/afero v1.15.0 // indirect
	github.com/spf13/cast v1.10.0 // indirect
	github.com/spf13/pflag v1.0.10 // indirect
	github.com/spf13/viper v1.21.0 // indirect
	github.com/subosito/gotenv v1.6.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	github.com/valyala/bytebufferpool v1.0.0 // indirect
	github.com/valyala/fasttemplate v1.2.2 // indirect
	github.com/vmihailenco/msgpack/v5 v5.4.1 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/woodsbury/decimal128 v1.3.0 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.68.0 // indirect
	go.opentelemetry.io/contrib/instrumentation/runtime v0.63.0 // indirect
	go.opentelemetry.io/otel v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlplog/otlploghttp v0.14.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetrichttp v1.38.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp v1.43.0 // indirect
	go.opentelemetry.io/otel/exporters/stdout/stdouttrace v1.38.0 // indirect
	go.opentelemetry.io/otel/log v0.14.0 // indirect
	go.opentelemetry.io/otel/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk v1.43.0 // indirect
	go.opentelemetry.io/otel/sdk/log v0.14.0 // indirect
	go.opentelemetry.io/otel/sdk/metric v1.43.0 // indirect
	go.opentelemetry.io/otel/trace v1.43.0 // indirect
	go.opentelemetry.io/proto/otlp v1.10.0 // indirect
	go.yaml.in/yaml/v2 v2.4.3 // indirect
	go.yaml.in/yaml/v3 v3.0.4 // indirect
	golang.org/x/arch v0.8.0 // indirect
	golang.org/x/crypto v0.50.0 // indirect
	golang.org/x/exp v0.0.0-20260218203240-3dfff04db8fa // indirect
	golang.org/x/mod v0.34.0 // indirect
	golang.org/x/net v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.43.0 // indirect
	golang.org/x/text v0.36.0 // indirect
	golang.org/x/tools v0.43.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260401024825-9d38bb4040a9 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20260406210006-6f92a3bedf2d // indirect
	google.golang.org/protobuf v1.36.11 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

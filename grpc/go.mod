module github.com/ggsrc/gopkg/grpc

go 1.25.0

replace (
	github.com/ggsrc/gopkg/env => ../env
	github.com/ggsrc/gopkg/interceptor => ../interceptor
	github.com/ggsrc/gopkg/mctx => ../mctx
	github.com/ggsrc/gopkg/utils => ../utils
	github.com/ggsrc/gopkg/zerolog => ../zerolog
)

require (
	github.com/ggsrc/gopkg/env v0.0.0-20250307074235-8cbb76b9e006
	github.com/ggsrc/gopkg/interceptor v0.0.0-20250307074235-8cbb76b9e006
	github.com/grpc-ecosystem/go-grpc-middleware/v2 v2.3.2
	github.com/grpc-ecosystem/go-grpc-prometheus v1.2.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/rs/zerolog v1.34.0
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.55.0
	google.golang.org/grpc v1.76.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/bytedance/gopkg v0.1.1 // indirect
	github.com/bytedance/sonic v1.12.3 // indirect
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/getsentry/sentry-go v0.32.0 // indirect
	github.com/ggsrc/gopkg/mctx v0.0.0-20250307074235-8cbb76b9e006 // indirect
	github.com/ggsrc/gopkg/utils v0.0.0-20250307074235-8cbb76b9e006 // indirect
	github.com/ggsrc/gopkg/zerolog v0.0.0-20250307074235-8cbb76b9e006 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/jinzhu/copier v0.4.0 // indirect
	github.com/klauspost/cpuid/v2 v2.2.5 // indirect
	github.com/mattn/go-colorable v0.1.14 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/maypok86/otter v1.2.3 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_golang v1.24.0 // indirect
	github.com/prometheus/client_model v0.6.2 // indirect
	github.com/prometheus/common v0.70.0 // indirect
	github.com/prometheus/procfs v0.21.1 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.38.0 // indirect
	go.opentelemetry.io/otel/metric v1.38.0 // indirect
	go.opentelemetry.io/otel/trace v1.38.0 // indirect
	golang.org/x/arch v0.4.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.21.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.38.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251103181224-f26f9409b101 // indirect
	google.golang.org/protobuf v1.36.11 // indirect
)

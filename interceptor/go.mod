module github.com/ggsrc/gopkg/interceptor

go 1.22

replace (
	github.com/ggsrc/gopkg/env => ../env
	github.com/ggsrc/gopkg/zerolog => ../zerolog
)

require (
	github.com/bytedance/sonic v1.12.2
	github.com/getsentry/sentry-go v0.28.0
	github.com/ggsrc/gopkg/env v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/zerolog v0.0.0-20240701121102-34284860bec7
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/jinzhu/copier v0.4.0
	go.opentelemetry.io/otel v1.29.0
	go.opentelemetry.io/otel/trace v1.29.0
	google.golang.org/grpc v1.65.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/agoda-com/opentelemetry-go/otelzerolog v0.0.2-0.20240530231629-5ecb4b699e80 // indirect
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pingcap/errors v0.11.5-0.20211224045212-9687c2b0f87c // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240823204242-4ba0660f739c // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240823204242-4ba0660f739c // indirect
)

module github.com/ggsrc/gopkg/interceptor

go 1.22

replace (
	github.com/ggsrc/gopkg/env => ../env
	github.com/ggsrc/gopkg/utils => ../utils
	github.com/ggsrc/gopkg/zerolog => ../zerolog
)

require (
	github.com/bytedance/gopkg v0.1.1
	github.com/bytedance/sonic v1.12.3
	github.com/degenChat/gopkg/interceptor v0.0.0-20240926142320-fe9714e0b197
	github.com/getsentry/sentry-go v0.29.0
	github.com/ggsrc/gopkg/env v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/utils v0.0.0-20240925113213-b8f2b05dcb7a
	github.com/ggsrc/gopkg/zerolog v0.0.0-20240925113213-b8f2b05dcb7a
	github.com/jinzhu/copier v0.4.0
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/otel v1.30.0
	go.opentelemetry.io/otel/trace v1.30.0
	google.golang.org/grpc v1.66.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/agoda-com/opentelemetry-go/otelzerolog v0.0.2-0.20240530231629-5ecb4b699e80 // indirect
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/kr/text v0.1.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.opentelemetry.io/otel/metric v1.30.0 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/net v0.28.0 // indirect
	golang.org/x/sys v0.24.0 // indirect
	golang.org/x/text v0.17.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240823204242-4ba0660f739c // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

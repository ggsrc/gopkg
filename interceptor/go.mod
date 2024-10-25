module github.com/ggsrc/gopkg/interceptor

go 1.22

replace (
	github.com/ggsrc/gopkg/env => ../env
	github.com/ggsrc/gopkg/mctx => ../mctx
	github.com/ggsrc/gopkg/utils => ../utils
	github.com/ggsrc/gopkg/zerolog => ../zerolog
)

require (
	github.com/bytedance/gopkg v0.1.1
	github.com/bytedance/sonic v1.12.3
	github.com/getsentry/sentry-go v0.29.0
	github.com/ggsrc/gopkg/env v0.0.0-20240701121102-34284860bec7
	github.com/ggsrc/gopkg/mctx v0.0.0-20241023073304-d2bc50fff59d
	github.com/ggsrc/gopkg/utils v0.0.0-20241023073304-d2bc50fff59d
	github.com/ggsrc/gopkg/zerolog v0.0.0-20240927074458-8b290f89f1fb
	github.com/jinzhu/copier v0.4.0
	github.com/kelseyhightower/envconfig v1.4.0
	github.com/maypok86/otter v1.2.3
	github.com/stretchr/testify v1.9.0
	go.opentelemetry.io/otel v1.30.0
	go.opentelemetry.io/otel/trace v1.30.0
	golang.org/x/sync v0.8.0
	google.golang.org/grpc v1.67.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/bytedance/sonic/loader v0.2.0 // indirect
	github.com/cloudwego/base64x v0.1.4 // indirect
	github.com/cloudwego/iasm v0.2.0 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/dolthub/maphash v0.1.0 // indirect
	github.com/gammazero/deque v0.2.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/klauspost/cpuid/v2 v2.0.9 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pingcap/errors v0.11.5-0.20211224045212-9687c2b0f87c // indirect
	github.com/pmezard/go-difflib v1.0.0 // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	github.com/twitchyliquid64/golang-asm v0.15.1 // indirect
	go.opentelemetry.io/otel/metric v1.30.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/arch v0.0.0-20210923205945-b76863e36670 // indirect
	golang.org/x/net v0.29.0 // indirect
	golang.org/x/sys v0.25.0 // indirect
	golang.org/x/text v0.18.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240903143218-8af14fe29dc1 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)

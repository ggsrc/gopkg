module github.com/ggsrc/gopkg/interceptor

go 1.22

replace github.com/ggsrc/gopkg/env => ../env

replace github.com/ggsrc/gopkg/zerolog => ../zerolog

require (
	github.com/getsentry/sentry-go v0.28.0
	github.com/ggsrc/gopkg/env v0.0.0-20240627103648-9470085e7ddf
	github.com/ggsrc/gopkg/zerolog v0.0.0-20240627103648-9470085e7ddf
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/jinzhu/copier v0.4.0
	google.golang.org/grpc v1.64.0
	google.golang.org/protobuf v1.34.2
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pingcap/errors v0.11.5-0.20211224045212-9687c2b0f87c // indirect
	github.com/rs/zerolog v1.33.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/net v0.26.0 // indirect
	golang.org/x/sys v0.21.0 // indirect
	golang.org/x/text v0.16.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20240617180043-68d350f18fd4 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240617180043-68d350f18fd4 // indirect
)

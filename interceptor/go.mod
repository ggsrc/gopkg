module github.com/ggsrc/gopkg/interceptor

go 1.22.3

replace github.com/ggsrc/gopkg/env => ../env

require (
	github.com/getsentry/sentry-go v0.28.0
	github.com/ggsrc/gopkg/env v0.0.0-00010101000000-000000000000
	github.com/grpc-ecosystem/go-grpc-middleware v1.4.0
	github.com/rs/zerolog v1.33.0
	google.golang.org/grpc v1.64.0
)

require (
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/pingcap/errors v0.11.5-0.20211224045212-9687c2b0f87c // indirect
	github.com/stretchr/testify v1.9.0 // indirect
	go.uber.org/atomic v1.10.0 // indirect
	golang.org/x/net v0.25.0 // indirect
	golang.org/x/sys v0.20.0 // indirect
	golang.org/x/text v0.15.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20240506185236-b8a5c65736ae // indirect
	google.golang.org/protobuf v1.34.1 // indirect
)

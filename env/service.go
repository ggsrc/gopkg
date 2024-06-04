package env

import (
	"os"
	"sync"
)

const (
	UnknownServiceName = "UnknownService"
)

var (
	serviceName    string
	setServiceName sync.Once
)

func init() {
	serviceName = os.Getenv("SERVICE_NAME")
	if serviceName == "" {
		serviceName = os.Getenv("service_name")
	}
	if serviceName == "" {
		serviceName = os.Getenv("service-name")
	}
	if serviceName == "" {
		serviceName = os.Getenv("OTEL_SERVICE_NAME")
	}
	if serviceName == "" {
		serviceName = UnknownServiceName
	}
}

func SetServiceName(name string) {
	setServiceName.Do(func() {
		serviceName = name
	})
}

func ServiceName() string {
	return serviceName
}

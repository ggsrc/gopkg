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

	serviceVersion    string
	setServiceVersion sync.Once
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

	serviceVersion = os.Getenv("SERVICE_VERSION")
	if serviceVersion == "" {
		serviceVersion = os.Getenv("service_version")
	}
	if serviceVersion == "" {
		serviceVersion = os.Getenv("service-version")
	}
	if serviceVersion == "" {
		serviceVersion = os.Getenv("OTEL_SERVICE_VERSION")
	}
	if serviceVersion == "" {
		serviceVersion = os.Getenv("GIT_SHA")
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

func ServiceVersion() string {
	return serviceVersion
}

func SetServiceVersion(version string) {
	setServiceVersion.Do(func() {
		serviceVersion = version
	})
}
